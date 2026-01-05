module github.com/rlch/scaf/example

go 1.25.1

require (
	github.com/neo4j/neo4j-go-driver/v5 v5.28.4
	github.com/rlch/neogo v0.0.0-20251222040623-d3268222ee8e
	github.com/rlch/scaf v0.0.0
	github.com/urfave/cli/v3 v3.6.1
)

require (
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/expr-lang/expr v1.17.6 // indirect
	github.com/oklog/ulid/v2 v2.1.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/rlch/scaf => ..

replace github.com/alecthomas/participle/v2 => github.com/rlch/participle/v2 v2.1.5-0.20251126160008-edf31da19af2
