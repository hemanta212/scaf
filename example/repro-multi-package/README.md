# Reproduction: Multi-Package Support

This example demonstrates the multi-package support feature:

1. Multiple `.scaf` files in same directory are merged into single output
2. Same-package imports are detected and warned about
3. Config is discovered by walking up from cwd

## Directory Structure

```
internal/users/
  queries.scaf    # defines GetUser, GetAllUsers + tests
  fixtures.scaf   # defines CreateTestUsers, CleanupUsers
  scaf.go         # MERGED: all functions from both files
  scaf_test.go    # MERGED: test mocks from both files
```

## Same-Package Import Warning

When `fixtures.scaf` imports `./queries` (a sibling file), the merge emits a warning:

```
7:1: warning: import "./queries" resolves to sibling file in same package
```

This tells the user the import is redundant after merge - functions from `queries.scaf`
are already available directly.

## Running

```bash
cd example/repro-multi-package

# Run from project root - config discovered automatically
scaf generate

# Output:
# 7:1: warning: import "./queries" resolves to sibling file in same package
# wrote internal/users/scaf.go
# wrote internal/users/scaf_test.go
```

## Verify Merge

```bash
# Check that scaf.go has functions from BOTH files
grep "^func " internal/users/scaf.go
# CreateTestUsers
# CleanupUsers
# GetUser
# GetAllUsers
```

## Files

- `internal/users/queries.scaf` - Query functions with tests
- `internal/users/fixtures.scaf` - Test fixture functions (demonstrates same-package import warning)
- `.scaf.yaml` - Root config (dialect, adapter settings)
- `.scaf-schema.yaml` - Shared schema for type inference
