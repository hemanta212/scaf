# Reproduction: Multi-Package Support

Demonstrates two issues with multi-file scaf projects:

1. Multiple `.scaf` files in same directory overwrite output
2. Same-package imports create redundant/broken references after merge

## The Problems

### Problem 1: File Overwrite

```
internal/users/
├── queries.scaf    # defines GetUser, CreateUser
├── fixtures.scaf   # defines CreateTestUsers
└── scaf.go         # ONLY has fixtures - queries.scaf was overwritten!
```

### Problem 2: Same-Package Import

```scaf
// fixtures.scaf
fn CreateTestUsers() `CREATE (:User {id: 1})`

// queries.scaf  
import fixtures "./fixtures"  // <-- imports sibling file

fn GetUser(id: int) `MATCH (u:User {id: $id}) RETURN u`

GetUser {
    setup fixtures.CreateTestUsers()  // after merge, this is redundant
}
```

After merge, `fixtures.CreateTestUsers` is already in the same file - the import is meaningless.

## Expected Behavior

1. All `.scaf` files in a directory merge into single `scaf.go` / `scaf_test.go`
2. Imports pointing to sibling files are detected and warned
3. Functions from sibling files are directly available (no import needed)
4. Package name inferred from existing Go files or folder name

## Steps to Reproduce

```bash
cd example/repro-multi-package

# Generate - currently overwrites, only last file wins
scaf generate internal/users/

# Check output - missing functions from queries.scaf
cat internal/users/scaf.go

# Expected: both GetUser and CreateTestUsers present
# Actual: only one of them (depending on file order)
```

## Files

- `internal/users/queries.scaf` - Query functions
- `internal/users/fixtures.scaf` - Test fixture functions (imports queries)
- `.scaf.yaml` - Root config
- `.scaf-schema.yaml` - Shared schema
