package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureRun(name string) string {
	return filepath.Join("testdata", "runs", "a3d", name)
}

func fixtureOutputs(name string) string {
	return filepath.Join(fixtureRun(name), "outputs")
}

func runCLI(t *testing.T, args ...string) (string, string) {
	t.Helper()
	stdout, stderr, err := runCLIError(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	return stdout, stderr
}

func runCLIError(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestAnalyzeRunFromFixture(t *testing.T) {
	row, err := analyzeRun(fixtureRun("20110602.00"))
	if err != nil {
		t.Fatal(err)
	}

	if row.Identifier != "a3d/20110602.00" {
		t.Fatalf("identifier = %q", row.Identifier)
	}
	if row.Ranks != 3 {
		t.Fatalf("Ranks = %d", row.Ranks)
	}
	if row.Elements != 5839 {
		t.Fatalf("Elements = %d", row.Elements)
	}
	if row.Nodes != 3140 {
		t.Fatalf("Nodes = %d", row.Nodes)
	}
	if row.Layers != 49 {
		t.Fatalf("Layers = %d", row.Layers)
	}
	if row.Tracers != 2 {
		t.Fatalf("Tracers = %d", row.Tracers)
	}
	if row.DT != 900 {
		t.Fatalf("DT = %d", row.DT)
	}
	assertClose(t, row.Rnday, 1.0/24.0)
	assertClose(t, row.Timings[0], 10.0/150.0)
	assertClose(t, row.Timings[1], 14.0/150.0)
	assertClose(t, row.Timings[2], 18.0/150.0)
	assertClose(t, row.Timings[3], 22.0/150.0)
	assertClose(t, row.Timings[4], 26.0/150.0)
	assertClose(t, row.Timings[5], 30.0/150.0)
	assertClose(t, row.Timings[6], 34.0/150.0)
	assertClose(t, row.StepsTotal, 154.0/150.0)
	assertClose(t, row.InitDuration, 26.0/3600.0)
	assertClose(t, row.Duration, 180.0/3600.0)
}

func TestAnalyzeRunIdentifierIgnoresTrailingSlash(t *testing.T) {
	row, err := analyzeRun(fixtureRun("20110602.00") + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}

	if row.Identifier != "a3d/20110602.00" {
		t.Fatalf("identifier = %q", row.Identifier)
	}
}

func TestAnalyzeRunsPreservesInputOrderWithInvalidWorkerCount(t *testing.T) {
	completeRun := fixtureRun("20110602.00")
	incompleteRun := fixtureRun("20110601.00")

	results := analyzeRuns([]string{completeRun, incompleteRun}, 0)

	if len(results) != 2 {
		t.Fatalf("len(results) = %d", len(results))
	}
	if results[0].path != completeRun {
		t.Fatalf("results[0].path = %q", results[0].path)
	}
	if results[0].err != nil {
		t.Fatalf("results[0].err = %v", results[0].err)
	}
	if results[1].path != incompleteRun {
		t.Fatalf("results[1].path = %q", results[1].path)
	}
	if !errors.Is(results[1].err, errNoTimingSamples) {
		t.Fatalf("expected errNoTimingSamples, got %v", results[1].err)
	}
}

func TestReadMeshInfo(t *testing.T) {
	mesh, err := readMeshInfo(fixtureOutputs("20110602.00"))
	if err != nil {
		t.Fatal(err)
	}
	if mesh.Ranks != 3 {
		t.Fatalf("Ranks = %d", mesh.Ranks)
	}
	if mesh.Elements != 5839 {
		t.Fatalf("Elements = %d", mesh.Elements)
	}
	if mesh.Nodes != 3140 {
		t.Fatalf("Nodes = %d", mesh.Nodes)
	}
	if mesh.Layers != 49 {
		t.Fatalf("Layers = %d", mesh.Layers)
	}
	if mesh.Tracers != 2 {
		t.Fatalf("Tracers = %d", mesh.Tracers)
	}
}

func TestRunIdentifierIgnoresTrailingSlash(t *testing.T) {
	identifier := runIdentifier("/project/home/p201203/90_models/joseph_2023/schism513/20110601.00/")
	if identifier != "schism513/20110601.00" {
		t.Fatalf("identifier = %q", identifier)
	}
}

func TestParseParamOutNMLRequiresDTOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "param.out.nml")
	if err := os.WriteFile(path, []byte("&CORE\n DT= 900,\n /\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dt, err := parseParamOutNML(path)
	if err != nil {
		t.Fatal(err)
	}
	if dt != 900 {
		t.Fatalf("dt = %d", dt)
	}
}

func TestAnalyzeRunSkipsIncompleteTimingFile(t *testing.T) {
	_, err := analyzeRun(fixtureRun("20110601.00"))
	if !errors.Is(err, errNoTimingSamples) {
		t.Fatalf("expected errNoTimingSamples, got %v", err)
	}
}

func TestRunWritesCSVAndWarnsForIncompleteInputs(t *testing.T) {
	incompleteRun := fixtureRun("20110601.00")
	completeRun := fixtureRun("20110602.00")

	stdout, stderr := runCLI(t, "--csv", "--workers", "2", "--report-skipped", incompleteRun, completeRun)

	if !strings.Contains(stderr, "warning: skipping "+incompleteRun) {
		t.Fatalf("missing warning, stderr:\n%s", stderr)
	}
	if !strings.Contains(stdout, "identifier,ranks,elements,nodes,layers,tracers,dt,rnday") {
		t.Fatalf("missing CSV header, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "a3d/20110602.00,3,5839,3140,49,2,900,0.0417,") {
		t.Fatalf("missing analyzed row, stdout:\n%s", stdout)
	}
}

func TestRunWritesGroupedTable(t *testing.T) {
	stdout, stderr := runCLI(t, fixtureRun("20110602.00"))
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, "identifier      | ranks") {
		t.Fatalf("missing identifier separator, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "tracers |  dt") {
		t.Fatalf("missing mesh/config separator, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "rnday | force_prep") {
		t.Fatalf("missing config/timing separator, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "steps_total |") {
		t.Fatalf("missing timing/duration separator, stdout:\n%s", stdout)
	}
}

func TestRunSkipsIncompleteInputsQuietlyByDefault(t *testing.T) {
	stdout, stderr := runCLI(t, "--csv", fixtureRun("20110601.00"), fixtureRun("20110602.00"))

	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, "a3d/20110602.00,3,5839,3140,49,2,900,0.0417,") {
		t.Fatalf("missing analyzed row, stdout:\n%s", stdout)
	}
}

func TestRunWritesJSON(t *testing.T) {
	stdout, stderr := runCLI(t, "--json", fixtureRun("20110602.00"))
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, `"identifier": "a3d/20110602.00"`) {
		t.Fatalf("missing identifier, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"dt": 900`) {
		t.Fatalf("missing integer dt, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"ranks": 3`) {
		t.Fatalf("missing ranks, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"elements": 5839`) {
		t.Fatalf("missing elements, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"nodes": 3140`) {
		t.Fatalf("missing nodes, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"layers": 49`) {
		t.Fatalf("missing layers, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"tracers": 2`) {
		t.Fatalf("missing tracers, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"force_prep": 0.06666666666666667`) {
		t.Fatalf("missing full precision force_prep, stdout:\n%s", stdout)
	}
}

func TestRunWritesSelectedCSVColumns(t *testing.T) {
	stdout, stderr := runCLI(t, "--csv", "--columns", "identifier,outputs", fixtureRun("20110602.00"))
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	want := "identifier,outputs\na3d/20110602.00,0.2267\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunWritesSelectedJSONColumns(t *testing.T) {
	stdout, stderr := runCLI(t, "--json", "--columns", "identifier,outputs", fixtureRun("20110602.00"))
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("invalid JSON:\n%s\nerror: %v", stdout, err)
	}
	if len(decoded) != 1 {
		t.Fatalf("len(decoded) = %d", len(decoded))
	}
	if len(decoded[0]) != 2 {
		t.Fatalf("decoded row has %d fields, want 2: %#v", len(decoded[0]), decoded[0])
	}
	if decoded[0]["identifier"] != "a3d/20110602.00" {
		t.Fatalf("identifier = %#v", decoded[0]["identifier"])
	}
	assertClose(t, decoded[0]["outputs"].(float64), 34.0/150.0)
}

func TestRunRejectsUnknownOutputColumn(t *testing.T) {
	_, _, err := runCLIError(t, "--columns", "identifier,outpts", fixtureRun("20110602.00"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `invalid output column "outpts"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "outputs") {
		t.Fatalf("valid columns not included in error: %v", err)
	}
}

func TestWriteJSONEncodesNaNAsNull(t *testing.T) {
	var stdout bytes.Buffer
	rows := []runTiming{
		{
			Identifier:   "missing-init",
			InitDuration: math.NaN(),
		},
	}

	if err := writeJSON(&stdout, rows, resultColumnDefs); err != nil {
		t.Fatal(err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON:\n%s\nerror: %v", stdout.String(), err)
	}
	if len(decoded) != 1 {
		t.Fatalf("len(decoded) = %d", len(decoded))
	}
	if value, ok := decoded[0]["init"]; !ok {
		t.Fatalf("missing init field, stdout:\n%s", stdout.String())
	} else if value != nil {
		t.Fatalf("init = %#v, want nil", value)
	}
}

func TestRunParsesOutputFlagsAfterPositionalArgs(t *testing.T) {
	stdout, stderr := runCLI(t, fixtureRun("20110602.00"), "--csv")
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, "identifier,ranks,elements,nodes,layers,tracers,dt,rnday") {
		t.Fatalf("missing CSV header, stdout:\n%s", stdout)
	}
}

func TestRunParsesColumnsFlagAfterPositionalArgs(t *testing.T) {
	stdout, stderr := runCLI(t, fixtureRun("20110602.00"), "--csv", "--columns", "identifier,outputs")
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	want := "identifier,outputs\na3d/20110602.00,0.2267\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunParsesSortFlagAfterPositionalArgs(t *testing.T) {
	stdout, stderr := runCLI(t, fixtureRun("20110602.00"), "--csv", "--sort", "-duration,identifier")
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, "a3d/20110602.00,3,5839,3140,49,2,900,0.0417,") {
		t.Fatalf("missing analyzed row, stdout:\n%s", stdout)
	}
}

func TestRunRejectsMultipleOutputFormats(t *testing.T) {
	_, _, err := runCLIError(t, "--csv", "--json", fixtureRun("20110602.00"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "choose only one output format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsMultipleOutputFormatsAfterPositionalArgs(t *testing.T) {
	_, _, err := runCLIError(t, fixtureRun("20110602.00"), "--csv", "--json")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "choose only one output format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSortRows(t *testing.T) {
	tests := []struct {
		name string
		spec string
		rows []runTiming
		want []string
	}{
		{
			name: "identifier",
			spec: "identifier",
			rows: []runTiming{
				{Identifier: "b", Duration: 1},
				{Identifier: "a", Duration: 2},
			},
			want: []string{"a", "b"},
		},
		{
			name: "descending duration",
			spec: "-duration",
			rows: []runTiming{
				{Identifier: "a", Duration: 1},
				{Identifier: "b", Duration: 2},
			},
			want: []string{"b", "a"},
		},
		{
			name: "multiple keys",
			spec: "ranks,-duration,identifier",
			rows: []runTiming{
				{Identifier: "a", Ranks: 2, Duration: 1},
				{Identifier: "b", Ranks: 1, Duration: 1},
				{Identifier: "c", Ranks: 1, Duration: 3},
			},
			want: []string{"c", "b", "a"},
		},
		{
			name: "nodes",
			spec: "nodes",
			rows: []runTiming{
				{Identifier: "a", Nodes: 20},
				{Identifier: "b", Nodes: 10},
			},
			want: []string{"b", "a"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keys, err := parseSortKeys(test.spec)
			if err != nil {
				t.Fatal(err)
			}

			sortRows(test.rows, keys)

			got := make([]string, len(test.rows))
			for index, row := range test.rows {
				got[index] = row.Identifier
			}
			for index := range test.want {
				if got[index] != test.want[index] {
					t.Fatalf("unexpected order: got %v, want %v", got, test.want)
				}
			}
		})
	}
}

func TestParseSortKeysRejectsUnknownColumns(t *testing.T) {
	_, err := parseSortKeys("identifier,unknown")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `invalid sort column "unknown"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "duration") {
		t.Fatalf("valid columns not included in error: %v", err)
	}
}

func TestRunFailsWhenNoInputsCanBeAnalyzed(t *testing.T) {
	_, _, err := runCLIError(t, fixtureRun("20110601.00"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no analyzable run directories") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWritesHelpToStdout(t *testing.T) {
	stdout, stderr := runCLI(t, "--help")
	if !strings.Contains(stdout, "Usage: schism-timings [OPTIONS] DIR [DIR...]") {
		t.Fatalf("missing usage, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--workers int") {
		t.Fatalf("missing workers option, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--csv") {
		t.Fatalf("missing csv option, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--json") {
		t.Fatalf("missing json option, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--columns columns") {
		t.Fatalf("missing columns option, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--sort columns") {
		t.Fatalf("missing sort option, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--report-skipped") {
		t.Fatalf("missing report-skipped option, stdout:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
}

func TestRunWritesHelpToStdoutAfterPositionalArgs(t *testing.T) {
	stdout, stderr := runCLI(t, fixtureRun("20110602.00"), "--help")
	if !strings.Contains(stdout, "Usage: schism-timings [OPTIONS] DIR [DIR...]") {
		t.Fatalf("missing usage, stdout:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
}

func TestRunWritesVersion(t *testing.T) {
	stdout, stderr := runCLI(t, "--version")
	if stdout != "schism-timings dev\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
}

func TestRunWritesVersionAfterPositionalArgs(t *testing.T) {
	stdout, stderr := runCLI(t, fixtureRun("20110602.00"), "--version")
	if stdout != "schism-timings dev\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
}

func assertClose(t *testing.T, got float64, want float64) {
	t.Helper()
	const tolerance = 1e-12
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("got %.17g, want %.17g", got, want)
	}
}
