package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"
	pb "mcp/shared/proto"
)

type ToolManager struct {
	pb.UnimplementedToolServer
}

func (t *ToolManager) Name() string {
	return "tool_manager"
}

func (t *ToolManager) Description() string {
	return "Manage/Delete tools. Args: operation ('delete'), tool_name."
}

// GetDescription for gRPC
func (t *ToolManager) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchemaJson: `{
			"type": "object",
			"properties": {
				"operation": {
					"type": "string",
					"description": "Operation: 'list' or 'delete'."
				},
				"tool_name": {
					"type": "string",
					"description": "Tool name to delete."
				}
			},
			"required": ["operation"]
		}`,
	}, nil
}

// Run for gRPC
func (t *ToolManager) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	args := in.Arguments.AsMap()
	result, err := t.execute(args)
	if err != nil {
		return &pb.ToolRunResponse{Error: err.Error()}, nil
	}

	val, _ := structpb.NewValue(result)
	return &pb.ToolRunResponse{Result: val}, nil
}

// execute is the shared logic
func (t *ToolManager) execute(args map[string]interface{}) (string, error) {
	operation, _ := args["operation"].(string)

	switch operation {
	case "list":
		return t.listTools()
	case "delete":
		toolName, ok := args["tool_name"].(string)
		if !ok || toolName == "" {
			return "", fmt.Errorf("tool_name is required for delete operation")
		}
		return t.deleteTool(toolName)
	default:
		return "", fmt.Errorf("unknown operation: %s", operation)
	}
}

// GetToolDefinition for Stdio
func (t *ToolManager) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: `{
			"type": "object",
			"properties": {
				"operation": {
					"type": "string"
				},
				"tool_name": {
					"type": "string"
				}
			},
			"required": ["operation"]
		}`,
	}
}

// StdioHandler
func (t *ToolManager) StdioHandler(args map[string]interface{}) (interface{}, error) {
	res, err := t.execute(args)
	if err != nil {
		return nil, err
	}
	return map[string]string{"result": res}, nil
}

func (t *ToolManager) listTools() (string, error) {
	return "Use 'tools/list' via MCP to see active tools. To see physical directories, use 'list_directory' tool.", nil
}

var adminTokenFile string

func (t *ToolManager) deleteTool(toolName string) (string, error) {
	if toolName == "tool_manager" || toolName == "file_writer" || toolName == "file_reader" {
		return "", fmt.Errorf("cannot delete system critical tool: %s", toolName)
	}

	url := fmt.Sprintf("http://127.0.0.1:8080/admin/tools/delete?name=%s", toolName)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Read admin token from file
	if adminTokenFile != "" {
		tokenBytes, err := os.ReadFile(adminTokenFile)
		if err != nil {
			return "", fmt.Errorf("failed to read admin token file: %v", err)
		}
		token := strings.TrimSpace(string(tokenBytes))
		// Use the new custom header for internal admin communication
		req.Header.Set("X-MCP-Admin-Key", token)
	} else {
		return "", fmt.Errorf("ADMIN_TOKEN not provided")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to connect to server admin API: %v. Is the server running on port 8080?", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("server returned error: %s - %s", resp.Status, string(body))
	}

	return fmt.Sprintf("Tool '%s' successfully deleted and unloaded: %s", toolName, string(body)), nil
}

func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.StringVar(&adminTokenFile, "admin_token-from-file", "", "Path to ADMIN_TOKEN secret file")
	flag.Parse()

	toolcore.Start(&ToolManager{}, *transport, *port)
}
