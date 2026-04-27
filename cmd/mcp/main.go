package main

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vng/review-code-agent/internal/config"
	"github.com/vng/review-code-agent/pkg/logger"
)

func main() {
	_ = config.Load() // preload for Phase 08 (AI keys, etc.)
	log := logger.Default()

	s := server.NewMCPServer(
		"review-code-agent",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	// Placeholder tool — replaced in Phase 08.
	s.AddTool(mcp.NewTool("ping",
		mcp.WithDescription("Health check tool"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("pong"), nil
	})

	log.Info("mcp: starting stdio transport")
	if err := server.ServeStdio(s); err != nil {
		log.Error("mcp: serve error", "err", err)
	}
}
