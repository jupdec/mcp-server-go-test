package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// Example request payload structure (adjust as needed for your MCP API)
type MCPRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// Example response structure (adjust as needed for your MCP API)
type MCPResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mcpURL := "http://localhost:3001/mcp"
	reqBody := MCPRequest{
		Command: "list_clusters",
		Args:    []string{},
	}

	// Marshal request to JSON
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Fatalf("failed to marshal request: %v", err)
	}

	// Prepare HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", mcpURL, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read and parse response
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("failed to read response: %v", err)
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		log.Fatalf("failed to unmarshal response: %v", err)
	}

	// Output result
	if mcpResp.Error != "" {
		log.Fatalf("MCP error: %s", mcpResp.Error)
	}
	fmt.Printf("MCP Result: %s\n", mcpResp.Result)
}
