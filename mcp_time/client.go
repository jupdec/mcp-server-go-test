package main

import (
	"context"
	"log"
	"os/exec"

	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

func main() {
	// Configure Docker command for MCP server
	cmd := exec.Command("docker", "run", "-i", "--rm", "mcp/time")

	// Create IO pipes to Docker container
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to create stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to create stdout pipe: %v", err)
	}

	// Start Docker container
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start Docker container: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// Create MCP transport using Docker's stdio
	transport := stdio.NewStdioServerTransportWithIO(stdout, stdin)
	client := mcp_golang.NewClient(transport)

	// Initialize MCP connection
	if _, err := client.Initialize(context.Background()); err != nil {
		log.Fatalf("MCP initialization failed: %v", err)
	}

	// Discover available tools
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	log.Println("Available Tools:")
	for _, tool := range tools.Tools {
		desc := "No description"
		if tool.Description != nil {
			desc = *tool.Description
		}
		log.Printf("- %s: %s", tool.Name, desc)
	}

	// Call time tool with specific format
	timeArgs := map[string]interface{}{
		"format": "2006-01-02 15:04:05 MST",
	}

	log.Println("\nCalling time tool:")
	timeResponse, err := client.CallTool(context.Background(), "time", timeArgs)
	if err != nil {
		log.Fatalf("Time tool call failed: %v", err)
	}

	if timeResponse != nil &&
		len(timeResponse.Content) > 0 &&
		timeResponse.Content[0].TextContent != nil {
		log.Printf("Current time: %s", timeResponse.Content[0].TextContent.Text)
	}
}
