package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/pflag"
)

const appName = "schism-timings"

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	fs := pflag.NewFlagSet(appName, pflag.ContinueOnError)
	fs.SetInterspersed(true)
	fs.SetOutput(stderr)

	csvOutput := fs.Bool("csv", false, "write CSV output")
	jsonOutput := fs.Bool("json", false, "write JSON output")
	sortSpec := fs.String("sort", "identifier", "comma-separated sort columns; prefix with - for descending")
	workers := fs.Int("workers", defaultWorkers(), "number of run directories to analyze concurrently")
	reportSkipped := fs.Bool("report-skipped", false, "report input directories that cannot be analyzed")
	showVersion := fs.Bool("version", false, "print version and exit")
	showHelp := fs.BoolP("help", "h", false, "show help and exit")
	writeUsage := func(format string, args ...any) {
		_, _ = fmt.Fprintf(fs.Output(), format, args...)
	}
	fs.Usage = func() {
		writeUsage("Usage: %s [OPTIONS] DIR [DIR...]\n\n", appName)
		writeUsage("Print SCHISM timing summaries for run directories.\n")
		writeUsage("\nOptions:\n")
		writeUsage("  --csv\n")
		writeUsage("    \twrite CSV output\n")
		writeUsage("  --json\n")
		writeUsage("    \twrite JSON output\n")
		writeUsage("  --sort columns\n")
		writeUsage("    \tcomma-separated sort columns; prefix with - for descending (default \"identifier\")\n")
		writeUsage("  --workers int\n")
		writeUsage("    \tnumber of run directories to analyze concurrently (default %d)\n", defaultWorkers())
		writeUsage("  --report-skipped\n")
		writeUsage("    \treport input directories that cannot be analyzed\n")
		writeUsage("  --version\n")
		writeUsage("    \tprint version and exit\n")
		writeUsage("  -h, --help\n")
		writeUsage("    \tshow help and exit\n")
	}
	if hasHelpFlag(args) {
		fs.SetOutput(stdout)
		fs.Usage()
		return nil
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showHelp {
		fs.SetOutput(stdout)
		fs.Usage()
		return nil
	}
	if *showVersion {
		_, err := fmt.Fprintf(stdout, "%s %s\n", appName, version)
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("missing DIR argument")
	}
	if *workers < 1 {
		return errors.New("workers must be at least 1")
	}
	if *csvOutput && *jsonOutput {
		return errors.New("choose only one output format: --csv or --json")
	}
	sortKeys, err := parseSortKeys(*sortSpec)
	if err != nil {
		return err
	}

	var rows []runTiming
	for _, result := range analyzeRuns(fs.Args(), *workers) {
		if result.err != nil {
			if *reportSkipped {
				if _, err := fmt.Fprintf(stderr, "warning: skipping %s: %v\n", result.path, result.err); err != nil {
					return err
				}
			}
			continue
		}
		rows = append(rows, result.row)
	}

	if len(rows) == 0 {
		return errors.New("no analyzable run directories")
	}

	sortRows(rows, sortKeys)

	switch {
	case *csvOutput:
		return writeCSV(stdout, rows)
	case *jsonOutput:
		return writeJSON(stdout, rows)
	default:
		return writeTable(stdout, rows)
	}
}

type runResult struct {
	path string
	row  runTiming
	err  error
}

func analyzeRuns(paths []string, workers int) []runResult {
	if len(paths) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(paths) {
		workers = len(paths)
	}

	work := make(chan int)
	results := make([]runResult, len(paths))
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for index := range work {
				path := paths[index]
				row, err := analyzeRun(path)
				results[index] = runResult{
					path: path,
					row:  row,
					err:  err,
				}
			}
		}()
	}

	for index := range paths {
		work <- index
	}
	close(work)
	wg.Wait()
	return results
}

type sortKey struct {
	column     string
	descending bool
}

func parseSortKeys(spec string) ([]sortKey, error) {
	parts := strings.Split(spec, ",")
	keys := make([]sortKey, 0, len(parts))
	for _, part := range parts {
		key, err := parseSortKey(part)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func parseSortKey(spec string) (sortKey, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return sortKey{}, fmt.Errorf("empty sort column; valid columns: %s", strings.Join(resultColumns, ", "))
	}

	key := sortKey{}
	switch spec[0] {
	case '-':
		key.descending = true
		spec = strings.TrimSpace(spec[1:])
	case '+':
		spec = strings.TrimSpace(spec[1:])
	}

	if !isResultColumn(spec) {
		return sortKey{}, fmt.Errorf("invalid sort column %q; valid columns: %s", spec, strings.Join(resultColumns, ", "))
	}

	key.column = spec
	return key, nil
}

func isResultColumn(column string) bool {
	for _, resultColumn := range resultColumns {
		if column == resultColumn {
			return true
		}
	}
	return false
}

func sortRows(rows []runTiming, keys []sortKey) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, key := range keys {
			result := compareRows(rows[i], rows[j], key.column)
			if result == 0 {
				continue
			}
			if key.descending {
				return result > 0
			}
			return result < 0
		}
		return false
	})
}

