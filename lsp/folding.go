package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
)

// FoldingRanges handles textDocument/foldingRange requests.
// Returns folding ranges for queries, scopes, groups, tests, and setup blocks.
func (s *Server) FoldingRanges(_ context.Context, params *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	s.logger.Debug("FoldingRanges",
		zap.String("uri", string(params.TextDocument.URI)))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil
	}

	// Get document line count to validate ranges
	lineCount := uint32(len(splitLines(doc.Content)))

	var ranges []protocol.FoldingRange

	// Add folding ranges for imports (if multiple)
	if len(doc.Analysis.Suite.Imports) > 1 {
		firstImport := doc.Analysis.Suite.Imports[0]
		lastImport := doc.Analysis.Suite.Imports[len(doc.Analysis.Suite.Imports)-1]
		if r, ok := s.validFoldingRange(firstImport.Pos.Line-1, lastImport.EndPos.Line-1, lineCount, protocol.ImportsFoldingRange); ok {
			ranges = append(ranges, r)
		}
	}

	// Add folding ranges for queries
	for _, q := range doc.Analysis.Suite.Functions {
		if r, ok := s.validFoldingRange(q.Pos.Line-1, q.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
			ranges = append(ranges, r)
		}
	}

	// Add folding range for global setup
	if doc.Analysis.Suite.Setup != nil {
		if r, ok := s.validFoldingRange(doc.Analysis.Suite.Setup.Pos.Line-1, doc.Analysis.Suite.Setup.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
			ranges = append(ranges, r)
		}
	}

	// Add folding ranges for scopes
	for _, scope := range doc.Analysis.Suite.Scopes {
		ranges = append(ranges, s.scopeFoldingRanges(scope, lineCount)...)
	}

	s.logger.Debug("FoldingRanges result",
		zap.Int("count", len(ranges)),
		zap.Uint32("lineCount", lineCount))

	return ranges, nil
}

// validFoldingRange creates a folding range only if the line numbers are valid.
// Returns false if the range is invalid (e.g., from a parse error with bad positions).
func (s *Server) validFoldingRange(startLine, endLine int, lineCount uint32, kind protocol.FoldingRangeKind) (protocol.FoldingRange, bool) {
	// Check for invalid/negative line numbers
	if startLine < 0 || endLine < 0 {
		return protocol.FoldingRange{}, false
	}

	start := uint32(startLine)
	end := uint32(endLine)

	// Check for overflow (very large numbers from uninitialized positions)
	if start > lineCount || end > lineCount {
		return protocol.FoldingRange{}, false
	}

	// Folding range must span at least 2 lines
	if end <= start {
		return protocol.FoldingRange{}, false
	}

	return protocol.FoldingRange{
		StartLine: start,
		EndLine:   end,
		Kind:      kind,
	}, true
}

// splitLines splits content into lines for counting.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// scopeFoldingRanges creates folding ranges for a query scope and its contents.
func (s *Server) scopeFoldingRanges(scope *scaf.QueryScope, lineCount uint32) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	// Add range for the scope itself
	if r, ok := s.validFoldingRange(scope.Pos.Line-1, scope.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
		ranges = append(ranges, r)
	}

	// Add range for scope setup if present
	if scope.Setup != nil {
		if r, ok := s.validFoldingRange(scope.Setup.Pos.Line-1, scope.Setup.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
			ranges = append(ranges, r)
		}
	}

	// Add ranges for items (tests and groups)
	for _, item := range scope.Items {
		ranges = append(ranges, s.itemFoldingRanges(item, lineCount)...)
	}

	return ranges
}

// itemFoldingRanges creates folding ranges for a test or group.
func (s *Server) itemFoldingRanges(item *scaf.TestOrGroup, lineCount uint32) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	if item.Test != nil {
		ranges = append(ranges, s.testFoldingRanges(item.Test, lineCount)...)
	}

	if item.Group != nil {
		ranges = append(ranges, s.groupFoldingRanges(item.Group, lineCount)...)
	}

	return ranges
}

// testFoldingRanges creates folding ranges for a test.
func (s *Server) testFoldingRanges(test *scaf.Test, lineCount uint32) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	// Add range for the test itself
	if r, ok := s.validFoldingRange(test.Pos.Line-1, test.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
		ranges = append(ranges, r)
	}

	// Add range for test setup if present
	if test.Setup != nil {
		if r, ok := s.validFoldingRange(test.Setup.Pos.Line-1, test.Setup.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
			ranges = append(ranges, r)
		}
	}

	// Add ranges for asserts
	for _, assert := range test.Asserts {
		if r, ok := s.validFoldingRange(assert.Pos.Line-1, assert.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
			ranges = append(ranges, r)
		}
	}

	return ranges
}

// groupFoldingRanges creates folding ranges for a group and its contents.
func (s *Server) groupFoldingRanges(group *scaf.Group, lineCount uint32) []protocol.FoldingRange {
	var ranges []protocol.FoldingRange

	// Add range for the group itself
	if r, ok := s.validFoldingRange(group.Pos.Line-1, group.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
		ranges = append(ranges, r)
	}

	// Add range for group setup if present
	if group.Setup != nil {
		if r, ok := s.validFoldingRange(group.Setup.Pos.Line-1, group.Setup.EndPos.Line-1, lineCount, protocol.RegionFoldingRange); ok {
			ranges = append(ranges, r)
		}
	}

	// Add ranges for nested items
	for _, item := range group.Items {
		ranges = append(ranges, s.itemFoldingRanges(item, lineCount)...)
	}

	return ranges
}
