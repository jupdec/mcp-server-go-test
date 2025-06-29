package main

import (
    "context"
    "log"
    "mcp-client-go/config"
    "mcp-client-go/mcp"
    "mcp-client-go/tools"
)

func main() {
    ctx := context.Background()
    cfg := config.Load()

    // Start MCP server (streamable HTTP)
    mcpClient := mcp.NewClient(cfg.MCPURL)
    defer mcpClient.Close()

    // Register example tool
    mcpClient.RegisterTool("echo", tools.EchoTool)

    log.Println("Starting MCP stream loop...")
    mcpClient.Start(ctx)
}
