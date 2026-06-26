package main

import (
	"bytes"
	"encoding/json"
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

func tempRun(t *testing.T, name string, extraOutputFiles map[string]string) string {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), "a3d", name)
	outputs := filepath.Join(runDir, "outputs")
	if err := os.MkdirAll(outputs, 0o700); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"param.out.nml": "&CORE\n DT= 900.00000000000000,\n /\n",
		"mirror.out":    parseableMirrorOut(),
	}
	for name, content := range extraOutputFiles {
		files[name] = content
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(outputs, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return runDir
}

func parseableMirrorOut() string {
	lines := []string{
		"Run begins at 20250101, 000000.000",
		" Total # of tracers=           2",
		"Global Grid Size (ne,np,ns,nvrt):       5839      3140      8986        49",
		"",
		"**********Augmented Subdomain Sizes**********",
		" rank     nea      ne     neg",
		"    0    1141     975     166",
		"    1    1134     975     159",
		"    2    1052     975      77",
		" Max. dot product of 3 axes=   1.66533454E-16",
		" # of scribe can be set as small as:           1           2",
		"Run completed successfully at 20250101, 000300.000",
		"",
	}
	return strings.Join(lines, "\n")
}

func mirrorOutWithThreads(threads string) string {
	return strings.Replace(
		parseableMirrorOut(),
		"Run begins at 20250101, 000000.000\n",
		"Run begins at 20250101, 000000.000\n hybrid openMP-MPI run with # of threads=           "+threads+"\n",
		1,
	)
}

func mirrorOutWithoutTimestamps() string {
	lines := []string{
		"run started sometime",
		" Total # of tracers=           2",
		"Global Grid Size (ne,np,ns,nvrt):       5839      3140      8986        49",
		"",
		"**********Augmented Subdomain Sizes**********",
		" rank     nea      ne     neg",
		"    0    1141     975     166",
		"    1    1134     975     159",
		"    2    1052     975      77",
		" Max. dot product of 3 axes=   1.66533454E-16",
		" # of scribe can be set as small as:           1           2",
		"run ended later",
		"",
	}
	return strings.Join(lines, "\n")
}

func TestAnalyzeRunFromFixture(t *testing.T) {
	row, err := analyzeRun(fixtureRun("20110602.00"), "")
	if err != nil {
		t.Fatal(err)
	}

	if row.Identifier != "a3d/20110602.00" {
		t.Fatalf("identifier = %q", row.Identifier)
	}
	if row.Ranks != 3 {
		t.Fatalf("Ranks = %d", row.Ranks)
	}
	if row.Threads != 0 {
		t.Fatalf("Threads = %d", row.Threads)
	}
	if row.Scribes != 2 {
		t.Fatalf("Scribes = %d", row.Scribes)
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
	row, err := analyzeRun(fixtureRun("20110602.00")+string(filepath.Separator), "")
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

	results := analyzeRuns([]string{completeRun, incompleteRun}, nil, 0)

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
	if results[1].err == nil || !strings.Contains(results[1].err.Error(), "mirror.out") {
		t.Fatalf("expected missing mirror.out error, got %v", results[1].err)
	}
}

func TestParseMirrorOutMetadata(t *testing.T) {
	metadata, durationSec, hasDuration, err := parseMirrorOut(filepath.Join(fixtureOutputs("20110602.00"), "mirror.out"))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Ranks != 3 {
		t.Fatalf("Ranks = %d", metadata.Ranks)
	}
	if metadata.Threads != 0 {
		t.Fatalf("Threads = %d", metadata.Threads)
	}
	if metadata.Scribes != 2 {
		t.Fatalf("Scribes = %d", metadata.Scribes)
	}
	if metadata.Elements != 5839 {
		t.Fatalf("Elements = %d", metadata.Elements)
	}
	if metadata.Nodes != 3140 {
		t.Fatalf("Nodes = %d", metadata.Nodes)
	}
	if metadata.Layers != 49 {
		t.Fatalf("Layers = %d", metadata.Layers)
	}
	if metadata.Tracers != 2 {
		t.Fatalf("Tracers = %d", metadata.Tracers)
	}
	if !hasDuration {
		t.Fatal("expected duration")
	}
	assertClose(t, durationSec, 180)
}

func TestParseMirrorOutRanksFromBoundaryTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mirror.out")
	content := strings.Join([]string{
		"Run begins at 20250101, 000000.000",
		" Total # of tracers=           2",
		"Global Grid Size (ne,np,ns,nvrt):       5839      3140      8986        49",
		"**********Augmented Subdomain Boundary Sizes**********",
		"    rank    nope    neta   nland    nvel",
		"       0       0       0       7      93",
		"       1       0       0       6      96",
		"       2       0       0       5      83",
		" done domain decomp...",
		" # of scribe can be set as small as:           1           2",
		"Run completed successfully at 20250101, 000300.000",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	metadata, _, _, err := parseMirrorOut(path)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Ranks != 3 {
		t.Fatalf("Ranks = %d", metadata.Ranks)
	}
}

func TestParseMirrorOutOpenMPThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mirror.out")
	if err := os.WriteFile(path, []byte(mirrorOutWithThreads("1")), 0o600); err != nil {
		t.Fatal(err)
	}

	metadata, _, _, err := parseMirrorOut(path)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Threads != 1 {
		t.Fatalf("Threads = %d", metadata.Threads)
	}
}

func TestRunIdentifierIgnoresTrailingSlash(t *testing.T) {
	identifier := runIdentifier("/project/home/p201203/90_models/joseph_2023/schism513/20110601.00/", "")
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

func TestAnalyzeRunRequiresMirrorMetadataForPartialRows(t *testing.T) {
	_, err := analyzeRun(fixtureRun("20110601.00"), "")
	if err == nil || !strings.Contains(err.Error(), "mirror.out") {
		t.Fatalf("expected missing mirror.out error, got %v", err)
	}
}

func TestAnalyzeRunFallsBackWithoutTimingFile(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": parseableMirrorOut(),
	})

	row, err := analyzeRun(runDir, "")
	if err != nil {
		t.Fatal(err)
	}

	if row.Identifier != "a3d/20110603.00" {
		t.Fatalf("identifier = %q", row.Identifier)
	}
	if row.Ranks != 3 {
		t.Fatalf("Ranks = %d", row.Ranks)
	}
	if row.Threads != 0 {
		t.Fatalf("Threads = %d", row.Threads)
	}
	if row.Scribes != 2 {
		t.Fatalf("Scribes = %d", row.Scribes)
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
	assertNaN(t, row.Rnday)
	for index, value := range row.Timings {
		if !math.IsNaN(value) {
			t.Fatalf("Timings[%d] = %v, want NaN", index, value)
		}
	}
	assertNaN(t, row.StepsTotal)
	assertNaN(t, row.InitDuration)
	assertClose(t, row.Duration, 180.0/3600.0)
}

func TestAnalyzeRunFallsBackWithEmptyTimingFile(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out":      parseableMirrorOut(),
		"nonfatal_000000": "",
	})

	row, err := analyzeRun(runDir, "")
	if err != nil {
		t.Fatal(err)
	}

	assertNaN(t, row.Timings[0])
	assertNaN(t, row.StepsTotal)
	assertNaN(t, row.InitDuration)
	assertClose(t, row.Duration, 180.0/3600.0)
}

