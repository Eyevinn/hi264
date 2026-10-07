.PHONY: all build test coverage check pre-commit pre-commit-install codespell clean install

CMDS = hi264dec hi264gen
BINARIES = $(addprefix out/,$(CMDS))

all: check build test

build: $(BINARIES)

# Binaries are built as packages, not as main.go files, so that they carry the
# version Go embeds from the git tag and commit (see internal/buildinfo.go).
# They are .PHONY because that version is not a file prerequisite: a binary
# built before a commit or a tag would be kept, still naming the old one. The
# build cache makes the rebuild cheap.
.PHONY: $(BINARIES)
$(BINARIES): out/%:
	go build -o $@ ./cmd/$*

test:
	go test ./...

coverage:
	go test -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out -o coverage.txt
	@echo "Coverage report: coverage.html"

check:
	golangci-lint run

pre-commit-install: venv/bin/pre-commit
	venv/bin/pre-commit install

pre-commit: venv/bin/pre-commit
	venv/bin/pre-commit run --all-files

venv/bin/pre-commit venv/bin/codespell:
	python3 -m venv venv
	venv/bin/pip install pre-commit codespell

codespell: venv/bin/codespell
	venv/bin/codespell -S venv,coverage.html,'*.y4m','*.264','*.mp4' -L ue,trun,truns

clean:
	rm -rf out/ coverage.out coverage.html coverage.txt venv/

install:
	go install $(addprefix ./cmd/,$(CMDS))
