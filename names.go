package scaf

// Database names.
const (
	DatabaseNeo4j    = "neo4j"
	DatabasePostgres = "postgres"
	DatabaseMySQL    = "mysql"
	DatabaseSQLite   = "sqlite"
)

// Dialect names.
const (
	DialectCypher = "cypher"
	DialectSQL    = "sql"
)

// Adapter names (for code generation).
const (
	AdapterNeogo  = "neogo"
	AdapterPgx    = "pgx"
	AdapterMySQL  = "mysql"
	AdapterSQLite = "sqlite"
)

// Language names.
const (
	LangGo = "go"
)

// DatabaseInfo maps database names to their dialect and default adapter.
type DatabaseInfo struct {
	Dialect string
	Adapter string // default adapter for Go
}

// KnownDatabases maps database names to their info.
var KnownDatabases = map[string]DatabaseInfo{
	DatabaseNeo4j:    {Dialect: DialectCypher, Adapter: AdapterNeogo},
	DatabasePostgres: {Dialect: DialectSQL, Adapter: AdapterPgx},
	DatabaseMySQL:    {Dialect: DialectSQL, Adapter: AdapterMySQL},
	DatabaseSQLite:   {Dialect: DialectSQL, Adapter: AdapterSQLite},
}

// DialectForDatabase returns the dialect name for a database.
func DialectForDatabase(dbName string) string {
	if info, ok := KnownDatabases[dbName]; ok {
		return info.Dialect
	}

	return ""
}

// AdapterForDatabase returns the default adapter name for a database and language.
func AdapterForDatabase(dbName, lang string) string {
	if lang != LangGo {
		return ""
	}

	if info, ok := KnownDatabases[dbName]; ok {
		return info.Adapter
	}

	return ""
}
