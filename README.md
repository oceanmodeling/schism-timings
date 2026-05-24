# schism-timings

Command-line tool for printing SCHISM timing summaries for run directories.

## Install From GitHub Releases

Download the latest linux-amd64 executable:

```sh
curl -L -o schism-timings https://github.com/seareport/schism_timings/releases/latest/download/schism-timings-linux-amd64
chmod +x schism-timings
./schism-timings --version
```

For reproducible installs, replace `latest` with a specific release tag such as
`v0.1.0`.

## Examples

Print timing summaries for a set of SCHISM run directories:

```sh
schism-timings /runs/r04*/20*
```

```text
identifier         | ranks  elements     nodes  layers  tracers |  dt    rnday | force_prep  mom_advection  matrix_prep  solver  3D_vel  transport  outputs  steps_total |   init  duration
r0401/20200101.00  |  2556  21136211  11078206      33        2 | 120   1.0000 |     0.1071         0.2088       0.0447  0.0603  0.0250     0.7033   0.0604       1.2095 | 0.3314    1.5410
r0401b/20200101.00 |  2556  21136211  11078206      33        2 | 120   1.0000 |     0.1045         0.2107       0.0401  0.0483  0.0254     0.7051   0.0419       1.1761 | 0.3063    1.4824
r0403/20200131.00  |  5276  21136211  11078206      33        2 | 120  60.0000 |     0.0578         0.1096       0.0192  0.0363  0.0128     0.3718   0.0429       0.6505 | 0.3401   39.3713
r0403/20200331.00  |  5276  21136211  11078206      33        2 | 120  60.0000 |     0.0570         0.1064       0.0192  0.0398  0.0127     0.3753   0.0431       0.6535 | 0.3384   39.5478
r0404/20200101.00  |  5276  21136211  11078206      33        2 | 120  16.6333 |     0.0574         0.1088       0.0190  0.0381  0.0126     0.3760   0.0500       0.6618 |     NA  -11.0081
```

Limit output to selected columns:

```sh
schism-timings /runs/r04*/20* --csv --columns identifier,outputs,duration
```

```csv
identifier,outputs,duration
r0401/20200101.00,0.0604,1.5410
r0401b/20200101.00,0.0419,1.4824
r0403/20200131.00,0.0429,39.3713
r0403/20200331.00,0.0431,39.5478
r0404/20200101.00,0.0500,-11.0081
```

Use JSON for scripting:

```sh
schism-timings /runs/r04*/20* --json --columns identifier,outputs
```

```json
[
  {
    "identifier": "r0401/20200101.00",
    "outputs": 0.0603720115677775
  },
  {
    "identifier": "r0401b/20200101.00",
    "outputs": 0.04192608129999946
  },
  {
    "identifier": "r0403/20200131.00",
    "outputs": 0.042935252041970046
  },
  {
    "identifier": "r0403/20200331.00",
    "outputs": 0.04314985318005873
  },
  {
    "identifier": "r0404/20200101.00",
    "outputs": 0.0499941743997357
  }
]
```

## Development

This project uses `pre-commit` to run the standard Go checks before commits:
`gofmt`, `go mod tidy`, `go vet ./...`, and `go test ./...`.

Install the Git hook once per clone:

```sh
pre-commit install
```

Run the hooks manually:

```sh
pre-commit run --all-files
```

## SCHISM Timing Requirements

`schism-timings` reads the per-step timing lines from `outputs/nonfatal_000000`,
such as `Time (sec) taken for force prep=` and `Time taken for transport=`.
In SCHISM, those lines are emitted by the `TIMER2` instrumentation in
`src/Hydro/schism_step.F90`.

Build SCHISM with `TIMER2` enabled before analyzing runs with this tool. For a
CMake build, set:

```cmake
set(TIMER2 ON CACHE BOOLEAN "Print timing information")
```

For the legacy make build, enable the corresponding module option:

```make
USE_TIMER2 = yes
EXEC := $(EXEC)_TIMER2
```

`INCLUDE_TIMING` / `USE_TIMER` is separate SCHISM instrumentation and does not
produce the `nonfatal_000000` timing lines parsed by this CLI.
