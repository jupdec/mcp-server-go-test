package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/davecgh/go-spew/spew"
	mcp_golang "github.com/metoro-io/mcp-golang"
	mcphttp "github.com/metoro-io/mcp-golang/transport/http"
)

func main() {
	// Create an HTTP client that sets the correct Accept header for Streamable HTTP
	httpClient := &http.Client{}
	transport := mcphttp.NewHTTPClientTransport("/mcp")
	transport.WithBaseURL("http://localhost:3001")
	transport.WithHTTPClient(httpClient)
	// Set Accept header for both JSON and SSE (Streamable HTTP)
	transport.WithDefaultHeader("Accept", "application/json, text/event-stream")

	client := mcp_golang.NewClient(transport)

	// Initialize the client
	resp, err := client.Initialize(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}
	log.Printf("Initialized client: %v", spew.Sdump(resp))

	// List available tools
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	log.Println("Available Tools:")
	for _, tool := range tools.Tools {
		desc := ""
		if tool.Description != nil {
			desc = *tool.Description
		}
		log.Printf("Tool: %s. Description: %s", tool.Name, desc)
	}

	// Call the time tool with different formats
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"Mon, 02 Jan 2006",
	}

	for _, format := range formats {
		args := map[string]interface{}{
			"format": format,
		}

		response, err := client.CallTool(context.Background(), "time", args)
		if err != nil {
			log.Printf("Failed to call time tool: %v", err)
			continue
		}

		if len(response.Content) > 0 && response.Content[0].TextContent != nil {
			log.Printf("Time in format %q: %s", format, response.Content[0].TextContent.Text)
		}
	}
}
