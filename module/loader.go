package module

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/rlch/scaf"
)

// Loader handles loading and caching of scaf modules.
type Loader struct {
	// cache stores loaded modules by absolute path.
	cache map[string]*Module

	// Parser is the function used to parse .scaf files.
	// Defaults to scaf.Parse but can be overridden for testing.
	Parser func(data []byte) (*scaf.Suite, error)
}

// NewLoader creates a new module loader.
func NewLoader() *Loader {
	return &Loader{
		cache:  make(map[string]*Module),
		Parser: scaf.Parse,
	}
}

// Load loads a module from the given path.
// Relative paths are resolved from the current working directory.
// Returns a cached module if already loaded.
func (l *Loader) Load(path string) (*Module, error) {
	absPath, err := l.resolvePath(path, "")
	if err != nil {
		return nil, err
	}

	return l.loadAbsolute(absPath, "")
}

// LoadFrom loads a module, resolving the path relative to a base module.
// This is used for loading imports.
func (l *Loader) LoadFrom(path string, from *Module) (*Module, error) {
	absPath, err := l.resolvePath(path, from.Path)
	if err != nil {
		return nil, &LoadError{
			Path:         path,
			ImportedFrom: from.Path,
			Cause:        err,
		}
	}

	return l.loadAbsolute(absPath, from.Path)
}

// resolvePath resolves a path to an absolute path.
// If basePath is provided, relative paths are resolved from its directory.
func (l *Loader) resolvePath(path, basePath string) (string, error) {
	// If path is already absolute, use it directly
	if filepath.IsAbs(path) {
		return l.normalizeScafPath(path)
	}

	// Resolve relative path
	var baseDir string
	if basePath != "" {
		baseDir = filepath.Dir(basePath)
	} else {
		var err error

		baseDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	absPath := filepath.Join(baseDir, path)

	return l.normalizeScafPath(absPath)
}

// normalizeScafPath ensures the path has .scaf extension and exists.
func (l *Loader) normalizeScafPath(path string) (string, error) {
	// Clean the path
	path = filepath.Clean(path)

	// Try the path as-is first
	if _, err := os.Stat(path); err == nil {
		return filepath.Abs(path)
	}

	// If no extension, try adding .scaf
	if filepath.Ext(path) == "" {
		withExt := path + ".scaf"
		if _, err := os.Stat(withExt); err == nil {
			return filepath.Abs(withExt)
		}
	}

	return "", fmt.Errorf("%w: %s", ErrModuleNotFound, path)
}

// loadAbsolute loads a module from an absolute path.
func (l *Loader) loadAbsolute(absPath, importedFrom string) (*Module, error) {
	// Check cache
	if mod, ok := l.cache[absPath]; ok {
		return mod, nil
	}

	// Read file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, &LoadError{
			Path:         absPath,
			ImportedFrom: importedFrom,
			Cause:        err,
		}
	}

	// Parse
	suite, err := l.Parser(data)
	if err != nil {
		return nil, &LoadError{
			Path:         absPath,
			ImportedFrom: importedFrom,
			Cause:        fmt.Errorf("%w: %w", ErrParseError, err),
		}
	}

	// Create module and cache it
	mod := NewModule(absPath, suite)
	l.cache[absPath] = mod

	return mod, nil
}

// Clear clears the module cache.
func (l *Loader) Clear() {
	l.cache = make(map[string]*Module)
}

// Cached returns all cached modules.
func (l *Loader) Cached() map[string]*Module {
	result := make(map[string]*Module, len(l.cache))
	maps.Copy(result, l.cache)

	return result
}
