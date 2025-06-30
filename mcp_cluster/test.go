package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol-ce/go-sdk/client"
	"github.com/modelcontextprotocol-ce/go-sdk/client/stream"
	"github.com/modelcontextprotocol-ce/go-sdk/spec"
)

// --- Your custom HTTP Streaming Transport ---
type HttpStreamingTransport struct {
	*stream.StreamingTransport
	serverURL string
	client    *http.Client
}

func NewHttpStreamingTransport(serverURL string, debug bool) *HttpStreamingTransport {
	transport := &HttpStreamingTransport{
		StreamingTransport: stream.NewStreamingTransport(debug),
		serverURL:          serverURL,
		client:             &http.Client{Timeout: 60 * time.Second},
	}
	transport.StreamingTransport.MessageSender = transport.httpSendMessage
	return transport
}

func (t *HttpStreamingTransport) httpSendMessage(ctx context.Context, message *spec.JSONRPCMessage) (*spec.JSONRPCMessage, error) {
	jsonData, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", t.serverURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	var response spec.JSONRPCMessage
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &response, nil
}

// --- Main MCP Client Example ---

func main() {
	serverURL := "http://localhost:3001/mcp"
	transport := NewHttpStreamingTransport(serverURL, true)

	// Use the session API, not the builder pattern!
	session, err := client.NewClientSession(transport, 30*time.Second)
	if err != nil {
		log.Fatalf("Failed to create MCP client session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Choose your tool: "list-clusters" or "describe-clusters"
	toolCommand := "list-clusters"
	// toolCommand := "describe-clusters"

	request := spec.NewCreateMessageRequestBuilder().
		Content(toolCommand).
		Build()

	fmt.Printf("Sending request: %s\n", toolCommand)
	resultCh, errCh := session.CreateMessageStream(ctx, &request)

	var fullContent string
	for {
		select {
		case result, ok := <-resultCh:
			if !ok {
				fmt.Println("\nStreaming complete. Final content:")
				fmt.Println(fullContent)
				return
			}
			fmt.Printf("%s", result.Content)
			fullContent += result.Content
		case err, ok := <-errCh:
			if ok {
				log.Fatalf("Streaming error: %v", err)
			}
		case <-ctx.Done():
			log.Fatalf("Streaming timed out: %v", ctx.Err())
		}
	}
}
