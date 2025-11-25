# Scaf Test Engine Architecture

## Research Summary

### From gotestsum
- **Event-driven streaming**: Process test events as they arrive, not batched
- **EventHandler interface**: Pluggable sink for events (writes files, triggers formatters)
- **EventFormatter interface**: Pluggable presentation (dots, verbose, JSON, etc.)
- **Execution accumulator**: Mutable state that grows with each event, queryable by handlers
- **Separation of layers**: Execution → Parsing → Presentation

### From Go's testing package
- **Hierarchical execution**: Tests → Subtests form a tree
- **Semaphore-based parallelism**: Simple slot system with `maxParallel`
- **Signal channels**: Parent/child coordination via channels
- **test2json format**: Streaming JSON events with Action, Package, Test, Elapsed, Output
- **LIFO cleanup**: Setup/teardown in reverse order

## Scaf Test Hierarchy

```
Suite
├── Setup (global)
├── QueryScope (e.g., "GetUser")
│   ├── Setup (scope-level)
│   ├── Group "existing users"
│   │   ├── Setup (group-level)
│   │   ├── Test "finds Alice by id"
│   │   ├── Test "finds Bob by id"
│   │   └── Group "with posts" (nested)
│   │       └── Test "user has posts"
│   └── Group "edge cases"
│       └── Test "non-existent user"
└── QueryScope "CreatePost"
    └── Test "creates a post"
```

Maps to execution path: `GetUser/existing users/finds Alice by id`

## Core Design Decisions

### 1. Event Model

```go
type Action string

const (
    ActionRun    Action = "run"     // Test/group starting
    ActionPass   Action = "pass"    // Test passed
    ActionFail   Action = "fail"    // Test failed
    ActionSkip   Action = "skip"    // Test skipped
    ActionError  Action = "error"   // Infrastructure error (db connection, etc.)
    ActionOutput Action = "output"  // Log/debug output
    ActionSetup  Action = "setup"   // Setup block executing
)

type Event struct {
    Time     time.Time
    Action   Action
    Suite    string   // file path
    Path     []string // ["GetUser", "existing users", "finds Alice"]
    Elapsed  time.Duration
    Output   string   // for ActionOutput
    Error    error    // for ActionFail/ActionError
    Expected any      // for ActionFail - what we expected
    Actual   any      // for ActionFail - what we got
}
```

### 2. Handler Interface

```go
type Handler interface {
    // Event is called for each test event as it happens
    Event(ctx context.Context, event Event, result *Result) error
    
    // Err is called for non-test errors (stderr, infra issues)
    Err(text string) error
}
```

### 3. Result Accumulator

```go
type Result struct {
    StartTime time.Time
    EndTime   time.Time
    
    Total   int
    Passed  int
    Failed  int
    Skipped int
    Errors  int
    
    // Indexed by path string: "GetUser/existing users/finds Alice"
    Tests map[string]*TestResult
    
    // Ordered list for display
    Order []string
}

type TestResult struct {
    Path     []string
    Status   Action // pass, fail, skip, error
    Elapsed  time.Duration
    Error    error
    Expected any
    Actual   any
    Output   []string // captured output
}
```

### 4. Formatter Interface

```go
type Formatter interface {
    // Format renders an event to output
    Format(event Event, result *Result) error
    
    // Summary renders final results
    Summary(result *Result) error
}
```

Built-in formatters:
- `dots` - minimal: `.` for pass, `F` for fail, `S` for skip
- `verbose` - full test names and output
- `json` - streaming JSON events (for tooling)
- `tui` - interactive Charm-based display (future)

### 5. Runner Architecture

```go
type Runner struct {
    dialect  Dialect
    handler  Handler
    parallel int // max concurrent tests (default: 1 for DB sanity)
}

func (r *Runner) Run(ctx context.Context, suite *Suite) (*Result, error)
```

**Execution flow:**
```
1. Connect to database via Dialect
2. Execute global Setup
3. For each QueryScope:
   a. Execute scope Setup
   b. For each Test/Group (respecting parallel limit):
      - Execute group Setup (inherited)
      - Execute test Setup
      - Build params from $variables
      - Execute query via Dialect
      - Compare results to expectations
      - Execute assertions if present
      - Emit pass/fail Event
      - Run cleanup (reverse order)
   c. Cleanup scope
4. Cleanup global
5. Return Result
```

### 6. Setup/Teardown Chain

Each test inherits setup from all ancestors, executed in order:
```
Suite.Setup → QueryScope.Setup → Group.Setup → Test.Setup
```

Teardown is implicit (transaction rollback) or explicit cleanup in reverse.

**Transaction strategy** (configurable per dialect):
- Option A: Each test runs in a transaction that rolls back
- Option B: Truncate/reset between tests
- Option C: User-managed (no automatic cleanup)

