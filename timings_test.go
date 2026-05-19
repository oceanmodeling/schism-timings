package main

import (
	"bytes"
	"errors"
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

func TestAnalyzeRunSkipsIncompleteTimingFile(t *testing.T) {
	_, err := analyzeRun(fixtureRun("20110601.00"))
	if !errors.Is(err, errNoTimingSamples) {
		t.Fatalf("expected errNoTimingSamples, got %v", err)
	}
}

func TestRunWritesCSVAndWarnsForIncompleteInputs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	incompleteRun := fixtureRun("20110601.00")
	completeRun := fixtureRun("20110602.00")

	err := run(
		[]string{"--csv", "--workers", "2", "--report-skipped", incompleteRun, completeRun},
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stderr.String(), "warning: skipping "+incompleteRun) {
		t.Fatalf("missing warning, stderr:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "identifier,ranks,elements,nodes,layers,tracers,dt,rnday") {
		t.Fatalf("missing CSV header, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "a3d/20110602.00,3,5839,3140,49,2,900,0.0417,") {
		t.Fatalf("missing analyzed row, stdout:\n%s", stdout.String())
	}
}

func TestRunWritesGroupedTable(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{fixtureRun("20110602.00")}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "identifier      | ranks") {
		t.Fatalf("missing identifier separator, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "tracers |  dt") {
		t.Fatalf("missing mesh/config separator, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "rnday | force_prep") {
		t.Fatalf("missing config/timing separator, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "steps_total |") {
		t.Fatalf("missing timing/duration separator, stdout:\n%s", stdout.String())
	}
}

func TestRunSkipsIncompleteInputsQuietlyByDefault(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(
		[]string{"--csv", fixtureRun("20110601.00"), fixtureRun("20110602.00")},
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "a3d/20110602.00,3,5839,3140,49,2,900,0.0417,") {
		t.Fatalf("missing analyzed row, stdout:\n%s", stdout.String())
	}
}

func TestRunWritesJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", fixtureRun("20110602.00")}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"identifier": "a3d/20110602.00"`) {
		t.Fatalf("missing identifier, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"dt": 900`) {
		t.Fatalf("missing integer dt, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ranks": 3`) {
		t.Fatalf("missing ranks, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"elements": 5839`) {
		t.Fatalf("missing elements, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"nodes": 3140`) {
		t.Fatalf("missing nodes, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"layers": 49`) {
		t.Fatalf("missing layers, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"tracers": 2`) {
		t.Fatalf("missing tracers, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"force_prep": 0.06666666666666667`) {
		t.Fatalf("missing full precision force_prep, stdout:\n%s", stdout.String())
	}
}

func TestRunParsesOutputFlagsAfterPositionalArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{fixtureRun("20110602.00"), "--csv"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "identifier,ranks,elements,nodes,layers,tracers,dt,rnday") {
		t.Fatalf("missing CSV header, stdout:\n%s", stdout.String())
	}
}

func TestRunParsesSortFlagAfterPositionalArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{fixtureRun("20110602.00"), "--csv", "--sort", "-duration,identifier"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "a3d/20110602.00,3,5839,3140,49,2,900,0.0417,") {
		t.Fatalf("missing analyzed row, stdout:\n%s", stdout.String())
	}
}

func TestRunRejectsMultipleOutputFormats(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--csv", "--json", fixtureRun("20110602.00")}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "choose only one output format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsMultipleOutputFormatsAfterPositionalArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{fixtureRun("20110602.00"), "--csv", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "choose only one output format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSortRowsByIdentifier(t *testing.T) {
	rows := []runTiming{
		{Identifier: "b", Duration: 1},
		{Identifier: "a", Duration: 2},
	}
	keys, err := parseSortKeys("identifier")
	if err != nil {
		t.Fatal(err)
	}

	sortRows(rows, keys)

	if rows[0].Identifier != "a" || rows[1].Identifier != "b" {
		t.Fatalf("unexpected order: %s, %s", rows[0].Identifier, rows[1].Identifier)
	}
}

func TestSortRowsByDescendingDuration(t *testing.T) {
	rows := []runTiming{
		{Identifier: "a", Duration: 1},
		{Identifier: "b", Duration: 2},
	}
	keys, err := parseSortKeys("-duration")
	if err != nil {
		t.Fatal(err)
	}

	sortRows(rows, keys)

	if rows[0].Identifier != "b" || rows[1].Identifier != "a" {
		t.Fatalf("unexpected order: %s, %s", rows[0].Identifier, rows[1].Identifier)
	}
}

func TestSortRowsByMultipleKeys(t *testing.T) {
	rows := []runTiming{
		{Identifier: "a", Ranks: 2, Duration: 1},
		{Identifier: "b", Ranks: 1, Duration: 1},
		{Identifier: "c", Ranks: 1, Duration: 3},
	}
	keys, err := parseSortKeys("ranks,-duration,identifier")
	if err != nil {
		t.Fatal(err)
	}

	sortRows(rows, keys)

	got := []string{rows[0].Identifier, rows[1].Identifier, rows[2].Identifier}
	want := []string{"c", "b", "a"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected order: got %v, want %v", got, want)
		}
	}
}

func TestSortRowsByNodes(t *testing.T) {
	rows := []runTiming{
		{Identifier: "a", Nodes: 20},
		{Identifier: "b", Nodes: 10},
	}
	keys, err := parseSortKeys("nodes")
	if err != nil {
		t.Fatal(err)
	}

	sortRows(rows, keys)

	if rows[0].Identifier != "b" || rows[1].Identifier != "a" {
		t.Fatalf("unexpected order: %s, %s", rows[0].Identifier, rows[1].Identifier)
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
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{fixtureRun("20110601.00")}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no analyzable run directories") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWritesHelpToStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Usage: schism-timings [OPTIONS] DIR [DIR...]") {
		t.Fatalf("missing usage, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--workers int") {
		t.Fatalf("missing workers option, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--csv") {
		t.Fatalf("missing csv option, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--json") {
		t.Fatalf("missing json option, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--sort columns") {
		t.Fatalf("missing sort option, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--report-skipped") {
		t.Fatalf("missing report-skipped option, stdout:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
}

func TestRunWritesHelpToStdoutAfterPositionalArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{fixtureRun("20110602.00"), "--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Usage: schism-timings [OPTIONS] DIR [DIR...]") {
		t.Fatalf("missing usage, stdout:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
}

func TestRunWritesVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--version"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "schism-timings dev\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
	}
}

func TestRunWritesVersionAfterPositionalArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{fixtureRun("20110602.00"), "--version"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "schism-timings dev\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got:\n%s", stderr.String())
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
