package mcp

import (
    "context"
    "log"
    "github.com/mark3labs/mcp-go/client"
    "github.com/mark3labs/mcp-go/mcp"
)

type MCPClient struct {
    session *mcp.Session
    tools   map[string]ToolHandler
}

type ToolHandler func(params map[string]interface{}) (map[string]interface{}, error)

func NewClient(url string) *MCPClient {
    c, _ := client.NewStreamableHTTPClient(url)
    s, _ := c.Initialize(context.Background(), &mcp.InitializeRequest{})
    return &MCPClient{session: s, tools: make(map[string]ToolHandler)}
}

func (m *MCPClient) RegisterTool(name string, handler ToolHandler) {
    m.tools[name] = handler
}

func (m *MCPClient) Start(ctx context.Context) {
    for {
        msg, err := m.session.NextMessage(ctx)
        if err != nil {
            log.Printf("MCP error: %v", err)
            break
        }

        switch req := msg.(type) {
        case *mcp.JsonRpcRequest:
            if req.Method == "invokeTool" {
                toolName := req.Params["name"].(string)
                handler := m.tools[toolName]
                result, err := handler(req.Params)
                if err != nil {
                    m.session.Respond(ctx, mcp.NewError(req.Id, err))
                } else {
                    m.session.Respond(ctx, mcp.NewResponse(req.Id, result))
                }
            }
        }
    }
}

func (m *MCPClient) Close() {
    m.session.Close()
}
