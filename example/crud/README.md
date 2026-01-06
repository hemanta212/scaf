# CRUD Example

Users/Posts/Comments CRUD example demonstrating scaf's capabilities.

## Prerequisites

Start Neo4j using docker-compose:

```bash
docker compose up -d
```

This starts Neo4j on port 7696 (browser at 7478). Connection details are in `.scaf.yaml`.

## Quick Start

```bash
# From scaf root directory
cd example/crud

# Bootstrap sample data (for schema extraction)
./bootstrap.sh

# Extract schema from database
make schema

# Run tests
scaf test .

# Generate Go code (requires neogo adapter fix)
scaf generate .
```

## Files

- `.scaf.yaml` - Database connection config
- `.scaf-schema.yaml` - Type schema for code generation
- `users.scaf` - User CRUD queries and tests
- `posts.scaf` - Post CRUD queries and tests
- `comments.scaf` - Comment CRUD queries and tests
- `shared/fixtures.scaf` - Shared setup fixtures

## Schema

```
(User)-[:AUTHORED]->(Post)
(User)-[:WROTE]->(Comment)
(Comment)-[:ON_POST]->(Post)
```

## CLI Usage

```bash
# Extract schema from live database
scaf schema                    # outputs .scaf-schema.yaml
scaf schema -o custom.yaml     # custom output file

# Run tests
scaf test .                    # run all tests
scaf test users.scaf           # run specific file
scaf test --run "GetUserById"  # filter by pattern
scaf test -v .                 # verbose output
scaf test --json .             # JSON output for CI

# Generate code
scaf generate .                # generate Go code
scaf fmt .                     # format scaf files
```

## Fixture Pattern

Tests use layered fixtures via imports:

```scaf
import fixtures "./shared/fixtures"

setup {
    fixtures                    // runs module's setup (DETACH DELETE)
    fixtures.CreateUsers()      // creates users
    fixtures.CreatePosts()      // creates posts (MATCHes users)
}
```

This allows testing at each layer independently.
