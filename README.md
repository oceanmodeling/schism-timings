# schism-timings

Command-line tool for printing SCHISM timing summaries for run directories.

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

## Install From GitHub Releases

Release tags use semantic versions, for example `v0.1.0`.

Download the linux-amd64 executable:

```sh
curl -L -o schism-timings https://github.com/OWNER/REPO/releases/download/v0.1.0/schism-timings-linux-amd64
chmod +x schism-timings
./schism-timings --version
```

Replace `OWNER/REPO` with the GitHub repository path and `v0.1.0` with the release tag to install.
