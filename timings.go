package main

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const timingColumnCount = 7

var timingColumns = [timingColumnCount]string{
	"force_prep",
	"mom_advection",
	"matrix_prep",
	"solver",
	"3D_vel",
	"transport",
	"outputs",
}

var nonFatalTimingPrefixes = [][]string{
	{"Time (sec) taken for force prep="},
	{"Time taken for mom advection="},
	{
		"Time taken for matrix prep=",
		"Time taken for maxtrix prep=",
	},
	{"Time for solver="},
	{"Time taken for 3D vel="},
	{"Time taken for transport="},
	{"Time taken for outputs="},
}

const maxScannerTokenSize = 1024 * 1024

type runTiming struct {
	Identifier   string
	Ranks        int
	Threads      int
	Scribes      int
	Elements     int
	Nodes        int
	Layers       int
	Tracers      int
	DT           int
	Rnday        float64
	Timings      [timingColumnCount]float64
	StepsTotal   float64
	InitDuration float64
	Duration     float64
}

type runLayout struct {
	RunDir    string
	Outputs   string
	ParamNML  string
	MirrorOut string
	NonFatal  string
}

func analyzeRun(path string, root string) (runTiming, error) {
	layout, err := resolveRunLayout(path)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: %w", path, err)
	}

	dt, err := parseParamOutNML(layout.ParamNML)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: parse param.out.nml: %w", path, err)
	}

	metadata, durationSec, hasDuration, err := parseMirrorOut(layout.MirrorOut)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: parse mirror.out metadata: %w", path, err)
	}

	stats, err := parseNonFatal(layout.NonFatal)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: parse nonfatal_000000: %w", path, err)
	}
	if len(stats) == 0 {
		return runTiming{
			Identifier:   runIdentifier(layout.RunDir, root),
			Ranks:        metadata.Ranks,
			Threads:      metadata.Threads,
			Scribes:      metadata.Scribes,
			Elements:     metadata.Elements,
			Nodes:        metadata.Nodes,
			Layers:       metadata.Layers,
			Tracers:      metadata.Tracers,
			DT:           dt,
			Rnday:        math.NaN(),
			Timings:      nanTimings(),
			StepsTotal:   math.NaN(),
			InitDuration: math.NaN(),
			Duration:     optionalHours(durationSec, hasDuration),
		}, nil
	}

	var sums [timingColumnCount]float64
	for _, row := range stats {
		for i, value := range row {
			sums[i] += value
		}
	}

	rnday := float64(len(stats)*dt) / 86400.0
	if rnday <= 0 {
		return runTiming{}, fmt.Errorf("%s: non-positive analyzed RNDAY %.12g", path, rnday)
	}

	var timings [timingColumnCount]float64
	var stepsTotalSec float64
	var stepsTotalPerRnday float64
	for i, value := range sums {
		timings[i] = value / rnday / 3600.0
		stepsTotalPerRnday += timings[i]
		stepsTotalSec += value
	}

	init := math.NaN()
	duration := -stepsTotalSec / 3600.0
	if hasDuration {
		init = (durationSec - stepsTotalSec) / 3600.0
		duration = durationSec / 3600.0
	}

	return runTiming{
		Identifier:   runIdentifier(layout.RunDir, root),
		Ranks:        metadata.Ranks,
		Threads:      metadata.Threads,
		Scribes:      metadata.Scribes,
		Elements:     metadata.Elements,
		Nodes:        metadata.Nodes,
		Layers:       metadata.Layers,
		Tracers:      metadata.Tracers,
		DT:           dt,
		Rnday:        rnday,
		Timings:      timings,
		StepsTotal:   stepsTotalPerRnday,
		InitDuration: init,
		Duration:     duration,
	}, nil
}

func nanTimings() [timingColumnCount]float64 {
	var timings [timingColumnCount]float64
	for index := range timings {
		timings[index] = math.NaN()
	}
	return timings
}

func optionalHours(seconds float64, ok bool) float64 {
	if !ok {
		return math.NaN()
	}
	return seconds / 3600.0
}

func resolveRunLayout(path string) (runLayout, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return runLayout{}, err
	}
	if !info.IsDir() {
		return runLayout{}, fmt.Errorf("%s is not a directory", path)
	}

	runDir := path
	outputs := filepath.Join(path, "outputs")
	if filepath.Base(path) == "outputs" {
		outputs = path
		runDir = filepath.Dir(path)
	}

	layout := runLayout{
		RunDir:    runDir,
		Outputs:   outputs,
		ParamNML:  filepath.Join(outputs, "param.out.nml"),
		MirrorOut: filepath.Join(outputs, "mirror.out"),
		NonFatal:  filepath.Join(outputs, "nonfatal_000000"),
	}

	if _, err := os.Stat(layout.ParamNML); err != nil {
		return runLayout{}, fmt.Errorf("missing required file %s: %w", layout.ParamNML, err)
	}

	return layout, nil
}

