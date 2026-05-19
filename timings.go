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

var timingColumns = []string{
	"force_prep",
	"mom_advection",
	"matrix_prep",
	"solver",
	"3D_vel",
	"transport",
	"outputs",
}

var resultColumns = []string{
	"identifier",
	"ranks",
	"elements",
	"nodes",
	"layers",
	"tracers",
	"dt",
	"rnday",
	"force_prep",
	"mom_advection",
	"matrix_prep",
	"solver",
	"3D_vel",
	"transport",
	"outputs",
	"steps_total",
	"init",
	"duration",
}

var errNoTimingSamples = errors.New("no complete timing samples")

type runTiming struct {
	Identifier   string
	Ranks        int
	Elements     int
	Nodes        int
	Layers       int
	Tracers      int
	DT           int
	Rnday        float64
	Timings      [7]float64
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

func analyzeRun(path string) (runTiming, error) {
	layout, err := resolveRunLayout(path)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: %w", path, err)
	}

	_, dt, err := parseParamOutNML(layout.ParamNML)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: parse param.out.nml: %w", path, err)
	}

	stats, err := parseNonFatal(layout.NonFatal)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: parse nonfatal_000000: %w", path, err)
	}
	if len(stats) == 0 {
		return runTiming{}, fmt.Errorf("%s: %w", path, errNoTimingSamples)
	}

	durationSec, hasDuration := parseMirrorDuration(layout.MirrorOut)
	mesh, err := readMeshInfo(layout.Outputs)
	if err != nil {
		return runTiming{}, fmt.Errorf("%s: read mesh metadata: %w", path, err)
	}

	var sums [7]float64
	for _, row := range stats {
		for i, value := range row {
			sums[i] += value
		}
	}

	rnday := float64(len(stats)*dt) / 86400.0
	if rnday <= 0 {
		return runTiming{}, fmt.Errorf("%s: non-positive analyzed RNDAY %.12g", path, rnday)
	}

	var timings [7]float64
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
		Identifier:   runIdentifier(layout.RunDir),
		Ranks:        mesh.Ranks,
		Elements:     mesh.Elements,
		Nodes:        mesh.Nodes,
		Layers:       mesh.Layers,
		Tracers:      mesh.Tracers,
		DT:           dt,
		Rnday:        rnday,
		Timings:      timings,
		StepsTotal:   stepsTotalPerRnday,
		InitDuration: init,
		Duration:     duration,
	}, nil
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

	for _, required := range []string{layout.ParamNML, layout.NonFatal} {
		if _, err := os.Stat(required); err != nil {
			return runLayout{}, fmt.Errorf("missing required file %s: %w", required, err)
		}
	}

	return layout, nil
}

func runIdentifier(runDir string) string {
	runDir = filepath.Clean(runDir)
	parent := filepath.Base(filepath.Dir(runDir))
	base := filepath.Base(runDir)
	if parent == "." || parent == string(filepath.Separator) {
		return base
	}
	return filepath.ToSlash(filepath.Join(parent, base))
}

func parseParamOutNML(path string) (float64, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = file.Close() }()

	var rnday float64
	var dt int
	var haveRnday, haveDT bool

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "RNDAY="):
			value, err := parseNMLFloat(line)
			if err != nil {
				return 0, 0, fmt.Errorf("RNDAY: %w", err)
			}
			rnday = value
			haveRnday = true
		case strings.HasPrefix(line, "DT="):
			value, err := parseNMLInt(line)
			if err != nil {
				return 0, 0, fmt.Errorf("DT: %w", err)
			}
			dt = value
			haveDT = true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if !haveRnday {
		return 0, 0, errors.New("RNDAY not found")
	}
	if !haveDT {
		return 0, 0, errors.New("DT not found")
	}
	return rnday, dt, nil
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

func parseNonFatal(path string) ([][7]float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var values []float64
	lineNumber := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNumber++
		if lineNumber <= 2 {
			continue
		}
		line := scanner.Text()
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
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	values = values[:len(values)/len(timingColumns)*len(timingColumns)]
	rows := make([][7]float64, 0, len(values)/len(timingColumns))
	for i := 0; i < len(values); i += len(timingColumns) {
		var row [7]float64
		copy(row[:], values[i:i+len(timingColumns)])
		rows = append(rows, row)
	}
	return rows, nil
}

func parseMirrorDuration(path string) (float64, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	var first, last string
	for scanner.Scan() {
		line := scanner.Text()
		if first == "" {
			first = line
		}
		if strings.TrimSpace(line) != "" {
			last = line
		}
	}
	if scanner.Err() != nil {
		return 0, false
	}

	start, err := parseMirrorTimestamp(first)
	if err != nil {
		return 0, false
	}
	end, err := parseMirrorTimestamp(last)
	if err != nil {
		return 0, false
	}
	return end.Sub(start).Seconds(), true
}

func parseMirrorTimestamp(line string) (time.Time, error) {
	idx := strings.LastIndex(line, " at ")
	if idx == -1 {
		return time.Time{}, fmt.Errorf("missing timestamp marker")
	}
	value := strings.TrimSpace(line[idx+4:])
	return time.Parse("20060102, 150405.999999999", value)
}

type meshInfo struct {
	Ranks    int
	Elements int
	Nodes    int
	Layers   int
	Tracers  int
}

func readMeshInfo(outputs string) (meshInfo, error) {
	path := filepath.Join(outputs, "local_to_global_000000")
	file, err := os.Open(path)
	if err != nil {
		return meshInfo{}, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return meshInfo{}, err
		}
		return meshInfo{}, errors.New("empty local_to_global_000000")
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 6 {
		return meshInfo{}, fmt.Errorf("expected at least 6 fields in first line, got %d", len(fields))
	}

	elements, err := parsePositiveField(fields, 1, "elements")
	if err != nil {
		return meshInfo{}, err
	}
	nodes, err := parsePositiveField(fields, 2, "nodes")
	if err != nil {
		return meshInfo{}, err
	}
	layers, err := parsePositiveField(fields, 3, "layers")
	if err != nil {
		return meshInfo{}, err
	}
	ranks, err := parsePositiveField(fields, 4, "ranks")
	if err != nil {
		return meshInfo{}, err
	}
	tracers, err := parseNonNegativeField(fields, 5, "tracers")
	if err != nil {
		return meshInfo{}, err
	}

	return meshInfo{
		Ranks:    ranks,
		Elements: elements,
		Nodes:    nodes,
		Layers:   layers,
		Tracers:  tracers,
	}, nil
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
		return 0, fmt.Errorf("parse %s from first line: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("negative %s %d", name, value)
	}
	return value, nil
}
