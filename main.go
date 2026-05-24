package main

import (
	"bytes"
	"cmp"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"slices"
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
	// Keep this in sync with the flag definitions above; pflag.PrintDefaults
	// does not match the compact layout used by this CLI.
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

type resultColumn struct {
	name        string
	stringValue func(runTiming) string
	compare     func(runTiming, runTiming) int
	jsonValue   func(runTiming) any
	groupHead   bool
}

var resultColumnDefs = buildResultColumns()
var resultColumns = resultColumnNames(resultColumnDefs)
var resultColumnsByName = resultColumnLookup(resultColumnDefs)

func buildResultColumns() []resultColumn {
	columns := []resultColumn{
		stringColumn("identifier", func(row runTiming) string { return row.Identifier }, false),
		intColumn("ranks", func(row runTiming) int { return row.Ranks }, true),
		intColumn("elements", func(row runTiming) int { return row.Elements }, false),
		intColumn("nodes", func(row runTiming) int { return row.Nodes }, false),
		intColumn("layers", func(row runTiming) int { return row.Layers }, false),
		intColumn("tracers", func(row runTiming) int { return row.Tracers }, false),
		intColumn("dt", func(row runTiming) int { return row.DT }, true),
		floatColumn("rnday", func(row runTiming) float64 { return row.Rnday }, false),
	}
	for index, name := range timingColumns {
		index := index
		columns = append(columns, floatColumn(name, func(row runTiming) float64 {
			return row.Timings[index]
		}, index == 0))
	}
	columns = append(columns,
		floatColumn("steps_total", func(row runTiming) float64 { return row.StepsTotal }, false),
		floatColumn("init", func(row runTiming) float64 { return row.InitDuration }, true),
		floatColumn("duration", func(row runTiming) float64 { return row.Duration }, false),
	)
	return columns
}

func stringColumn(name string, value func(runTiming) string, groupHead bool) resultColumn {
	return resultColumn{
		name:        name,
		stringValue: value,
		compare: func(left runTiming, right runTiming) int {
			return strings.Compare(value(left), value(right))
		},
		jsonValue: func(row runTiming) any {
			return value(row)
		},
		groupHead: groupHead,
	}
}

func intColumn(name string, value func(runTiming) int, groupHead bool) resultColumn {
	return resultColumn{
		name: name,
		stringValue: func(row runTiming) string {
			return strconv.Itoa(value(row))
		},
		compare: func(left runTiming, right runTiming) int {
			return cmp.Compare(value(left), value(right))
		},
		jsonValue: func(row runTiming) any {
			return value(row)
		},
		groupHead: groupHead,
	}
}

func floatColumn(name string, value func(runTiming) float64, groupHead bool) resultColumn {
	return resultColumn{
		name: name,
		stringValue: func(row runTiming) string {
			return formatFloat(value(row))
		},
		compare: func(left runTiming, right runTiming) int {
			return compareFloats(value(left), value(right))
		},
		jsonValue: func(row runTiming) any {
			value := value(row)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil
			}
			return value
		},
		groupHead: groupHead,
	}
}

func resultColumnNames(columns []resultColumn) []string {
	names := make([]string, len(columns))
	for index, column := range columns {
		names[index] = column.name
	}
	return names
}

func resultColumnLookup(columns []resultColumn) map[string]resultColumn {
	lookup := make(map[string]resultColumn, len(columns))
	for _, column := range columns {
		lookup[column.name] = column
	}
	return lookup
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

	if _, ok := resultColumnsByName[spec]; !ok {
		return sortKey{}, fmt.Errorf("invalid sort column %q; valid columns: %s", spec, strings.Join(resultColumns, ", "))
	}

	key.column = spec
	return key, nil
}

func sortRows(rows []runTiming, keys []sortKey) {
	slices.SortStableFunc(rows, func(left runTiming, right runTiming) int {
		for _, key := range keys {
			result := compareRows(left, right, key.column)
			if result == 0 {
				continue
			}
			if key.descending {
				return -result
			}
			return result
		}
		return 0
	})
}

func compareRows(left runTiming, right runTiming, column string) int {
	columnDef, ok := resultColumnsByName[column]
	if !ok {
		return 0
	}
	return columnDef.compare(left, right)
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
	default:
		return cmp.Compare(left, right)
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
	return resultColumnDefs[columnIndex].groupHead
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

type jsonField struct {
	name  string
	value any
}

type jsonRow []jsonField

func (row jsonRow) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, field := range row {
		if index > 0 {
			buffer.WriteByte(',')
		}

		name, err := json.Marshal(field.name)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(field.value)
		if err != nil {
			return nil, err
		}

		buffer.Write(name)
		buffer.WriteByte(':')
		buffer.Write(value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func toJSONRows(rows []runTiming) []jsonRow {
	jsonRows := make([]jsonRow, 0, len(rows))
	for _, row := range rows {
		fields := make(jsonRow, len(resultColumnDefs))
		for index, column := range resultColumnDefs {
			fields[index] = jsonField{
				name:  column.name,
				value: column.jsonValue(row),
			}
		}
		jsonRows = append(jsonRows, fields)
	}
	return jsonRows
}

func (row runTiming) asStrings() []string {
	fields := make([]string, len(resultColumnDefs))
	for index, column := range resultColumnDefs {
		fields[index] = column.stringValue(row)
	}
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
	return fmt.Sprintf("%*s", width, value)
}

func padRight(value string, width int) string {
	return fmt.Sprintf("%-*s", width, value)
}

func hasHelpFlag(args []string) bool {
	// Scan before fs.Parse so --help still wins when another flag has a bad or
	// missing value, for example: --workers --help.
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