func runIdentifier(runDir, root string) string {
	runDir = filepath.Clean(runDir)
	if root != "" {
		root = filepath.Clean(root)
		rel, err := filepath.Rel(root, runDir)
		if err == nil {
			return filepath.ToSlash(rel)
		}
	}
	parent := filepath.Base(filepath.Dir(runDir))
	base := filepath.Base(runDir)
	if parent == "." || parent == string(filepath.Separator) {
		return base
	}
	return filepath.ToSlash(filepath.Join(parent, base))
}

func parseParamOutNML(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	var dt int
	var haveDT bool

	scanner := newScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "DT=") {
			value, err := parseNMLInt(line)
			if err != nil {
				return 0, fmt.Errorf("DT: %w", err)
			}
			dt = value
			haveDT = true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if !haveDT {
		return 0, errors.New("DT not found")
	}
	return dt, nil
}

func parseNMLInt(line string) (int, error) {
	value, err := parseNMLFloat(line)
	if err != nil {
		return 0, err
	}
	if value != math.Trunc(value) {
		return 0, fmt.Errorf("expected integer, got %.12g", value)
	}
	if value <= 0 {
		return 0, fmt.Errorf("expected positive integer, got %.12g", value)
	}
	return int(value), nil
}

func parseNMLFloat(line string) (float64, error) {
	_, value, ok := strings.Cut(line, "=")
	if !ok {
		return 0, fmt.Errorf("missing '=' in %q", line)
	}
	value = strings.TrimSpace(value)
	if beforeComma, _, ok := strings.Cut(value, ","); ok {
		value = beforeComma
	}
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

func parseNonFatal(path string) ([][timingColumnCount]float64, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var rows [][timingColumnCount]float64
	var row [timingColumnCount]float64
	nextColumn := 0
	lineNumber := 0
	scanner := newScanner(file)
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		column, ok := nonFatalTimingColumn(line)
		if !ok {
			continue
		}
		if column != nextColumn {
			row = [timingColumnCount]float64{}
			nextColumn = 0
			if column != 0 {
				continue
			}
		}
		_, rhs, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields := strings.Fields(rhs)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		row[column] = value
		nextColumn++
		if nextColumn == len(timingColumns) {
			rows = append(rows, row)
			row = [timingColumnCount]float64{}
			nextColumn = 0
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rows, nil
}

func nonFatalTimingColumn(line string) (int, bool) {
	for column, prefixes := range nonFatalTimingPrefixes {
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				return column, true
			}
		}
	}
	return 0, false
}

type runMetadata struct {
	Ranks    int
	Threads  int
	Scribes  int
	Elements int
	Nodes    int
	Layers   int
	Tracers  int
}

func parseMirrorOut(path string) (runMetadata, float64, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return runMetadata{}, 0, false, err
	}
	defer func() { _ = file.Close() }()

	scanner := newScanner(file)
	var metadata runMetadata
	var first, last string
	var haveTracers, haveGrid, haveRanks, haveScribes bool
	var rankRows map[int]bool
	inRankTable := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && first == "" {
			first = line
		}
		if trimmed != "" {
			last = line
		}

		if inRankTable {
			if trimmed == "" {
				continue
			}
			rank, ok, err := parseRankTableRow(trimmed)
			if err != nil {
				return runMetadata{}, 0, false, err
			}
			if ok {
				if rankRows[rank] {
					return runMetadata{}, 0, false, fmt.Errorf("duplicate rank %d in mirror.out rank table", rank)
				}
				rankRows[rank] = true
				continue
			}
			if len(rankRows) > 0 {
				ranks, err := rankCount(rankRows)
				if err != nil {
					return runMetadata{}, 0, false, err
				}
				metadata.Ranks = ranks
				haveRanks = true
			}
			inRankTable = false
		}

		if !haveTracers && strings.HasPrefix(trimmed, "Total # of tracers=") {
			tracers, err := parseMirrorTracers(trimmed)
			if err != nil {
				return runMetadata{}, 0, false, err
			}
			metadata.Tracers = tracers
			haveTracers = true
			continue
		}

		if strings.HasPrefix(trimmed, "hybrid openMP-MPI run with # of threads=") {
			threads, err := parseMirrorThreads(trimmed)
			if err != nil {
				return runMetadata{}, 0, false, err
			}
			metadata.Threads = threads
			continue
		}

		if !haveGrid && strings.HasPrefix(trimmed, "Global Grid Size") {
			elements, nodes, layers, err := parseMirrorGridSize(trimmed)
			if err != nil {
				return runMetadata{}, 0, false, err
			}
			metadata.Elements = elements
			metadata.Nodes = nodes
			metadata.Layers = layers
			haveGrid = true
			continue
		}

		if !haveRanks && isRankTableHeader(trimmed) {
			inRankTable = true
			rankRows = make(map[int]bool)
			continue
		}

		if !haveScribes && strings.HasPrefix(trimmed, "# of scribe can be set as small as:") {
			scribes, err := parseMirrorScribes(trimmed)
			if err != nil {
				return runMetadata{}, 0, false, err
			}
			metadata.Scribes = scribes
			haveScribes = true
			continue
		}
	}
	if inRankTable && len(rankRows) > 0 {
		ranks, err := rankCount(rankRows)
		if err != nil {
			return runMetadata{}, 0, false, err
		}
		metadata.Ranks = ranks
		haveRanks = true
	}
	if err := scanner.Err(); err != nil {
		return runMetadata{}, 0, false, err
	}

	switch {
	case !haveTracers:
		return runMetadata{}, 0, false, errors.New("Total # of tracers not found")
	case !haveGrid:
		return runMetadata{}, 0, false, errors.New("Global Grid Size not found")
	case !haveRanks:
		return runMetadata{}, 0, false, errors.New("rank table not found")
	case !haveScribes:
		return runMetadata{}, 0, false, errors.New("# of scribe line not found")
	}

	start, err := parseMirrorTimestamp(first)
	if err != nil {
		return metadata, 0, false, nil
	}
	end, err := parseMirrorTimestamp(last)
	if err != nil {
		return metadata, 0, false, nil
	}
	return metadata, end.Sub(start).Seconds(), true, nil
}