func TestAnalyzeRunFallsBackWithNonTimingNonFatalFile(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out":      parseableMirrorOut(),
		"nonfatal_000000": "Max. # of Kriging points = 3\nMax. error in inverting Kriging matrix = 0.0\nWarning detail = not-a-timing-value\n",
	})

	row, err := analyzeRun(runDir, "")
	if err != nil {
		t.Fatal(err)
	}

	assertNaN(t, row.Timings[0])
	assertNaN(t, row.StepsTotal)
	assertNaN(t, row.InitDuration)
	assertClose(t, row.Duration, 180.0/3600.0)
}

func TestAnalyzeRunAcceptsSchismMatrixPrepTypo(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": parseableMirrorOut(),
		"nonfatal_000000": strings.Join([]string{
			"Time (sec) taken for force prep= 1.0 1",
			"Time taken for mom advection= 2.0 1",
			"Time taken for maxtrix prep= 3.0 1",
			"Time for solver= 4.0 1",
			"Time taken for 3D vel= 5.0 1",
			"Time taken for transport= 6.0 1",
			"Time taken for outputs= 7.0 1",
			"",
		}, "\n"),
	})

	row, err := analyzeRun(runDir, "")
	if err != nil {
		t.Fatal(err)
	}

	assertClose(t, row.Rnday, 900.0/86400.0)
	assertClose(t, row.Timings[0], 1.0/(900.0/86400.0)/3600.0)
	assertClose(t, row.Timings[1], 2.0/(900.0/86400.0)/3600.0)
	assertClose(t, row.Timings[2], 3.0/(900.0/86400.0)/3600.0)
	assertClose(t, row.StepsTotal, 28.0/(900.0/86400.0)/3600.0)
	assertClose(t, row.InitDuration, 152.0/3600.0)
	assertClose(t, row.Duration, 180.0/3600.0)
}

