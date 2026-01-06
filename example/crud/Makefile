.PHONY: all build tui bootstrap test clean schema generate run fmt check

# Context-efficient test runner - only show output on failure
# Based on https://www.hlyr.dev/blog/context-efficient-backpressure

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
	@tmp=$$(mktemp); \
	if $(SCAF) generate internal/ > "$$tmp" 2>&1; then \
		printf "  ✓ scaf generate\n"; \
	else \
		printf "  ✗ scaf generate\n"; \
		cat "$$tmp"; \
		exit 1; \
	fi; \
	rm -f "$$tmp"

bootstrap:
	./cmd/bootstrap/bootstrap.sh

test:
	@tmp=$$(mktemp); \
	if $(SCAF) test internal/ --json --fail-fast > "$$tmp" 2>&1; then \
		printf "  ✓ scaf test\n"; \
	else \
		printf "  ✗ scaf test\n"; \
		jq -r 'select(.action == "failed" or .action == "error") | "\(.path): \(.short)"' "$$tmp"; \
		exit 1; \
	fi; \
	rm -f "$$tmp"

fmt:
	@tmp=$$(mktemp); \
	if $(SCAF) fmt internal/ > "$$tmp" 2>&1; then \
		printf "  ✓ scaf fmt\n"; \
	else \
		printf "  ✗ scaf fmt\n"; \
		cat "$$tmp"; \
		exit 1; \
	fi; \
	rm -f "$$tmp"

run: tui
	$(BINDIR)/crud-tui

clean:
	rm -rf $(BINDIR)

check: fmt test generate
	@echo "All checks passed"
