// Command scaf-lsp is a Language Server Protocol server for the scaf DSL.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/rlch/scaf/lsp"

	// Import dialects to register their analyzers via init().
	_ "github.com/rlch/scaf/dialects/cypher"
)

var (
	dialectFlag = flag.String("dialect", "cypher", "Query dialect (cypher, sql)")
	debugFlag   = flag.Bool("debug", false, "Enable debug logging")
	logfileFlag = flag.String("logfile", "", "Log file path (in addition to LSP window/logMessage)")
	traceFlag   = flag.Bool("trace", false, "Enable trace logging (very verbose)")
)

func main() {
	flag.Parse()

	// Determine log level
	var level zapcore.Level
	switch {
	case *traceFlag:
		level = zapcore.DebugLevel
	case *debugFlag:
		level = zapcore.DebugLevel
	default:
		level = zapcore.InfoLevel
	}

	// Create a stderr logger for initial startup (before we have a client)
	stderrConfig := zap.NewDevelopmentConfig()
	stderrConfig.OutputPaths = []string{"stderr"}
	stderrConfig.ErrorOutputPaths = []string{"stderr"}
	stderrConfig.Level = zap.NewAtomicLevelAt(level)

	startupLogger, err := stderrConfig.Build()
	if err != nil {
		panic(err)
	}

	startupLogger.Info("Starting scaf-lsp server",
		zap.String("dialect", *dialectFlag),
		zap.Bool("debug", *debugFlag),
		zap.Bool("trace", *traceFlag),
		zap.String("logfile", *logfileFlag))

	ctx := context.Background()

	err = run(ctx, startupLogger, os.Stdin, os.Stdout, *dialectFlag, level, *logfileFlag)
	if err != nil {
		// EOF is expected when client disconnects - don't treat as fatal
		if errors.Is(err, io.EOF) {
			startupLogger.Info("Client disconnected")
			return
		}
		// Check for "closed" error which is also normal shutdown
		if err.Error() == "closed" {
			startupLogger.Info("Connection closed")
			return
		}
		startupLogger.Error("Server error", zap.Error(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, startupLogger *zap.Logger, in io.Reader, out io.Writer, dialect string, level zapcore.Level, logfile string) error {
	// Create a JSON-RPC stream connection over stdio
	stream := jsonrpc2.NewStream(&readWriteCloser{in, out})
	conn := jsonrpc2.NewConn(stream)

	// Create a client to send notifications to the editor
	client := protocol.ClientDispatcher(conn, startupLogger)

	// Create a logger that sends to both LSP window/logMessage and stderr/file.
	// This ensures logs appear in Neovim's :LspLog (via window/logMessage)
	// and also in stderr for debugging.
	var stderrCore zapcore.Core
	if logfile != "" {
		// Log to file
		file, err := os.OpenFile(logfile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			startupLogger.Warn("Failed to open logfile, falling back to stderr", zap.Error(err))
			stderrCore = createStderrCore(level)
		} else {
			stderrCore = zapcore.NewCore(
				zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
				zapcore.AddSync(file),
				level,
			)
		}
	} else {
		stderrCore = createStderrCore(level)
	}

	// Create dual logger: sends to LSP client AND stderr/file
	logger := lsp.NewLSPLogger(client, stderrCore, level)

	logger.Info("LSP connection established, logging to window/logMessage")

	// Create our LSP server with the dual logger
	server := lsp.NewServer(client, logger, dialect)

	// Register the server handler with the connection
	conn.Go(ctx, protocol.ServerHandler(server, nil))

	// Wait for the connection to close
	<-conn.Done()

	return conn.Err()
}

func createStderrCore(level zapcore.Level) zapcore.Core {
	return zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.Lock(os.Stderr),
		level,
	)
}

// readWriteCloser wraps separate reader/writer into io.ReadWriteCloser.
type readWriteCloser struct {
	io.Reader
	io.Writer
}

func (rwc *readWriteCloser) Close() error {
	// Close writer if it's closeable
	if c, ok := rwc.Writer.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
