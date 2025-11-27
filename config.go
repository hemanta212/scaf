package scaf

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the .scaf.yaml configuration file.
type Config struct {
	// Database-specific configurations (new style)
	// Only one should be set. The presence of a database config determines
	// which database to use and implies the dialect.
	Neo4j    *Neo4jConfig    `yaml:"neo4j,omitempty"`
	Postgres *PostgresConfig `yaml:"postgres,omitempty"`

	// Generate config for code generation
	Generate GenerateConfig `yaml:"generate,omitempty"`

	// ==========================================================================
	// DEPRECATED: Legacy fields for backwards compatibility
	// ==========================================================================

	// Dialect is deprecated. Use database-specific config instead.
	// e.g., use "neo4j:" instead of "dialect: cypher"
	Dialect string `yaml:"dialect,omitempty"`

	// Connection is deprecated. Use database-specific config instead.
	Connection DialectConfig `yaml:"connection,omitempty"`

	// Files is deprecated. Use cascading configs instead.
	Files map[string]string `yaml:"files,omitempty"`
}

// Neo4jConfig holds Neo4j connection settings.
type Neo4jConfig struct {
	URI      string `yaml:"uri"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
	Database string `yaml:"database,omitempty"`
}

// PostgresConfig holds PostgreSQL connection settings.
type PostgresConfig struct {
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Database string `yaml:"database,omitempty"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
	SSLMode  string `yaml:"sslmode,omitempty"`
	// Alternative: connection string
	URI string `yaml:"uri,omitempty"`
}

// DatabaseName returns the configured database name, or empty if none.
func (c *Config) DatabaseName() string {
	switch {
	case c.Neo4j != nil:
		return DatabaseNeo4j
	case c.Postgres != nil:
		return DatabasePostgres
	default:
		return ""
	}
}

// DialectName returns the dialect name based on configuration.
// New style configs (neo4j:, postgres:) take precedence over legacy dialect field.
func (c *Config) DialectName() string {
	if dbName := c.DatabaseName(); dbName != "" {
		return DialectForDatabase(dbName)
	}

	return c.Dialect
}

// ToLegacyDialectConfig converts the config to a legacy DialectConfig.
// This is for backwards compatibility during migration.
func (c *Config) ToLegacyDialectConfig() DialectConfig {
	switch {
	case c.Neo4j != nil:
		cfg := DialectConfig{
			URI:      c.Neo4j.URI,
			Username: c.Neo4j.Username,
			Password: c.Neo4j.Password,
		}
		if c.Neo4j.Database != "" {
			cfg.Options = map[string]any{"database": c.Neo4j.Database}
		}

		return cfg
	case c.Postgres != nil:
		// Convert to URI if not already set
		uri := c.Postgres.URI
		if uri == "" && c.Postgres.Host != "" {
			// Build connection string (simplified)
			uri = "postgres://" + c.Postgres.Host
		}

		return DialectConfig{
			URI:      uri,
			Username: c.Postgres.User,
			Password: c.Postgres.Password,
		}
	default:
		return c.Connection
	}
}

// GenerateConfig holds settings for the generate command.
type GenerateConfig struct {
	// Language target (e.g., "go")
	Lang string `yaml:"lang,omitempty"`

	// Database adapter (e.g., "neogo")
	Adapter string `yaml:"adapter,omitempty"`

	// Output directory for generated files
	Out string `yaml:"out,omitempty"`

	// Package name for generated code (Go-specific)
	Package string `yaml:"package,omitempty"`
}

// DefaultConfigNames are the filenames we search for.
var DefaultConfigNames = []string{".scaf.yaml", ".scaf.yml", "scaf.yaml", "scaf.yml"}

// LoadConfig finds and loads the nearest .scaf.yaml walking up from dir.
func LoadConfig(dir string) (*Config, error) {
	path, err := FindConfig(dir)
	if err != nil {
		return nil, err
	}

	return LoadConfigFile(path)
}

// FindConfig searches for a config file starting from dir and walking up.
func FindConfig(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for dir := absDir; ; {
		for _, name := range DefaultConfigNames {
			path := filepath.Join(dir, name)

			_, err := os.Stat(path)
			if err == nil {
				return path, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrConfigNotFound
		}

		dir = parent
	}
}

// LoadConfigFile loads a config from a specific path.
func LoadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	var cfg Config

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DialectFor returns the dialect name for a given file path.
// It checks file-specific patterns first, then falls back to the default.
func (c *Config) DialectFor(filePath string) string {
	for pattern, dialect := range c.Files {
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return dialect
		}
	}

	return c.Dialect
}
