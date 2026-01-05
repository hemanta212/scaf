.PHONY: all build tui cli bootstrap test clean schema

BINDIR := bin
SCAF_ROOT := ../..

all: build

build: tui cli

tui:
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/crud-tui ./cmd/crud-tui

cli:
	@mkdir -p $(BINDIR)
	go build -mod=mod -o $(BINDIR)/crud ./cmd/crud

schema:
	go run github.com/rlch/scaf/databases/neo4j/cmd/schema > .scaf-schema.yaml
	@echo "Generated .scaf-schema.yaml"

bootstrap:
	./bootstrap.sh

test:
	$(SCAF_ROOT)/bin/scaf test .

run-tui: tui
	NEO4J_URI=bolt://localhost:7689 $(BINDIR)/crud-tui

run-cli: cli
	@echo "Usage: $(BINDIR)/crud user list"
	@echo "       $(BINDIR)/crud comment list 1"

clean:
	rm -rf $(BINDIR)
