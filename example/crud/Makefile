.PHONY: all build tui bootstrap test clean schema generate run

BINDIR := bin
SCAF := ../../bin/scaf

all: build

build: tui

tui:
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/crud-tui ./cmd/tui

schema:
	go run ./cmd/schema > .scaf-schema.yaml
	cp .scaf-schema.yaml internal/
	@echo "Generated .scaf-schema.yaml"

generate:
	$(SCAF) generate internal/

bootstrap:
	./cmd/bootstrap/bootstrap.sh

test:
	$(SCAF) test internal/

run: tui
	$(BINDIR)/crud-tui

clean:
	rm -rf $(BINDIR)
