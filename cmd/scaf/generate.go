package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/boyter/gocodewalker"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	"github.com/rlch/scaf/language"
	golang "github.com/rlch/scaf/language/go"
	"github.com/rlch/scaf/module"
	"github.com/urfave/cli/v3"

	// Register bindings and dialects.
	_ "github.com/rlch/scaf/adapters/neogo"
	_ "github.com/rlch/scaf/dialects/cypher"
)

// Generate command errors.
var (
	ErrNoScafFilesForGenerate   = errors.New("no .scaf files found")
	ErrUnknownLanguage          = errors.New("unknown language")
	ErrLanguageNoAdapters       = errors.New("language does not support code generation with adapters")
	ErrUnknownAdapter           = errors.New("unknown adapter")
	ErrGenerateDiagnosticErrors = errors.New("scaf files contain errors")
)

func generateCommand() *cli.Command {
	return &cli.Command{
		Name:      "generate",
		Aliases:   []string{"gen"},
		Usage:     "Generate code from scaf files",
		ArgsUsage: "[files or directories...]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "lang",
				Aliases: []string{"l"},
				Usage:   "target language (go)",
				Value:   "go",
			},
			&cli.StringFlag{
				Name:    "adapter",
				Aliases: []string{"a"},
				Usage:   "database adapter (neogo)",
			},
			&cli.StringFlag{
				Name:    "dialect",
				Aliases: []string{"d"},
				Usage:   "query dialect (cypher)",
			},
			&cli.StringFlag{
				Name:    "out",
				Aliases: []string{"o"},
				Usage:   "output directory (default: same as input file)",
			},
			&cli.StringFlag{
				Name:    "package",
				Aliases: []string{"p"},
				Usage:   "Go package name (default: directory name)",
			},
			&cli.StringFlag{
				Name:    "schema",
				Aliases: []string{"s"},
				Usage:   "path to schema HCL file (e.g., .scaf-schema.hcl)",
			},
		},
		Action: runGenerate,
	}
}

func runGenerate(ctx context.Context, cmd *cli.Command) error {
	// Step 1: Load config from cwd (project root)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}

	cfg, configDir, err := loadConfigWithDir(cwd)
	if err != nil {
		// Config is optional - continue with defaults if not found
		cfg = nil
		configDir = cwd
	}

	// Step 2: Get settings from flags, falling back to config
	langName := firstNonEmpty(cmd.String("lang"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Lang }), scaf.LangGo)
	adapterName := firstNonEmpty(cmd.String("adapter"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Adapter }))
	dialectName := firstNonEmpty(cmd.String("dialect"), cfgDialect(cfg), scaf.DialectCypher)
	outputDir := firstNonEmpty(cmd.String("out"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Out }))
	packageName := firstNonEmpty(cmd.String("package"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Package }))
	schemaPath := firstNonEmpty(cmd.String("schema"), cfgString(cfg, func(c *scaf.Config) string { return c.Generate.Schema }))

	// Infer adapter from database/dialect if not specified
	if adapterName == "" {
		if cfg != nil {
			if dbName := cfg.DatabaseName(); dbName != "" {
				adapterName = scaf.AdapterForDatabase(dbName, langName)
			}
		}
		// Fall back to dialect-based inference
		if adapterName == "" && dialectName == scaf.DialectCypher {
			adapterName = scaf.AdapterNeogo
		}
	}

	// Step 3: Load schema relative to config file location
	var schema *analysis.TypeSchema
	if schemaPath != "" {
		schema, err = analysis.LoadSchema(schemaPath, configDir)
		if err != nil {
			return fmt.Errorf("loading schema: %w", err)
		}
	}

	// Step 4: Get language and binding
	lang := language.Get(langName)
	if lang == nil {
		return fmt.Errorf("%w: %s (available: %v)", ErrUnknownLanguage, langName, language.RegisteredLanguages())
	}

	goLang, ok := lang.(*golang.GoLanguage)
	if !ok {
		return fmt.Errorf("%w: %s", ErrLanguageNoAdapters, langName)
	}

	var binding golang.Binding
	if adapterName != "" {
		binding = golang.GetBinding(adapterName)
		if binding == nil {
			return fmt.Errorf("%w: %s for language %s (available: %v)", ErrUnknownAdapter, adapterName, langName, golang.RegisteredBindings())
		}
	}

	queryAnalyzer := scaf.GetAnalyzer(dialectName)

	// Step 5: Discover packages - either from args or by walking cwd
	args := cmd.Args().Slice()
	if len(args) == 0 {
		args = []string{"."}
	}

	packages, err := discoverPackages(args)
	if err != nil {
		return fmt.Errorf("discovering packages: %w", err)
	}

	if len(packages) == 0 {
		return ErrNoScafFilesForGenerate
	}

	// Step 6: Run analysis on all files first
	semanticAnalyzer := analysis.NewAnalyzerWithQueryAnalyzer(nil, nil, queryAnalyzer)
	var hasErrors bool

	for _, scafFiles := range packages {
		for _, inputFile := range scafFiles {
			data, err := os.ReadFile(inputFile) //nolint:gosec // G304: file path from user input is expected
			if err != nil {
				return fmt.Errorf("reading %s: %w", inputFile, err)
			}

			result := semanticAnalyzer.Analyze(inputFile, data)
			if result.HasErrors() {
				hasErrors = true
				for _, diag := range result.Errors() {
					loc := ""
					if diag.Span.Start.Line > 0 {
						loc = fmt.Sprintf("%s:%d:%d: ", inputFile, diag.Span.Start.Line, diag.Span.Start.Column)
					} else {
						loc = fmt.Sprintf("%s: ", inputFile)
					}
					fmt.Fprintf(os.Stderr, "%serror: %s\n", loc, diag.Message)
				}
			}
		}
	}

	if hasErrors {
		return ErrGenerateDiagnosticErrors
	}

	// Step 7: Generate each package
	opts := &generateOptions{
		goLang:      goLang,
		analyzer:    queryAnalyzer,
		binding:     binding,
		schema:      schema,
		outputDir:   outputDir,
		packageName: packageName,
	}

	for pkgDir, scafFiles := range packages {
		// Output directory: use flag if specified, otherwise use package dir
		outDir := outputDir
		if outDir == "" {
			outDir = pkgDir
		}

		if err := generateMergedFiles(scafFiles, outDir, opts); err != nil {
			return err
		}
	}

	return nil
}

