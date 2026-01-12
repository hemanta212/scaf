# PR: Multi-Package Support with Same-Package File Merging

## Summary

Bundle two related features:
1. **Single-file merge fix** (commit `afdd5f3`): Merge multiple `.scaf` files per directory into one output
2. **Multi-package generation**: Scan project root, generate code per package with shared config

Plus fix the **edge case wall**: Same-package `.scaf` files importing each other.

## Background

### Current State (from `afdd5f3`)

`cmd/scaf/generate.go` already groups files by output directory and merges:

```go
// Lines 225-240: Group by output dir
groups := make(map[string][]string)
for _, inputFile := range files {
    outDir := outputDir
    if outDir == "" {
        outDir = filepath.Dir(inputFile)
    }
    groups[outDir] = append(groups[outDir], inputFile)
}

// Lines 270-280: Merge suites (concatenation only)
merged.Imports = append(merged.Imports, suite.Imports...)
merged.Functions = append(merged.Functions, suite.Functions...)
```

### The Wall: Same-Package Imports

**Problem**: If `users.scaf` and `posts.scaf` are in the same package and one imports the other:

```scaf
// users.scaf
fn GetUser(id) `MATCH (u:User {id: $id}) RETURN u`

// posts.scaf  
import users "./users"  // <-- THIS IS THE PROBLEM

fn GetPostsWithAuthor(postId) `
  MATCH (p:Post {id: $postId})-[:AUTHORED_BY]->(u:User)
  RETURN p, u
`

GetPostsWithAuthor {
  setup users.GetUser($id: 1)  // Calls function from users.scaf
}
```

When merged, `users.GetUser` is already in the merged suite - the import is redundant and confusing.

**Proposed Solution**: Go-style same-package semantics:
1. Same-directory `.scaf` files implicitly share a namespace
2. Functions from sibling files can be called directly (no import needed)
3. Explicit imports only for cross-package references
4. LSP provides completion/go-to-def across same-package files

## Implementation

### Package Name Detection (stdlib ladder)

```go
func inferPackageName(dir string) (string, error) {
    // 1. go/build.ImportDir - respects build tags
    if p, err := build.Default.ImportDir(dir, 0); err == nil && p.Name != "" {
        return p.Name, nil
    }
    
    // 2. Parse any .go file's package clause (catches build-tag-hidden files)
    if name := parseAnyPackageClause(dir); name != "" {
        return name, nil
    }
    
    // 3. Sanitize folder name
    return sanitizePackageName(filepath.Base(dir)), nil
}
```

### Merge Logic Enhancement

In `generateMergedFiles()`:
- Track sibling files to detect same-package imports
- Deduplicate imports by resolved path
- Warn on intra-package imports (unnecessary)
- Error on duplicate function names across files

### Schema/Config Resolution

Walk up from package dir to find `.scaf.yaml` and `.scaf-schema.yaml` at project root.

## Phases

1. **Merge fix**: Import dedup, same-package detection, duplicate fn check
2. **Multi-package**: Directory scan, per-package generation, root config lookup
3. **LSP**: Package-aware completion/go-to-def across sibling files
4. **Analysis**: Warn on same-package imports
