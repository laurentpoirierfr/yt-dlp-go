.DEFAULT_GOAL := help

BINARY := yt-dlp-go
MAIN := ./cmd/yt-dlp
GO ?= go

.PHONY: help build run test fmt vet clean install

help:
	@echo "Available targets:"
	@echo "  build       Compile the application"
	@echo "  run         Run the app from source"
	@echo "  test        Run Go tests"
	@echo "  fmt         Format Go source files"
	@echo "  vet         Run go vet"
	@echo "  clean       Remove build artifacts"
	@echo "  install     Install the binary in GOPATH/bin"

build:
	$(GO) build -o $(BINARY) $(MAIN)

run:
	$(GO) run $(MAIN)

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY)

install:
	$(GO) install $(MAIN)
