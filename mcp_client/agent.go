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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go/document"
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
	Code    int         `json:"code"`
	Message string      `json:"message"`
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(body))
	}

	// Handle empty responses
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
		jsonData := extractSSEData(bodyStr)
		if jsonData == "" {
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
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
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

	// Send initialized notification
	notifyParams := map[string]interface{}{}
	c.requestID++
	
	notifyReq := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  notifyParams,
	}

	reqBody, err := json.Marshal(notifyReq)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

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

	body, _ := io.ReadAll(resp2.Body)
	log.Printf("Notification response: %s", string(body))

	return nil
}

// ListTools retrieves available tools from the MCP server
func (c *MCPClient) ListTools(ctx context.Context) ([]Tool, error) {
	resp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

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

// ActionGroup represents a group of actions (MCP clients)
type ActionGroup struct {
	Name       string
	MCPClients []*MCPClient
	Tools      []Tool
}

// InlineAgent represents a Bedrock inline agent
type InlineAgent struct {
	FoundationModel string
	Instruction     string
	AgentName       string
	ActionGroups    []ActionGroup
	bedrockClient   *bedrockruntime.Client
}

// NewInlineAgent creates a new inline agent
func NewInlineAgent(foundationModel, instruction, agentName string) (*InlineAgent, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	return &InlineAgent{
		FoundationModel: foundationModel,
		Instruction:     instruction,
		AgentName:       agentName,
		ActionGroups:    []ActionGroup{},
		bedrockClient:   client,
	}, nil
}

// AddActionGroup adds an action group to the agent
func (a *InlineAgent) AddActionGroup(actionGroup ActionGroup) error {
	// Initialize all MCP clients and collect tools
	ctx := context.Background()
	
	for _, mcpClient := range actionGroup.MCPClients {
		if err := mcpClient.Initialize(ctx); err != nil {
			return fmt.Errorf("failed to initialize MCP client %s: %w", mcpClient.baseURL, err)
		}

		tools, err := mcpClient.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("failed to list tools from %s: %w", mcpClient.baseURL, err)
		}

		actionGroup.Tools = append(actionGroup.Tools, tools...)
		log.Printf("Added %d tools from MCP client %s", len(tools), mcpClient.baseURL)
	}

	a.ActionGroups = append(a.ActionGroups, actionGroup)
	return nil
}

// buildToolConfig converts MCP tools to Bedrock tool configuration
func (a *InlineAgent) buildToolConfig() []types.ToolConfiguration {
	var toolConfigs []types.ToolConfiguration

	for _, actionGroup := range a.ActionGroups {
		for _, tool := range actionGroup.Tools {
			// Convert map[string]interface{} to document.Document
			schemaDoc, err := document.NewEncoder().Encode(tool.InputSchema)
			if err != nil {
				log.Printf("Failed to encode schema for tool %s: %v", tool.Name, err)
				continue
			}

			toolSpec := types.ToolSpecification{
				Name:        aws.String(tool.Name),
				Description: aws.String(tool.Description),
				InputSchema: &types.ToolInputSchema{
					Json: schemaDoc,
				},
			}

			toolConfig := types.ToolConfiguration{
				ToolSpec: &toolSpec,
			}

			toolConfigs = append(toolConfigs, toolConfig)
		}
	}

	return toolConfigs
}

// findMCPClientForTool finds the MCP client that provides a specific tool
func (a *InlineAgent) findMCPClientForTool(toolName string) *MCPClient {
	for _, actionGroup := range a.ActionGroups {
		for _, tool := range actionGroup.Tools {
			if tool.Name == toolName {
				// Return the first MCP client (assuming one tool per client for simplicity)
				if len(actionGroup.MCPClients) > 0 {
					return actionGroup.MCPClients[0]
				}
			}
		}
	}
	return nil
}

