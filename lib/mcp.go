package lib

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func CallMCPTool(ctx context.Context, command string, args []string, toolName string, toolArgs map[string]interface{}) (string, error) {
	c, err := client.NewStdioMCPClient(command, nil, args...)
	if err != nil {
		return "", fmt.Errorf("failed to create MCP client: %w", err)
	}
	defer c.Close()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "golang_botwa",
		Version: "1.0",
	}

	_, err = c.Initialize(ctx, initReq)
	if err != nil {
		return "", fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	if toolName == "" {
		listReq := mcp.ListToolsRequest{}
		listRes, err := c.ListTools(ctx, listReq)
		if err != nil || len(listRes.Tools) == 0 {
			return "", fmt.Errorf("failed to list tools or no tools available")
		}
		toolName = listRes.Tools[0].Name
	}

	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = toolName
	callReq.Params.Arguments = toolArgs

	res, err := c.CallTool(ctx, callReq)
	if err != nil {
		return "", fmt.Errorf("failed to call tool %s: %w", toolName, err)
	}

	if res.IsError {
		return "", fmt.Errorf("tool returned error")
	}

	if len(res.Content) == 0 {
		return "No content returned", nil
	}

	var textResult string
	for _, content := range res.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			textResult += textContent.Text + "\n"
		} else if b, err := json.Marshal(content); err == nil {
			textResult += string(b) + "\n"
		}
	}

	return textResult, nil
}
