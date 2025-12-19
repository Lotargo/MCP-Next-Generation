package main

import (
	"context"
	"flag"
	"fmt"
	"google.golang.org/protobuf/types/known/structpb"
	"strings"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"
)

// FileReaderTool now acts as a proxy to the agent's file system capabilities.
type FileReaderTool struct {
	pb.UnimplementedToolServer
}

const schema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the file to read on the agent's machine (e.g., 'C:\\Users\\user\\document.txt')."
		}
	},
	"required": ["path"]
}`

// GetDescription returns the tool's description.
func (t *FileReaderTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "file_reader",
		Description:     "Read file content. Args: path (absolute).",
		InputSchemaJson: schema,
	}, nil
}

// Run for gRPC now returns a resource request for a 'read' operation.
func (t *FileReaderTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	path, ok := in.Arguments.Fields["path"].GetKind().(*structpb.Value_StringValue)
	if !ok || path.StringValue == "" {
		return &pb.ToolRunResponse{Error: "Invalid or missing 'path' argument"}, nil
	}

	// Ensure forward slashes for URI compatibility, especially on Windows
	uriPath := strings.ReplaceAll(path.StringValue, "\\", "/")

	resourceRequest, err := structpb.NewStruct(map[string]interface{}{
		"_type":     "resource_request",
		"operation": "read",
		"uri":       fmt.Sprintf("file:///%s", uriPath),
	})
	if err != nil {
		return &pb.ToolRunResponse{Error: "failed to create resource request"}, nil
	}

	return &pb.ToolRunResponse{Result: structpb.NewStructValue(resourceRequest)}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *FileReaderTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "file_reader",
		Description: "Read file content. Args: path (absolute).",
		InputSchema: schema,
	}
}

// StdioHandler now also returns a resource request for a 'read' operation.
func (t *FileReaderTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	path, ok := arguments["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("missing or invalid 'path' argument")
	}

	// Ensure forward slashes for URI compatibility, especially on Windows
	uriPath := strings.ReplaceAll(path, "\\", "/")

	return map[string]interface{}{
		"_type":     "resource_request",
		"operation": "read",
		"uri":       fmt.Sprintf("file:///%s", uriPath),
	}, nil
}

func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.Parse()

	toolcore.Start(&FileReaderTool{}, *transport, *port)
}