// discoverPackages finds all .scaf files and groups them by directory.
// Uses gocodewalker for fast traversal with .gitignore support.
// Returns map[directory][]scafFiles.
func discoverPackages(args []string) (map[string][]string, error) {
	packages := make(map[string][]string)
	var mu sync.Mutex

	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			// Use gocodewalker for directories - respects .gitignore
			if err := walkDirWithGocodewalker(arg, func(path string) {
				dir := filepath.Dir(path)
				mu.Lock()
				packages[dir] = append(packages[dir], path)
				mu.Unlock()
			}); err != nil {
				return nil, err
			}
		} else if strings.HasSuffix(arg, ".scaf") {
			dir := filepath.Dir(arg)
			packages[dir] = append(packages[dir], arg)
		}
	}

	return packages, nil
}

// walkDirWithGocodewalker uses gocodewalker for fast directory traversal.
// It respects .gitignore and .ignore files automatically.
func walkDirWithGocodewalker(root string, callback func(path string)) error {
	fileListQueue := make(chan *gocodewalker.File, 100)

	fileWalker := gocodewalker.NewFileWalker(root, fileListQueue)
	fileWalker.AllowListExtensions = []string{"scaf"}

	// Collect errors
	var walkErr error
	fileWalker.SetErrorHandler(func(e error) bool {
		walkErr = e
		return true // continue on error
	})

	// Process files as they come in
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for f := range fileListQueue {
			callback(f.Location)
		}
	}()

	// Start walking (blocks until done, closes queue)
	if err := fileWalker.Start(); err != nil {
		return err
	}

	wg.Wait()

	return walkErr
}

// Helper functions for config access
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func cfgString(cfg *scaf.Config, getter func(*scaf.Config) string) string {
	if cfg == nil {
		return ""
	}
	return getter(cfg)
}

func cfgDialect(cfg *scaf.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.DialectName()
}

// generateMergedFiles parses, merges, and generates code for a group of files.
func generateMergedFiles(inputFiles []string, outputDir string, opts *generateOptions) error {
	// Parse all files
	var inputs []module.ParsedFile
	for _, inputFile := range inputFiles {
		data, err := os.ReadFile(inputFile) //nolint:gosec // G304: file path from user input is expected
		if err != nil {
			return fmt.Errorf("reading %s: %w", inputFile, err)
		}

		suite, err := scaf.Parse(data)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", inputFile, err)
		}
		inputs = append(inputs, module.ParsedFile{File: suite, Path: inputFile})
	}

	// Merge files from the same directory
	merged, warnings, err := module.MergePackageFiles(inputs)
	if err != nil {
		return fmt.Errorf("merging files: %w", err)
	}

	// Print merge warnings (same-package imports)
	for _, w := range warnings {
		loc := ""
		if w.Span.Start.Line > 0 {
			loc = fmt.Sprintf("%d:%d: ", w.Span.Start.Line, w.Span.Start.Column)
		}
		fmt.Fprintf(os.Stderr, "%swarning: %s\n", loc, w.Message)
	}

	// Determine package name using the Go-specific inference ladder
	packageName := opts.packageName
	if packageName == "" {
		var err error
		packageName, err = golang.InferPackageName(outputDir)
		if err != nil {
			// Fallback to sanitized directory name
			packageName = golang.SanitizePackageName(filepath.Base(outputDir))
		}
	}

	goCtx := &golang.Context{
		GenerateContext: language.GenerateContext{
			Suite:         merged,
			QueryAnalyzer: opts.analyzer,
			Schema:        opts.schema,
			OutputDir:     outputDir,
		},
		PackageName: packageName,
		Binding:     opts.binding,
	}

	files, err := opts.goLang.GenerateWithContext(goCtx)
	if err != nil {
		return fmt.Errorf("generating code for %v: %w", inputFiles, err)
	}

	// Write output files
	for filename, content := range files {
		if content == nil {
			continue
		}

		outPath := filepath.Join(outputDir, filename)

		err := os.WriteFile(outPath, content, 0o644) //nolint:gosec // G306: output file permissions are fine
		if err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}

		fmt.Printf("wrote %s\n", outPath)
	}

	return nil
}

// generateOptions holds configuration for file generation.
type generateOptions struct {
	goLang      *golang.GoLanguage
	analyzer    scaf.QueryAnalyzer
	binding     golang.Binding
	schema      *analysis.TypeSchema
	outputDir   string
	packageName string
}
