package module

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/rlch/scaf"
)

// MergeWarning represents a non-fatal issue detected during merge.
type MergeWarning struct {
	Span    scaf.Span
	Code    string // e.g., "same-package-import"
	Message string
}

// MergeError represents a fatal error during merge.
type MergeError struct {
	Span    scaf.Span
	Code    string // e.g., "duplicate-function"
	Message string
}

func (e *MergeError) Error() string {
	return fmt.Sprintf("%s at %s: %s", e.Code, e.Span.Start, e.Message)
}

// ParsedFile pairs a parsed scaf file with its source path.
type ParsedFile struct {
	File *scaf.File
	Path string
}

// MergePackageFiles merges multiple parsed .scaf files from the same directory
// into a single logical suite. It handles import deduplication, detects same-package
// imports (warns and skips them), and reports errors for duplicate functions or
// conflicting setup/teardown.
//
// Returns the merged file, any warnings, and an error if merge fails.
func MergePackageFiles(inputs []ParsedFile) (*scaf.File, []MergeWarning, error) {
	if len(inputs) == 0 {
		return &scaf.File{}, nil, nil
	}

	if len(inputs) == 1 {
		return inputs[0].File, nil, nil
	}

	merged := &scaf.File{}
	var warnings []MergeWarning

	// Track imports by resolved path for deduplication
	importsByPath := make(map[string]*scaf.Import)

	// Track functions by name for duplicate detection
	functionsByName := make(map[string]struct {
		fn   *scaf.Function
		file int // index of file where function was defined
	})

	// Track which file has setup/teardown
	var setupFile, teardownFile int = -1, -1

	// Normalize sibling paths to absolute for comparison
	siblingSet := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		abs, err := filepath.Abs(input.Path)
		if err == nil {
			siblingSet[abs] = true
		}
	}

	for fileIdx, input := range inputs {
		file := input.File
		fileDir := filepath.Dir(input.Path)

		// Process imports
		for _, imp := range file.Imports {
			resolvedPath := resolveImportPath(imp.Path, fileDir)

			// Check for same-package import
			if siblingSet[resolvedPath] {
				warnings = append(warnings, MergeWarning{
					Span:    imp.Span(),
					Code:    "same-package-import",
					Message: fmt.Sprintf("import %q resolves to sibling file in same package", imp.Path),
				})
				continue // skip same-package imports entirely
			}

			// Deduplicate by resolved path
			if existing, ok := importsByPath[resolvedPath]; ok {
				// Already have this import, skip duplicate
				// Use existing import (keep first occurrence)
				_ = existing
			} else {
				importsByPath[resolvedPath] = imp
				merged.Imports = append(merged.Imports, imp)
			}
		}

		// Process functions
		for _, fn := range file.Functions {
			if existing, ok := functionsByName[fn.Name]; ok {
				return nil, warnings, &MergeError{
					Span: fn.Span(),
					Code: "duplicate-function",
					Message: fmt.Sprintf("function %q already defined in file %d at %s",
						fn.Name, existing.file+1, existing.fn.Span().Start),
				}
			}
			functionsByName[fn.Name] = struct {
				fn   *scaf.Function
				file int
			}{fn: fn, file: fileIdx}
			merged.Functions = append(merged.Functions, fn)
		}

		// Process setup
		if file.Setup != nil {
			if setupFile >= 0 {
				return nil, warnings, &MergeError{
					Span: file.Setup.Span(),
					Code: "conflicting-setup",
					Message: fmt.Sprintf("setup clause already defined in file %d; only one file can have file-level setup",
						setupFile+1),
				}
			}
			setupFile = fileIdx
			merged.Setup = file.Setup
		}

		// Process teardown
		if file.Teardown != nil {
			if teardownFile >= 0 {
				// For teardown we don't have a Span easily, use NodeMeta from file
				return nil, warnings, &MergeError{
					Span: file.Span(),
					Code: "conflicting-teardown",
					Message: fmt.Sprintf("teardown clause already defined in file %d; only one file can have file-level teardown",
						teardownFile+1),
				}
			}
			teardownFile = fileIdx
			merged.Teardown = file.Teardown
		}

		// Concatenate scopes
		merged.Scopes = append(merged.Scopes, file.Scopes...)
	}

	return merged, warnings, nil
}

// resolveImportPath resolves an import path relative to the file's directory.
// Returns an absolute path for comparison purposes.
func resolveImportPath(importPath, fileDir string) string {
	if filepath.IsAbs(importPath) {
		return filepath.Clean(importPath)
	}

	if fileDir == "" {
		// Can't resolve relative path without a base directory
		return importPath
	}

	// Resolve relative to file's directory
	resolved := filepath.Join(fileDir, importPath)
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return resolved
	}

	// Try common extensions if not present
	cleaned := filepath.Clean(abs)
	if filepath.Ext(cleaned) == "" {
		// Try .scaf extension
		withExt := cleaned + ".scaf"
		return withExt
	}

	return cleaned
}

// DeduplicateImports returns a deduplicated list of imports based on resolved paths.
// This is useful for standalone import deduplication without full merge.
func DeduplicateImports(imports []*scaf.Import, baseDir string) []*scaf.Import {
	seen := make(map[string]bool)
	var result []*scaf.Import

	for _, imp := range imports {
		resolved := resolveImportPath(imp.Path, baseDir)
		if !seen[resolved] {
			seen[resolved] = true
			result = append(result, imp)
		}
	}

	return result
}

// FindSamePackageImports identifies imports that point to sibling files.
// Returns the imports and their resolved sibling paths.
func FindSamePackageImports(imports []*scaf.Import, baseDir string, siblingPaths []string) []struct {
	Import      *scaf.Import
	SiblingPath string
} {
	// Normalize sibling paths
	siblingSet := make(map[string]string, len(siblingPaths))
	for _, p := range siblingPaths {
		abs, err := filepath.Abs(p)
		if err == nil {
			siblingSet[abs] = p
		}
	}

	var result []struct {
		Import      *scaf.Import
		SiblingPath string
	}

	for _, imp := range imports {
		resolved := resolveImportPath(imp.Path, baseDir)
		if siblingPath, ok := siblingSet[resolved]; ok {
			result = append(result, struct {
				Import      *scaf.Import
				SiblingPath string
			}{
				Import:      imp,
				SiblingPath: siblingPath,
			})
		}
	}

	return result
}

// ValidateMerge checks if a set of files can be merged without errors.
// Returns nil if merge would succeed, otherwise returns the first error.
func ValidateMerge(inputs []ParsedFile) error {
	_, _, err := MergePackageFiles(inputs)
	return err
}

// CollectFunctionNames returns all function names across multiple files.
// Returns duplicates as a map of name -> list of file indices where it appears.
func CollectFunctionNames(files []*scaf.File) map[string][]int {
	nameToFiles := make(map[string][]int)

	for fileIdx, file := range files {
		for _, fn := range file.Functions {
			nameToFiles[fn.Name] = append(nameToFiles[fn.Name], fileIdx)
		}
	}

	// Filter to only duplicates
	result := make(map[string][]int)
	for name, indices := range nameToFiles {
		if len(indices) > 1 {
			result[name] = slices.Clone(indices)
		}
	}

	return result
}