// handleToolUse processes tool use requests from Bedrock
func (a *InlineAgent) handleToolUse(ctx context.Context, toolUse map[string]interface{}) (map[string]interface{}, error) {
	toolUseID, _ := toolUse["toolUseId"].(string)
	name, ok := toolUse["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing tool name")
	}

	input, ok := toolUse["input"].(map[string]interface{})
	if !ok {
		input = make(map[string]interface{})
	}

	// Find the MCP client for this tool
	mcpClient := a.findMCPClientForTool(name)
	if mcpClient == nil {
		return map[string]interface{}{
			"toolUseId": toolUseID,
			"content": []map[string]interface{}{
				{"text": fmt.Sprintf("Tool '%s' not found", name)},
			},
			"status": "error",
		}, nil
	}

	// Execute the tool
	toolCall := ToolCall{
		Name:      name,
		Arguments: input,
	}

	result, err := mcpClient.CallTool(ctx, toolCall)
	if err != nil {
		return map[string]interface{}{
			"toolUseId": toolUseID,
			"content": []map[string]interface{}{
				{"text": fmt.Sprintf("Error executing tool: %v", err)},
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

// Invoke processes a user input and returns the agent's response
func (a *InlineAgent) Invoke(inputText string) (string, error) {
	ctx := context.Background()
	
	// Build the conversation with system prompt and user message
	messages := []types.Message{
		{
			Role: types.ConversationRoleUser,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: inputText,
				},
			},
		},
	}

	// Build tool configuration
	toolConfig := a.buildToolConfig()

	// Create the converse request
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(a.FoundationModel),
		Messages: messages,
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{
				Value: a.Instruction,
			},
		},
	}

	// Add tool configuration if we have tools
	if len(toolConfig) > 0 {
		input.ToolConfig = &types.ToolConfiguration{
			Tools: toolConfig,
		}
	}

	// Start the conversation loop
	for {
		// Call Bedrock
		result, err := a.bedrockClient.Converse(ctx, input)
		if err != nil {
			return "", fmt.Errorf("bedrock converse failed: %w", err)
		}

		// Add assistant's response to conversation
		messages = append(messages, types.Message{
			Role:    types.ConversationRoleAssistant,
			Content: result.Output.Message.Content,
		})

		// Check if the response contains tool use
		var toolUses []map[string]interface{}
		var textResponse strings.Builder

		for _, content := range result.Output.Message.Content {
			switch c := content.(type) {
			case *types.ContentBlockMemberText:
				textResponse.WriteString(c.Value)
			case *types.ContentBlockMemberToolUse:
				toolUse := map[string]interface{}{
					"toolUseId": *c.Value.ToolUseId,
					"name":      *c.Value.Name,
					"input":     c.Value.Input,
				}
				toolUses = append(toolUses, toolUse)
			}
		}

		// If no tool use, return the text response
		if len(toolUses) == 0 {
			return textResponse.String(), nil
		}

		// Process tool uses
		var toolResults []types.ContentBlock
		for _, toolUse := range toolUses {
			result, err := a.handleToolUse(ctx, toolUse)
			if err != nil {
				return "", fmt.Errorf("tool execution failed: %w", err)
			}

			// Convert tool result to Bedrock format
			toolUseID := result["toolUseId"].(string)
			content := result["content"].([]map[string]interface{})
			
			var contentText strings.Builder
			for _, c := range content {
				if text, ok := c["text"].(string); ok {
					contentText.WriteString(text)
				}
			}

			toolResult := &types.ContentBlockMemberToolResult{
				Value: types.ToolResultBlock{
					ToolUseId: aws.String(toolUseID),
					Content: []types.ToolResultContentBlock{
						&types.ToolResultContentBlockMemberText{
							Value: contentText.String(),
						},
					},
				},
			}

			toolResults = append(toolResults, toolResult)
		}

		// Add tool results to conversation and continue
		messages = append(messages, types.Message{
			Role:    types.ConversationRoleUser,
			Content: toolResults,
		})

		// Update input for next iteration
		input.Messages = messages
	}
}

// Example usage
func main() {
	// Create MCP clients
	mcpClient1 := NewMCPClient("http://localhost:3001/mcp")

	// Create inline agent
	agent, err := NewInlineAgent(
		"us.anthropic.claude-3-5-sonnet-20241022-v2:0",
		"You are a friendly assistant for resolving user queries using available tools.",
		"SampleAgent",
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Add action group with MCP clients
	actionGroup := ActionGroup{
		Name:       "SampleActionGroup",
		MCPClients: []*MCPClient{mcpClient1},
	}

	if err := agent.AddActionGroup(actionGroup); err != nil {
		log.Fatalf("Failed to add action group: %v", err)
	}

	// Test the agent
	response, err := agent.Invoke("Convert 11am from NYC time to London time")
	if err != nil {
		log.Fatalf("Agent invocation failed: %v", err)
	}

	fmt.Printf("Agent Response: %s\n", response)
}