func TestAnalyzeRunFallbackLeavesDurationUnavailableWithoutTimestamps(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": mirrorOutWithoutTimestamps(),
	})

	row, err := analyzeRun(runDir, "")
	if err != nil {
		t.Fatal(err)
	}

	assertNaN(t, row.Timings[0])
	assertNaN(t, row.StepsTotal)
	assertNaN(t, row.InitDuration)
	assertNaN(t, row.Duration)
}

func TestAnalyzeRunFallbackLeavesDurationUnavailableWithUnparseableMirror(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": mirrorOutWithoutTimestamps(),
	})

	row, err := analyzeRun(runDir, "")
	if err != nil {
		t.Fatal(err)
	}

	assertNaN(t, row.Timings[0])
	assertNaN(t, row.StepsTotal)
	assertNaN(t, row.InitDuration)
	assertNaN(t, row.Duration)
}

func TestRunWritesPartialTableWithoutWarning(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": parseableMirrorOut(),
	})

	stdout, stderr := runCLI(t, "--report-skipped", "--columns", "identifier,force_prep,steps_total,init,duration", runDir)

	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, "identifier      | force_prep  steps_total | init  duration") {
		t.Fatalf("missing partial table header, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "a3d/20110603.00 |         NA           NA |   NA    0.0500") {
		t.Fatalf("missing partial table row, stdout:\n%s", stdout)
	}
}

func TestRunWritesPartialCSVWithoutWarning(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": parseableMirrorOut(),
	})

	stdout, stderr := runCLI(t, "--csv", "--report-skipped", "--columns", "identifier,force_prep,steps_total,init,duration", runDir)

	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	want := "identifier,force_prep,steps_total,init,duration\na3d/20110603.00,NA,NA,NA,0.0500\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunWritesPartialJSONNulls(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": mirrorOutWithoutTimestamps(),
	})

	stdout, stderr := runCLI(t, "--json", "--columns", "identifier,force_prep,steps_total,init,duration", runDir)
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
	for _, column := range []string{"force_prep", "steps_total", "init", "duration"} {
		if value, ok := decoded[0][column]; !ok {
			t.Fatalf("missing %s field, stdout:\n%s", column, stdout)
		} else if value != nil {
			t.Fatalf("%s = %#v, want nil", column, value)
		}
	}
}

