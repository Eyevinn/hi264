.PHONY: all build test coverage check pre-commit codespell clean install

all: check build test

build: out/hi264dec out/hi264gen

out/hi264dec: $(shell find pkg cmd/hi264dec -name '*.go')
	go build -o $@ ./cmd/hi264dec

out/hi264gen: $(shell find pkg cmd/hi264gen -name '*.go')
	go build -o $@ ./cmd/hi264gen

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
	go install ./cmd/hi264dec
	go install ./cmd/hi264gen
