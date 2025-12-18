.PHONY: build install clean

BINDIR := bin
INSTALLDIR := $(HOME)/.local/bin
CMDS := scaf scaf-lsp scaf-viz

build:
	@mkdir -p $(BINDIR)
	@for cmd in $(CMDS); do \
		echo "Building $$cmd..."; \
		go build -o $(BINDIR)/$$cmd ./cmd/$$cmd; \
	done

install: build
	@mkdir -p $(INSTALLDIR)
	@for cmd in $(CMDS); do \
		ln -sf $(CURDIR)/$(BINDIR)/$$cmd $(INSTALLDIR)/$$cmd; \
		echo "Linked $$cmd -> $(INSTALLDIR)/$$cmd"; \
	done

clean:
	rm -rf $(BINDIR)
	@for cmd in $(CMDS); do \
		rm -f $(INSTALLDIR)/$$cmd; \
	done
