.PHONY: all build test coverage check pre-commit codespell clean install

LDFLAGS = -X github.com/Eyevinn/hi264/internal.commitVersion=$$(git describe --tags HEAD 2>/dev/null || echo dev-$$(git rev-parse --short HEAD)) \
          -X github.com/Eyevinn/hi264/internal.commitDate=$$(git log -1 --format=%ct)

all: check build test

build: out/hi264dec out/hi264gen

out/hi264dec: $(shell find pkg cmd/hi264dec internal -name '*.go')
	go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/hi264dec

out/hi264gen: $(shell find pkg cmd/hi264gen internal -name '*.go')
	go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/hi264gen

test:
	go test ./...

coverage:
	go test -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out -o coverage.txt
	@echo "Coverage report: coverage.html"

check:
	golangci-lint run

pre-commit: venv/bin/pre-commit
	venv/bin/pre-commit run --all-files

venv/bin/pre-commit:
	python3 -m venv venv
	venv/bin/pip install pre-commit

codespell: venv/bin/codespell
	venv/bin/codespell

venv/bin/codespell:
	python3 -m venv venv
	venv/bin/pip install codespell

clean:
	rm -rf out/ coverage.out coverage.html coverage.txt venv/

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/hi264dec
	go install -ldflags "$(LDFLAGS)" ./cmd/hi264gen
