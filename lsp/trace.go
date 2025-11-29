package lsp

import (
	"time"

	"go.uber.org/zap"
)

// traceHandler logs entry and exit of a handler for debugging freezes.
func (s *Server) traceHandler(name string) func() {
	start := time.Now()
	s.logger.Info(">>> HANDLER START", zap.String("handler", name))
	return func() {
		s.logger.Info("<<< HANDLER END", zap.String("handler", name), zap.Duration("elapsed", time.Since(start)))
	}
}
