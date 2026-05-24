BINARY := schism-timings
LINUX_AMD64_BINARY := schism-timings-linux-amd64
VERSION ?= $(shell git describe --tags --dirty --always --long)

.PHONY: all build build-linux-amd64 test clean

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) .

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(LINUX_AMD64_BINARY) .

test:
	go test ./...

clean:
	rm -f $(BINARY) $(LINUX_AMD64_BINARY) $(LINUX_AMD64_BINARY).sha256
