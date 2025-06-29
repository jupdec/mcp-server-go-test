package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// MCP Protocol Types
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Tool definitions
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type ToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCP Client
type MCPClient struct {
	baseURL    string
	httpClient *http.Client
	requestID  int
}

// NewMCPClient creates a new MCP client
func NewMCPClient(baseURL string) *MCPClient {
	return &MCPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		requestID: 0,
	}
}

// extractSSEData extracts JSON data from Server-Sent Events format
func extractSSEData(sseResponse string) string {
	scanner := bufio.NewScanner(strings.NewReader(sseResponse))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			// Extract everything after "data: "
			return strings.TrimSpace(line[5:])
		}
	}
	return ""
}

// sendRequest sends an MCP request and returns the response
func (c *MCPClient) sendRequest(ctx context.Context, method string, params interface{}) (*MCPResponse, error) {
	c.requestID++
	
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      c.requestID,
		Method:  method,
		Params:  params,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("Sending MCP request to %s: %s", c.baseURL, method)
	log.Printf("Request body: %s", string(reqBody))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("Response status: %d", resp.StatusCode)
	log.Printf("Response body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(body))
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle empty responses (common with notifications)
	if len(body) == 0 {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      c.requestID,
			Result:  nil,
		}, nil
	}

	// Check if response is Server-Sent Events format
	bodyStr := string(body)
	if strings.HasPrefix(bodyStr, "event:") {
		// Parse SSE format
		jsonData := extractSSEData(bodyStr)
		if jsonData == "" {
			log.Printf("No data found in SSE response: %s", bodyStr)
			return &MCPResponse{
				JSONRPC: "2.0",
				ID:      c.requestID,
				Result:  nil,
			}, nil
		}
		
		var mcpResp MCPResponse
		if err := json.Unmarshal([]byte(jsonData), &mcpResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal SSE JSON data: %w", err)
		}
		
		if mcpResp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
		}
		
		return &mcpResp, nil
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		// If it's not valid JSON, it might be a notification or SSE response
		log.Printf("Non-JSON response received: %s", string(body))
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      c.requestID,
			Result:  map[string]interface{}{"raw": string(body)},
		}, nil
	}

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	return &mcpResp, nil
}

// Initialize initializes the MCP connection
func (c *MCPClient) Initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
		},
		"clientInfo": map[string]interface{}{
			"name":    "bedrock-mcp-client",
			"version": "1.0.0",
		},
	}

	resp, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return err
	}

	log.Printf("Initialize response: %+v", resp.Result)

	// Send initialized notification - required for server to be ready
	log.Printf("Sending initialized notification...")
	
	notifyParams := map[string]interface{}{}
	c.requestID++
	
	notifyReq := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  notifyParams,
		// Note: notifications don't have an ID in MCP spec
	}

	reqBody, err := json.Marshal(notifyReq)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	log.Printf("Notification request: %s", string(reqBody))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create notification request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	resp2, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("notification request failed: %w", err)
	}
	defer resp2.Body.Close()

	// Read and parse notification response
	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		return fmt.Errorf("failed to read notification response: %w", err)
	}

	log.Printf("Notification response status: %d", resp2.StatusCode)
	log.Printf("Notification response body: %s", string(body))

	// Parse the notification response if it contains JSON
	if len(body) > 0 {
		bodyStr := string(body)
		if strings.HasPrefix(bodyStr, "event:") {
			// Extract JSON from SSE
			jsonData := extractSSEData(bodyStr)
			log.Printf("Extracted notification JSON: %s", jsonData)
			
			if jsonData != "" {
				var notifyResp MCPResponse
				if err := json.Unmarshal([]byte(jsonData), &notifyResp); err == nil {
					if notifyResp.Error != nil {
						return fmt.Errorf("notification error %d: %s", notifyResp.Error.Code, notifyResp.Error.Message)
					}
				}
			}
		} else {
			// Try to parse as direct JSON
			var notifyResp MCPResponse
			if err := json.Unmarshal(body, &notifyResp); err == nil {
				if notifyResp.Error != nil {
					return fmt.Errorf("notification error %d: %s", notifyResp.Error.Code, notifyResp.Error.Message)
				}
			}
		}
	}

	log.Printf("MCP client successfully initialized")
	return nil
}

// ListTools retrieves available tools from the MCP server
func (c *MCPClient) ListTools(ctx context.Context) ([]Tool, error) {
	resp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	// Parse the tools from the response
	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	toolsInterface, ok := resultMap["tools"]
	if !ok {
		return nil, fmt.Errorf("no tools found in response")
	}

	toolsBytes, err := json.Marshal(toolsInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tools: %w", err)
	}

	var tools []Tool
	if err := json.Unmarshal(toolsBytes, &tools); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tools: %w", err)
	}

	return tools, nil
}

