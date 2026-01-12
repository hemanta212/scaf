## Goals overview

- We need package-level code generation for scaf.
- Each package will have one scaf.go/scaf_test.go file.
- The go file will use the pkg name inferred by hueristics below.
- Currently, multiple `.scaf` files in same directory overwrite output, we need to merge these `scaf` files first and generate scaf.go and scaf_test.go
- However if same-package scaf files import each other, it creates redundant imports after merge.

## Solution

Go-style package semantics: same-directory `.scaf` files share namespace implicitly. Cross-package imports only. Package names inferred via `go/build.ImportDir` with fallback ladder as below;

## Package Detection

```go
// 1. go/build.ImportDir (respects build tags)
// 2. parser.PackageClauseOnly on any .go file
// 3. sanitize(filepath.Base(dir))
```

## Changes

- `cmd/scaf/generate.go`: Merge files per-dir, dedupe imports, detect/warn same-package imports, error on duplicate fns
- `cmd/scaf/package.go` (new): `inferPackageName()` using `build.ImportDir` -> `parser.PackageClauseOnly` -> folder name
- `cmd/scaf/config.go`: Walk up to find root `.scaf.yaml` / `.scaf-schema.yaml`
- `analysis/rules.go`: Warn on unnecessary same-package imports
- `lsp/server.go`, `lsp/completion.go`: Package-aware context for cross-file completion?