func TestRunWritesCSVAndWarnsForIncompleteInputs(t *testing.T) {
	incompleteRun := fixtureRun("20110601.00")
	completeRun := fixtureRun("20110602.00")

	stdout, stderr := runCLI(t, "--csv", "--workers", "2", "--report-skipped", incompleteRun, completeRun)

	if !strings.Contains(stderr, "warning: skipping "+incompleteRun) {
		t.Fatalf("missing warning, stderr:\n%s", stderr)
	}
	if !strings.Contains(stdout, "identifier,ranks,threads,scribes,elements,nodes,layers,tracers,dt,rnday") {
		t.Fatalf("missing CSV header, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "a3d/20110602.00,3,-,2,5839,3140,49,2,900,0.0417,") {
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
	if !strings.Contains(stdout, "a3d/20110602.00,3,-,2,5839,3140,49,2,900,0.0417,") {
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
	if !strings.Contains(stdout, `"threads": null`) {
		t.Fatalf("missing null threads, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"scribes": 2`) {
		t.Fatalf("missing scribes, stdout:\n%s", stdout)
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

func TestRunWritesOpenMPThreadsColumn(t *testing.T) {
	runDir := tempRun(t, "20110603.00", map[string]string{
		"mirror.out": mirrorOutWithThreads("1"),
	})

	stdout, stderr := runCLI(t, "--csv", "--columns", "identifier,threads", runDir)
	if stderr != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	want := "identifier,threads\na3d/20110603.00,1\n"
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
	if !strings.Contains(stdout, "identifier,ranks,threads,scribes,elements,nodes,layers,tracers,dt,rnday") {
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
	if !strings.Contains(stdout, "a3d/20110602.00,3,-,2,5839,3140,49,2,900,0.0417,") {
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

func assertNaN(t *testing.T, got float64) {
	t.Helper()
	if !math.IsNaN(got) {
		t.Fatalf("got %.17g, want NaN", got)
	}
}

func TestDiscoverOutputsDirectRunDir(t *testing.T) {
	runDir := tempRun(t, "20110602.00", nil)
	paths, _ := discoverOutputs([]string{runDir}, 3)
	if len(paths) != 1 || paths[0] != runDir {
		t.Fatalf("paths = %v, want [%s]", paths, runDir)
	}
}

func TestDiscoverOutputsDirectOutputsDir(t *testing.T) {
	runDir := tempRun(t, "20110602.00", nil)
	outputsDir := filepath.Join(runDir, "outputs")
	paths, _ := discoverOutputs([]string{outputsDir}, 3)
	if len(paths) != 1 || paths[0] != outputsDir {
		t.Fatalf("paths = %v, want [%s]", paths, outputsDir)
	}
}

func TestDiscoverOutputsNestedDiscovery(t *testing.T) {
	parent := t.TempDir()
	run1 := filepath.Join(parent, "run1")
	run2 := filepath.Join(parent, "run2")
	out1 := filepath.Join(run1, "outputs")
	out2 := filepath.Join(run2, "outputs")
	for _, p := range []string{out1, out2} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths, _ := discoverOutputs([]string{parent}, 3)
	if len(paths) != 2 || paths[0] != out1 || paths[1] != out2 {
		t.Fatalf("paths = %v, want [%s %s]", paths, out1, out2)
	}
}

func TestDiscoverOutputsDepthLimit(t *testing.T) {
	parent := t.TempDir()
	a := filepath.Join(parent, "aaa")
	bb := filepath.Join(a, "bbb")
	ccc := filepath.Join(bb, "ccc")
	out := filepath.Join(ccc, "outputs")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	paths, _ := discoverOutputs([]string{parent}, 3)
	if len(paths) != 0 {
		t.Fatalf("paths = %v, want []", paths)
	}
	paths, _ = discoverOutputs([]string{parent}, 4)
	if len(paths) != 1 || paths[0] != out {
		t.Fatalf("paths = %v, want [%s]", paths, out)
	}
}

func TestDiscoverOutputsSkipsHiddenDirs(t *testing.T) {
	parent := t.TempDir()
	hidden := filepath.Join(parent, ".secret")
	out := filepath.Join(hidden, "outputs")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	paths, _ := discoverOutputs([]string{parent}, 3)
	if len(paths) != 0 {
		t.Fatalf("paths = %v, want []", paths)
	}
}

func TestDiscoverOutputsSkipsZarrDirs(t *testing.T) {
	parent := t.TempDir()
	for _, name := range []string{"zarr", "archive.zarr"} {
		zarrDir := filepath.Join(parent, name)
		out := filepath.Join(zarrDir, "outputs")
		if err := os.MkdirAll(out, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths, _ := discoverOutputs([]string{parent}, 3)
	if len(paths) != 0 {
		t.Fatalf("paths = %v, want []", paths)
	}
}

func TestDiscoverOutputsDoesNotDescendIntoOutputs(t *testing.T) {
	parent := t.TempDir()
	intermediate := filepath.Join(parent, "intermediate")
	nested := filepath.Join(intermediate, "outputs", "nested", "outputs")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	paths, _ := discoverOutputs([]string{parent}, 3)
	out := filepath.Join(intermediate, "outputs")
	if len(paths) != 1 || paths[0] != out {
		t.Fatalf("paths = %v, want [%s]", paths, out)
	}
}

func TestDiscoverOutputsDeduplicates(t *testing.T) {
	parent := t.TempDir()
	runDir := filepath.Join(parent, "run")
	out := filepath.Join(runDir, "outputs")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	paths, _ := discoverOutputs([]string{parent, runDir, out}, 3)
	if len(paths) != 1 || paths[0] != out {
		t.Fatalf("paths = %v, want [%s]", paths, out)
	}
}

func TestRunHelpTextIncludesDiscoverDepth(t *testing.T) {
	stdout, _ := runCLI(t, "--help")
	if !strings.Contains(stdout, "--discover-depth") {
		t.Fatalf("missing --discover-depth in help, stdout:\n%s", stdout)
	}
}

func TestRunDiscoversRunsUnderParentDir(t *testing.T) {
	parent := t.TempDir()
	run1 := filepath.Join(parent, "a3d", "20110602.00")
	out1 := filepath.Join(run1, "outputs")
	run2 := filepath.Join(parent, "a3d", "20110601.00")
	out2 := filepath.Join(run2, "outputs")

	for _, out := range []string{out1, out2} {
		if err := os.MkdirAll(out, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "param.out.nml"), []byte("&CORE\n DT= 900,\n /\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "mirror.out"), []byte(parseableMirrorOut()), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	stdout, _ := runCLI(t, "--json", "--columns", "identifier,ranks,duration", parent)
	if !strings.Contains(stdout, `"identifier": "a3d/20110602.00"`) {
		t.Fatalf("missing discovered run, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"identifier": "a3d/20110601.00"`) {
		t.Fatalf("missing discovered run, stdout:\n%s", stdout)
	}
}

func TestRunDiscoverDepthZeroDisablesRecursion(t *testing.T) {
	parent := t.TempDir()
	run := filepath.Join(parent, "run")
	out := filepath.Join(run, "outputs")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "param.out.nml"), []byte("&CORE\n DT= 900,\n /\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCLIError(t, "--discover-depth", "0", parent)
	if err == nil || !strings.Contains(err.Error(), "no analyzable run directories") {
		t.Fatalf("expected 'no analyzable run directories' error, got %v", err)
	}
}

func TestRunIdentifierUsesRelativePathFromDiscoveryRoot(t *testing.T) {
	parent := t.TempDir()
	intermediate := filepath.Join(parent, "project")
	runDir := filepath.Join(intermediate, "a3d", "20110602.00")
	out := filepath.Join(runDir, "outputs")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "param.out.nml"), []byte("&CORE\n DT= 900,\n /\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "mirror.out"), []byte(parseableMirrorOut()), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _ := runCLI(t, "--json", "--columns", "identifier", parent)
	if !strings.Contains(stdout, `"identifier": "project/a3d/20110602.00"`) {
		t.Fatalf("expected identifier 'project/a3d/20110602.00', stdout:\n%s", stdout)
	}
}