// CallTool executes a tool with the given arguments
func (c *MCPClient) CallTool(ctx context.Context, toolCall ToolCall) (*ToolResult, error) {
	params := map[string]interface{}{
		"name":      toolCall.Name,
		"arguments": toolCall.Arguments,
	}

	resp, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	// Parse the tool result
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	var result ToolResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &result, nil
}

// BedrockToolHandler handles tool calls from Bedrock agents
type BedrockToolHandler struct {
	mcpClient *MCPClient
}

// NewBedrockToolHandler creates a new Bedrock tool handler
func NewBedrockToolHandler(mcpServerURL string) *BedrockToolHandler {
	return &BedrockToolHandler{
		mcpClient: NewMCPClient(mcpServerURL),
	}
}

// Initialize sets up the MCP connection and retrieves available tools
func (h *BedrockToolHandler) Initialize(ctx context.Context) ([]Tool, error) {
	if err := h.mcpClient.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	tools, err := h.mcpClient.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return tools, nil
}

// HandleToolUse processes a tool call from Bedrock
func (h *BedrockToolHandler) HandleToolUse(ctx context.Context, toolUse map[string]interface{}) (map[string]interface{}, error) {
	// Extract tool name and input from Bedrock format
	toolUseID, _ := toolUse["toolUseId"].(string)
	name, ok := toolUse["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing tool name")
	}

	input, ok := toolUse["input"].(map[string]interface{})
	if !ok {
		input = make(map[string]interface{})
	}

	// Create tool call
	toolCall := ToolCall{
		Name:      name,
		Arguments: input,
	}

	// Execute the tool
	result, err := h.mcpClient.CallTool(ctx, toolCall)
	if err != nil {
		return map[string]interface{}{
			"toolUseId": toolUseID,
			"content": []map[string]interface{}{
				{
					"text": fmt.Sprintf("Error executing tool: %v", err),
				},
			},
			"status": "error",
		}, nil
	}

	// Format response for Bedrock
	content := make([]map[string]interface{}, len(result.Content))
	for i, block := range result.Content {
		content[i] = map[string]interface{}{
			"text": block.Text,
		}
	}

	status := "success"
	if result.IsError {
		status = "error"
	}

	return map[string]interface{}{
		"toolUseId": toolUseID,
		"content":   content,
		"status":    status,
	}, nil
}

// ConvertToolsForBedrock converts MCP tools to Bedrock tool format
func (h *BedrockToolHandler) ConvertToolsForBedrock(tools []Tool) []map[string]interface{} {
	bedrockTools := make([]map[string]interface{}, len(tools))
	
	for i, tool := range tools {
		bedrockTools[i] = map[string]interface{}{
			"toolSpec": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": map[string]interface{}{
					"json": tool.InputSchema,
				},
			},
		}
	}
	
	return bedrockTools
}

// Example usage and HTTP server for Bedrock integration
func main() {
	// Try different common MCP endpoints
	mcpEndpoints := []string{
		"http://localhost:3001/mcp",  // We know this one works
	}
	
	var handler *BedrockToolHandler
	var workingEndpoint string
	
	for _, endpoint := range mcpEndpoints {
		log.Printf("Trying MCP endpoint: %s", endpoint)
		testHandler := NewBedrockToolHandler(endpoint)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		
		if err := testHandler.mcpClient.Initialize(ctx); err != nil {
			log.Printf("Failed to connect to %s: %v", endpoint, err)
			cancel()
			continue
		}
		
		handler = testHandler
		workingEndpoint = endpoint
		cancel()
		break
	}
	
	if handler == nil {
		log.Fatal("Could not connect to MCP server at any of the attempted endpoints. Please check:")
		log.Fatal("1. Your MCP server is running")
		log.Fatal("2. The correct endpoint URL")
		log.Fatal("3. The server accepts HTTP POST requests with JSON-RPC 2.0")
		return
	}
	
	log.Printf("Successfully connected to MCP server at: %s", workingEndpoint)
	
	ctx := context.Background()
	
	// Initialize and get tools
	tools, err := handler.Initialize(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	
	log.Printf("Found %d tools:", len(tools))
	for _, tool := range tools {
		log.Printf("- %s: %s", tool.Name, tool.Description)
	}
	
	// Convert tools for Bedrock format
	bedrockTools := handler.ConvertToolsForBedrock(tools)
	
	// Set up HTTP server for Bedrock integration
	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tools": bedrockTools,
		})
	})
	
	http.HandleFunc("/invoke", func(w http.ResponseWriter, r *http.Request) {
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		
		toolUse, ok := request["toolUse"].(map[string]interface{})
		if !ok {
			http.Error(w, "Missing toolUse", http.StatusBadRequest)
			return
		}
		
		result, err := handler.HandleToolUse(ctx, toolUse)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
	
	log.Println("Starting server on :8080")
	log.Println("Endpoints:")
	log.Println("  GET /tools - List available tools")
	log.Println("  POST /invoke - Execute tool")
	
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}