### 7. Parallel Execution

Unlike `go test`, database tests are **sequential by default** because:
- Shared database state
- Transaction isolation complexity
- Connection pool limits

Future: Allow `parallel` directive in groups that are explicitly isolated.

```go
type testState struct {
    running     int
    maxParallel int
    queue       chan *testContext
    results     chan Event
}
```

### 8. Error Handling

| Error Type | Action | Behavior |
|------------|--------|----------|
| Query syntax error | `error` | Stop test, report, continue suite |
| Connection lost | `error` | Stop suite, report |
| Assertion mismatch | `fail` | Report expected vs actual, continue |
| Setup failure | `error` | Skip dependent tests |
| Timeout | `error` | Cancel test, report |

### 9. Output Capture

During test execution, capture:
- Query being executed (with params)
- Raw database response
- Diff between expected and actual

```go
type testContext struct {
    output *bytes.Buffer
    // Write query info, responses here
    // Flushed to Event.Output on completion
}
```

## Dialect vs Adapter

**Important distinction:**

### Dialect (scaf's domain)
- Powers the **test runner**
- Executes raw queries with parameters
- Returns results as `[]map[string]any` for simple value comparisons
- Provides transaction support for test isolation (rollback after each test)
- No struct mapping, no ORM features - just raw execution

### Adapter (user's application domain)  
- Powers the **user's actual application code**
- ORM features, struct mapping, fluent query builders
- Examples: neogo (Neo4j ORM), gorm (SQL ORM), raw neo4j-go-driver
- **Not scaf's concern** - users choose their own adapter for production code

The dialect is intentionally simple because tests just need to:
1. Execute setup queries
2. Run the query under test with parameters
3. Compare returned values against expectations

```
┌─────────────────────────────────────────────────────────────┐
│                    User's Application                        │
├─────────────────────────────────────────────────────────────┤
│  adapters/neogo/       adapters/gorm/                       │
│  - Struct mapping, ORM features                             │
│  - Fluent query builders                                    │
│  - Associated with a dialect                                │
├─────────────────────────────────────────────────────────────┤
│  Database Driver (neo4j-go-driver, pgx, etc.)               │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                    scaf Test Runner                          │
├─────────────────────────────────────────────────────────────┤
│  dialects/cypher/      dialects/sql/                        │
│  - Raw query execution                                      │
│  - Returns []map[string]any                                 │
│  - Transaction support for test isolation                   │
├─────────────────────────────────────────────────────────────┤
│  Database Driver (neo4j-go-driver, pgx, etc.)               │
└─────────────────────────────────────────────────────────────┘

Adapter ←──────── associated with ────────→ Dialect
neogo                                        cypher
gorm                                         sql
```

## File Structure

```
scaf/
├── runner/
│   ├── runner.go       # Runner struct, main Run() loop
│   ├── event.go        # Event, Action types
│   ├── result.go       # Result, TestResult accumulators
│   ├── handler.go      # Handler interface + multiplexer
│   ├── format.go       # Formatter interface + implementations (dots, verbose, json)
│   └── errors.go       # Sentinel errors
├── dialect.go          # Dialect interface
├── dialects/
│   └── cypher/         # Cypher dialect (uses neo4j-go-driver)
├── adapters/
│   └── neogo/          # neogo adapter (associated with cypher dialect)
└── cmd/scaf/
    ├── main.go
    ├── fmt.go          # exists
    └── test.go         # NEW: test command
```

## CLI Interface

```bash
# Run all tests in current directory
scaf test

# Run specific file
scaf test ./queries/user.scaf

# Run with specific dialect (override config)
scaf test --dialect=neo4j

# Output formats
scaf test --format=dots      # default
scaf test --format=verbose
scaf test --format=json

# Parallel execution (when safe)
scaf test --parallel=4

# Filter by path pattern
scaf test --run="GetUser/existing"

# Fail fast
scaf test --fail-fast
```

## Phase 1 Implementation Plan

1. **Event model** (`runner/event.go`)
2. **Result accumulator** (`runner/result.go`)
3. **Handler interface + basic impl** (`runner/handler.go`)
4. **Dots formatter** (`format/dots.go`)
5. **Runner skeleton** (`runner/runner.go`)
6. **Test command** (`cmd/scaf/test.go`)

Phase 2: Actual execution against dialect
Phase 3: TUI formatter
Phase 4: Parallel execution

## Execution DAG

The test tree needs to be compiled into an execution plan that handles:
1. Setup dependency chains
2. Parallel execution (when safe)
3. Failure propagation (skip dependents)
4. Cleanup ordering

### Node Types