func compareRows(left runTiming, right runTiming, column string) int {
	switch column {
	case "identifier":
		return strings.Compare(left.Identifier, right.Identifier)
	case "ranks":
		return compareInts(left.Ranks, right.Ranks)
	case "elements":
		return compareInts(left.Elements, right.Elements)
	case "nodes":
		return compareInts(left.Nodes, right.Nodes)
	case "layers":
		return compareInts(left.Layers, right.Layers)
	case "tracers":
		return compareInts(left.Tracers, right.Tracers)
	case "dt":
		return compareInts(left.DT, right.DT)
	case "rnday":
		return compareFloats(left.Rnday, right.Rnday)
	case "force_prep":
		return compareFloats(left.Timings[0], right.Timings[0])
	case "mom_advection":
		return compareFloats(left.Timings[1], right.Timings[1])
	case "matrix_prep":
		return compareFloats(left.Timings[2], right.Timings[2])
	case "solver":
		return compareFloats(left.Timings[3], right.Timings[3])
	case "3D_vel":
		return compareFloats(left.Timings[4], right.Timings[4])
	case "transport":
		return compareFloats(left.Timings[5], right.Timings[5])
	case "outputs":
		return compareFloats(left.Timings[6], right.Timings[6])
	case "steps_total":
		return compareFloats(left.StepsTotal, right.StepsTotal)
	case "init":
		return compareFloats(left.InitDuration, right.InitDuration)
	case "duration":
		return compareFloats(left.Duration, right.Duration)
	default:
		return 0
	}
}

func compareInts(left int, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareFloats(left float64, right float64) int {
	leftNaN := math.IsNaN(left)
	rightNaN := math.IsNaN(right)
	switch {
	case leftNaN && rightNaN:
		return 0
	case leftNaN:
		return 1
	case rightNaN:
		return -1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func writeTable(writer io.Writer, rows []runTiming) error {
	tableRows := make([][]string, 0, len(rows)+1)
	tableRows = append(tableRows, resultColumns)
	for _, row := range rows {
		tableRows = append(tableRows, row.asStrings())
	}

	widths := columnWidths(tableRows)
	for _, fields := range tableRows {
		for columnIndex, field := range fields {
			if columnIndex > 0 {
				separator := "  "
				if hasTableGroupSeparatorBefore(columnIndex) {
					separator = " | "
				}
				if _, err := fmt.Fprint(writer, separator); err != nil {
					return err
				}
			}
			if columnIndex == 0 {
				if _, err := fmt.Fprint(writer, padRight(field, widths[columnIndex])); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprint(writer, padLeft(field, widths[columnIndex])); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(writer); err != nil {
			return err
		}
	}
	return nil
}

func hasTableGroupSeparatorBefore(columnIndex int) bool {
	switch resultColumns[columnIndex] {
	case "ranks", "dt", "force_prep", "init":
		return true
	default:
		return false
	}
}

func writeCSV(writer io.Writer, rows []runTiming) error {
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write(resultColumns); err != nil {
		return err
	}
	for _, row := range rows {
		if err := csvWriter.Write(row.asStrings()); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func writeJSON(writer io.Writer, rows []runTiming) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(toJSONRows(rows))
}

type jsonRow struct {
	Identifier   string  `json:"identifier"`
	Ranks        int     `json:"ranks"`
	Elements     int     `json:"elements"`
	Nodes        int     `json:"nodes"`
	Layers       int     `json:"layers"`
	Tracers      int     `json:"tracers"`
	DT           int     `json:"dt"`
	Rnday        float64 `json:"rnday"`
	ForcePrep    float64 `json:"force_prep"`
	MomAdvection float64 `json:"mom_advection"`
	MatrixPrep   float64 `json:"matrix_prep"`
	Solver       float64 `json:"solver"`
	Velocity3D   float64 `json:"3D_vel"`
	Transport    float64 `json:"transport"`
	Outputs      float64 `json:"outputs"`
	StepsTotal   float64 `json:"steps_total"`
	Init         float64 `json:"init"`
	Duration     float64 `json:"duration"`
}

func toJSONRows(rows []runTiming) []jsonRow {
	jsonRows := make([]jsonRow, 0, len(rows))
	for _, row := range rows {
		jsonRows = append(jsonRows, jsonRow{
			Identifier:   row.Identifier,
			Ranks:        row.Ranks,
			Elements:     row.Elements,
			Nodes:        row.Nodes,
			Layers:       row.Layers,
			Tracers:      row.Tracers,
			DT:           row.DT,
			Rnday:        row.Rnday,
			ForcePrep:    row.Timings[0],
			MomAdvection: row.Timings[1],
			MatrixPrep:   row.Timings[2],
			Solver:       row.Timings[3],
			Velocity3D:   row.Timings[4],
			Transport:    row.Timings[5],
			Outputs:      row.Timings[6],
			StepsTotal:   row.StepsTotal,
			Init:         row.InitDuration,
			Duration:     row.Duration,
		})
	}
	return jsonRows
}

func (row runTiming) asStrings() []string {
	fields := []string{
		row.Identifier,
		strconv.Itoa(row.Ranks),
		strconv.Itoa(row.Elements),
		strconv.Itoa(row.Nodes),
		strconv.Itoa(row.Layers),
		strconv.Itoa(row.Tracers),
		strconv.Itoa(row.DT),
		formatFloat(row.Rnday),
	}
	for _, value := range row.Timings {
		fields = append(fields, formatFloat(value))
	}
	fields = append(fields,
		formatFloat(row.StepsTotal),
		formatFloat(row.InitDuration),
		formatFloat(row.Duration),
	)
	return fields
}

func formatFloat(value float64) string {
	if math.IsNaN(value) {
		return "NA"
	}
	return strconv.FormatFloat(value, 'f', 4, 64)
}

func columnWidths(rows [][]string) []int {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for index, field := range row {
			if len(field) > widths[index] {
				widths[index] = len(field)
			}
		}
	}
	return widths
}

func padLeft(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return strings.Repeat(" ", width-len(value)) + value
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		switch arg {
		case "-h", "--help", "-help":
			return true
		}
	}
	return false
}

func defaultWorkers() int {
	workers := runtime.NumCPU()
	if workers < 4 {
		return 4
	}
	if workers > 32 {
		return 32
	}
	return workers
}
