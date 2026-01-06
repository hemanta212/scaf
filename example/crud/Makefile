.PHONY: all build tui cli bootstrap test clean schema generate

BINDIR := bin
SCAF := ../../bin/scaf

all: build

build: tui cli

tui:
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/crud-tui ./cmd/crud-tui

cli:
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/crud ./cmd/crud

schema:
	$(SCAF) schema
	@echo "Generated .scaf-schema.yaml"

generate:
	$(SCAF) generate .

bootstrap:
	./bootstrap.sh

test:
	$(SCAF) test .

run-tui: tui
	NEO4J_URI=bolt://localhost:7689 $(BINDIR)/crud-tui

run-cli: cli
	@echo "Usage: $(BINDIR)/crud user list"
	@echo "       $(BINDIR)/crud comment list 1"

clean:
	rm -rf $(BINDIR)