func parseMirrorTracers(line string) (int, error) {
	_, value, ok := strings.Cut(line, "=")
	if !ok {
		return 0, fmt.Errorf("parse tracers from %q: missing '='", line)
	}
	fields := strings.Fields(value)
	if len(fields) < 1 {
		return 0, fmt.Errorf("parse tracers from %q: missing value", line)
	}
	return parseNonNegativeField(fields, 0, "tracers")
}

func parseMirrorThreads(line string) (int, error) {
	_, value, ok := strings.Cut(line, "=")
	if !ok {
		return 0, fmt.Errorf("parse OpenMP threads from %q: missing '='", line)
	}
	fields := strings.Fields(value)
	if len(fields) < 1 {
		return 0, fmt.Errorf("parse OpenMP threads from %q: missing value", line)
	}
	return parsePositiveField(fields, 0, "threads")
}

func parseMirrorGridSize(line string) (int, int, int, error) {
	_, value, ok := strings.Cut(line, ":")
	if !ok {
		return 0, 0, 0, fmt.Errorf("parse grid size from %q: missing ':'", line)
	}
	fields := strings.Fields(value)
	if len(fields) < 4 {
		return 0, 0, 0, fmt.Errorf("expected at least 4 Global Grid Size fields, got %d", len(fields))
	}
	elements, err := parsePositiveField(fields, 0, "elements")
	if err != nil {
		return 0, 0, 0, err
	}
	nodes, err := parsePositiveField(fields, 1, "nodes")
	if err != nil {
		return 0, 0, 0, err
	}
	layers, err := parsePositiveField(fields, 3, "layers")
	if err != nil {
		return 0, 0, 0, err
	}
	return elements, nodes, layers, nil
}

func parseMirrorScribes(line string) (int, error) {
	_, value, ok := strings.Cut(line, ":")
	if !ok {
		return 0, fmt.Errorf("parse scribes from %q: missing ':'", line)
	}
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return 0, fmt.Errorf("expected at least 2 scribe fields, got %d", len(fields))
	}
	return parsePositiveField(fields, 1, "scribes")
}

func isRankTableHeader(line string) bool {
	fields := strings.Fields(line)
	return len(fields) > 0 && fields[0] == "rank"
}

func parseRankTableRow(line string) (int, bool, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return 0, false, nil
	}
	rank, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false, nil
	}
	if rank < 0 {
		return 0, false, fmt.Errorf("negative rank %d in mirror.out rank table", rank)
	}
	return rank, true, nil
}

func rankCount(rows map[int]bool) (int, error) {
	if len(rows) == 0 {
		return 0, errors.New("empty mirror.out rank table")
	}

	maxRank := -1
	for rank := range rows {
		if rank > maxRank {
			maxRank = rank
		}
	}
	for rank := 0; rank <= maxRank; rank++ {
		if !rows[rank] {
			return 0, fmt.Errorf("missing rank %d in mirror.out rank table", rank)
		}
	}
	return maxRank + 1, nil
}

func parseMirrorTimestamp(line string) (time.Time, error) {
	idx := strings.LastIndex(line, " at ")
	if idx == -1 {
		return time.Time{}, fmt.Errorf("missing timestamp marker")
	}
	value := strings.TrimSpace(line[idx+4:])
	return time.Parse("20060102, 150405.999999999", value)
}

func newScanner(file *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxScannerTokenSize)
	return scanner
}

func parsePositiveField(fields []string, index int, name string) (int, error) {
	value, err := parseNonNegativeField(fields, index, name)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("non-positive %s %d", name, value)
	}
	return value, nil
}

func parseNonNegativeField(fields []string, index int, name string) (int, error) {
	value, err := strconv.Atoi(fields[index])
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("negative %s %d", name, value)
	}
	return value, nil
}
