package main

import (
	"context"
	"flag"
	"fmt"
	"google.golang.org/protobuf/types/known/structpb"
	"path/filepath"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"
)

// ListDirectoryTool now acts as a proxy to the agent's file system capabilities.
type ListDirectoryTool struct {
	pb.UnimplementedToolServer
}

const schema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the directory to list (e.g., 'C:\\Users' or './src')."
		}
	},
	"required": ["path"]
}`

// GetDescription returns the tool's description.
func (t *ListDirectoryTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "list_directory",
		Description:     "View files in a folder. Args: path (e.g. 'C:/Users').",
		InputSchemaJson: schema,
	}, nil
}

// Run for gRPC now returns a resource request.
func (t *ListDirectoryTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	path, ok := in.Arguments.Fields["path"].GetKind().(*structpb.Value_StringValue)
	if !ok || path.StringValue == "" {
		return &pb.ToolRunResponse{Error: "Error: The 'path' argument is required and cannot be empty."}, nil
	}

	// This is the key change: instead of accessing the filesystem,
	// we create a structured response that the main server will intercept.

	// Ensure forward slashes for URI compatibility, especially on Windows
	uriPath := filepath.ToSlash(path.StringValue)

	resourceRequest, err := structpb.NewStruct(map[string]interface{}{
		"_type":     "resource_request",
		"operation": "list",
		"uri":       fmt.Sprintf("file:///%s", uriPath),
	})
	if err != nil {
		return &pb.ToolRunResponse{Error: "failed to create resource request"}, nil
	}

	return &pb.ToolRunResponse{Result: structpb.NewStructValue(resourceRequest)}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *ListDirectoryTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "list_directory",
		Description: "View files in a folder. Args: path (e.g. 'C:/Users').",
		InputSchema: schema,
	}
}

// StdioHandler now also returns a resource request.
func (t *ListDirectoryTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	path, ok := arguments["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("Error: The 'path' argument is required and cannot be empty.")
	}

	// The handler returns a map that will be marshaled to JSON.
	// The server's adapter will intercept this special structure.

	// Ensure forward slashes for URI compatibility, especially on Windows
	uriPath := filepath.ToSlash(path)

	return map[string]interface{}{
		"_type":     "resource_request",
		"operation": "list",
		"uri":       fmt.Sprintf("file:///%s", uriPath),
	}, nil
}

func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.Parse()

	toolcore.Start(&ListDirectoryTool{}, *transport, *port)
}
