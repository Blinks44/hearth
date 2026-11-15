package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/egoist/mygo"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Blinks44/hearth/internal/mcpserver"
	"github.com/Blinks44/hearth/internal/proposals"
)

// serveMCP is selected before font registration or window initialization.
// Stdout belongs exclusively to JSON-RPC; diagnostics go to stderr.
func serveMCP(args []string, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: hearth mcp")
		return 2
	}
	source, err := mcpserver.DefaultReader(mygo.App.Name())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "Hearth could not locate its local data directories.")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	version := mygo.App.Version()
	if version == "" {
		version = "dev"
	}
	if err = mcpserver.New(source, version, func(ctx context.Context, q proposals.Request) error {
		return proposals.Send(ctx, source.SettingsDir, q)
	}).Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		_, _ = fmt.Fprintln(stderr, "Hearth MCP session ended with an error.")
		return 1
	}
	return 0
}
