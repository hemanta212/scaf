package lsp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// lspLogCore is a zapcore.Core that sends logs to the LSP client via window/logMessage.
// This allows logs to appear in Neovim's :LspLog and other LSP client log viewers.
type lspLogCore struct {
	client    protocol.Client
	level     zapcore.Level
	encoder   zapcore.Encoder
	fields    []zapcore.Field
	mu        sync.Mutex
	ctx       context.Context
	cancelCtx context.CancelFunc

	// logQueue ensures async, non-blocking log delivery
	logQueue chan logEntry
}

type logEntry struct {
	level   protocol.MessageType
	message string
}

// NewLSPLogger creates a logger that sends logs via LSP window/logMessage notifications.
// It also logs to the provided fallback core (typically stderr) for debugging.
//
// The LSP logs will appear in Neovim's :LspLog and similar client log viewers.
func NewLSPLogger(client protocol.Client, fallbackCore zapcore.Core, level zapcore.Level) *zap.Logger {
	ctx, cancel := context.WithCancel(context.Background())

	lspCore := &lspLogCore{
		client:    client,
		level:     level,
		encoder:   zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey:     "msg",
			LevelKey:       "",  // Don't include level in message, it's in the MessageType
			TimeKey:        "",  // Don't include timestamp
			NameKey:        "logger",
			CallerKey:      "",
			FunctionKey:    "",
			StacktraceKey:  "",
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}),
		ctx:       ctx,
		cancelCtx: cancel,
		logQueue:  make(chan logEntry, 100), // Buffer for burst handling
	}

	// Start the async log sender
	go lspCore.logSender()

	// Tee to both LSP and fallback (stderr)
	tee := zapcore.NewTee(lspCore, fallbackCore)

	return zap.New(tee)
}

// logSender processes the log queue and sends to LSP client asynchronously.
func (c *lspLogCore) logSender() {
	for {
		select {
		case entry := <-c.logQueue:
			// Send to LSP client (ignore errors - client may be disconnected)
			_ = c.client.LogMessage(c.ctx, &protocol.LogMessageParams{
				Type:    entry.level,
				Message: entry.message,
			})
		case <-c.ctx.Done():
			return
		}
	}
}

// Close stops the log sender goroutine.
func (c *lspLogCore) Close() {
	c.cancelCtx()
}

// Enabled implements zapcore.Core.
func (c *lspLogCore) Enabled(level zapcore.Level) bool {
	return level >= c.level
}

// With implements zapcore.Core.
func (c *lspLogCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &lspLogCore{
		client:    c.client,
		level:     c.level,
		encoder:   c.encoder.Clone(),
		fields:    append(c.fields, fields...),
		ctx:       c.ctx,
		cancelCtx: c.cancelCtx,
		logQueue:  c.logQueue,
	}
	return clone
}

// Check implements zapcore.Core.
func (c *lspLogCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

// Write implements zapcore.Core.
func (c *lspLogCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Encode the message
	buf, err := c.encoder.EncodeEntry(entry, append(c.fields, fields...))
	if err != nil {
		return err
	}

	message := strings.TrimSpace(buf.String())
	buf.Free()

	// Map zap levels to LSP MessageType
	var msgType protocol.MessageType
	switch entry.Level {
	case zapcore.DebugLevel:
		msgType = protocol.MessageTypeLog
	case zapcore.InfoLevel:
		msgType = protocol.MessageTypeInfo
	case zapcore.WarnLevel:
		msgType = protocol.MessageTypeWarning
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		msgType = protocol.MessageTypeError
	default:
		msgType = protocol.MessageTypeInfo
	}

	// Queue the log entry (non-blocking)
	select {
	case c.logQueue <- logEntry{level: msgType, message: message}:
	default:
		// Queue full, drop the message (shouldn't happen with buffer size 100)
	}

	return nil
}

// Sync implements zapcore.Core.
func (c *lspLogCore) Sync() error {
	return nil
}

// LogToClient sends a single log message to the LSP client.
// This is a convenience function for one-off messages.
func LogToClient(ctx context.Context, client protocol.Client, level protocol.MessageType, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	_ = client.LogMessage(ctx, &protocol.LogMessageParams{
		Type:    level,
		Message: message,
	})
}
