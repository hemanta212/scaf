// Command scaf-lsp is a Language Server Protocol server for the scaf DSL.
package main

import (
	"context"
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
)

func main() {
	flag.Parse()

	// Set up logging to stderr (stdout is for LSP communication)
	config := zap.NewDevelopmentConfig()
	config.OutputPaths = []string{"stderr"}
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	if *debugFlag {
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}

	logger, err := config.Build()
	if err != nil {
		panic(err)
	}

	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("Starting scaf-lsp server", zap.String("dialect", *dialectFlag))

	ctx := context.Background()

	err = run(ctx, logger, os.Stdin, os.Stdout, *dialectFlag)
	if err != nil {
		logger.Fatal("Server error", zap.Error(err))
	}
}

func run(ctx context.Context, logger *zap.Logger, in io.Reader, out io.Writer, dialect string) error {
	// Create a JSON-RPC stream connection over stdio
	stream := jsonrpc2.NewStream(&readWriteCloser{in, out})
	conn := jsonrpc2.NewConn(stream)

	// Create a client to send notifications to the editor
	client := protocol.ClientDispatcher(conn, logger)

	// Create our LSP server
	server := lsp.NewServer(client, logger, dialect)

	// Register the server handler with the connection
	conn.Go(ctx, protocol.ServerHandler(server, nil))

	// Wait for the connection to close
	<-conn.Done()

	return conn.Err()
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