```go
type NodeKind int

const (
    NodeSetup NodeKind = iota  // Setup block (global, scope, group, test)
    NodeTest                    // Actual test execution
    NodeCleanup                 // Implicit or explicit teardown
)

type Node struct {
    ID       string      // Unique identifier
    Kind     NodeKind
    Path     []string    // Location in tree
    Query    string      // For setup/test: the query to run
    Deps     []string    // Node IDs this depends on
    Parallel bool        // Can run concurrently with siblings
}
```

### DAG Construction

From the AST:
```
Suite
├── Setup (S0)
└── QueryScope "GetUser"
    ├── Setup (S1)
    ├── Group "existing users"
    │   ├── Setup (S2)
    │   ├── Test "finds Alice" (T1)
    │   └── Test "finds Bob" (T2)
    └── Test "direct test" (T3)
```

Produces this DAG:
```
S0 ──┬──→ S1 ──┬──→ S2 ──┬──→ T1
     │         │         └──→ T2
     │         └──→ T3
     │
     └──→ [other scopes...]
```

Dependencies:
- `S1` depends on `S0`
- `S2` depends on `S1`
- `T1` depends on `S2`
- `T2` depends on `S2`
- `T3` depends on `S1` (not S2 - different branch)

### Execution Modes

**Sequential (default):**
```
S0 → S1 → S2 → T1 → T2 → T3
```
Simple topological sort, one at a time.

**Parallel within groups:**
```
S0 → S1 → S2 → [T1, T2] → T3
            ↘    ↗
```
Tests at same level can run concurrently if:
- Marked with `parallel` directive (future)
- Using transaction isolation (each test in own tx)

### Failure Propagation

If a setup fails, all dependent nodes are skipped:
```
S0 → S1 → S2(FAIL) → T1(SKIP) → T2(SKIP)
              ↓
           T3(RUN) ← still runs, doesn't depend on S2
```

### Execution Plan Structure

```go
type Plan struct {
    Nodes  map[string]*Node
    Order  []string        // Topological order for sequential
    Levels [][]string      // Grouped by depth for parallel
}

// Build from AST
func BuildPlan(suite *Suite) *Plan

// Execute the plan
func (p *Plan) Execute(ctx context.Context, r *Runner) *Result
```

### Scheduler

```go
type Scheduler struct {
    plan      *Plan
    completed map[string]bool
    failed    map[string]bool
    mu        sync.Mutex
    sem       chan struct{}  // Parallelism limiter
}

func (s *Scheduler) Run(ctx context.Context) {
    for _, nodeID := range s.plan.Order {
        node := s.plan.Nodes[nodeID]
        
        // Check if deps satisfied
        if s.anyDepFailed(node) {
            s.skip(node)
            continue
        }
        
        // Wait for parallel slot
        s.sem <- struct{}{}
        
        go func(n *Node) {
            defer func() { <-s.sem }()
            s.execute(n)
        }(node)
    }
}
```

### Transaction Strategies

How setup/cleanup interacts with database transactions:

**Strategy A: Shared Transaction (default)**
```
BEGIN
  S0: CREATE (:User {id: 1})
  S1: CREATE (:Post {authorId: 1})
  T1: MATCH... → assert
  T2: MATCH... → assert
ROLLBACK
```
All setup and tests share one transaction, rolled back at end.
Pro: Fast, isolated from other test runs.
Con: Tests can see each other's side effects.

**Strategy B: Per-Test Transactions**
```
For each test:
  BEGIN
    Run inherited setup chain
    Run test
  ROLLBACK
```
Pro: Tests fully isolated from each other.
Con: Setup runs multiple times, slower.

**Strategy C: Savepoints**
```
BEGIN
  S0: CREATE (:User)
  SAVEPOINT scope_1
    S1: CREATE (:Post)
    SAVEPOINT group_1
      T1: MATCH... → ROLLBACK TO group_1
      T2: MATCH... → ROLLBACK TO group_1
    RELEASE SAVEPOINT group_1
  RELEASE SAVEPOINT scope_1
ROLLBACK
```
Pro: Setup runs once, tests isolated.
Con: Not all databases support savepoints well.

### Implementation Plan

1. **Plan builder** (`runner/plan.go`)
   - Walk AST, emit nodes
   - Compute dependencies
   - Topological sort

2. **Scheduler** (`runner/scheduler.go`)
   - Execute plan
   - Handle parallelism
   - Propagate failures

3. **Transaction manager** (`runner/txn.go`)
   - Strategy selection
   - Savepoint management

## Open Questions

- [ ] Should setup blocks run in same transaction as test, or separate?
- [ ] How to handle dialect-specific setup syntax validation?
- [ ] Watch mode for re-running on file changes?
- [ ] Coverage-like metrics (which queries tested, which branches)?
- [ ] How to handle flaky tests / retries?
- [ ] Test timeouts - per test or per suite?