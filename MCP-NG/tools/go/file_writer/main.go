package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"google.golang.org/protobuf/types/known/structpb"
	"strings"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"
)

// FileWriterTool now acts as a proxy to the agent's file system capabilities.
type FileWriterTool struct {
	pb.UnimplementedToolServer
}

const schema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The full, absolute path of the file to write to, including filename and extension."
		},
		"content": {
			"type": "string",
			"description": "The content to write into the file."
		}
	},
	"required": ["path", "content"]
}`

// GetDescription returns the tool's description.
func (t *FileWriterTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "file_writer",
		Description:     "Write text to file. Args: path (absolute), content.",
		InputSchemaJson: schema,
	}, nil
}

// Run for gRPC now returns a resource request for a 'write' operation.
func (t *FileWriterTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	path, ok := in.Arguments.Fields["path"].GetKind().(*structpb.Value_StringValue)
	if !ok || path.StringValue == "" {
		return &pb.ToolRunResponse{Error: "Invalid or missing 'path' argument"}, nil
	}
	content, ok := in.Arguments.Fields["content"].GetKind().(*structpb.Value_StringValue)
	if !ok {
		// content can be an empty string, so we just check for presence
		return &pb.ToolRunResponse{Error: "Invalid or missing 'content' argument"}, nil
	}

	// Content is Base64 encoded to ensure safe transport
	encodedContent := base64.StdEncoding.EncodeToString([]byte(content.StringValue))

	// Ensure forward slashes for URI compatibility, especially on Windows
	uriPath := strings.ReplaceAll(path.StringValue, "\\", "/")

	resourceRequest, err := structpb.NewStruct(map[string]interface{}{
		"_type":     "resource_request",
		"operation": "write",
		"uri":       fmt.Sprintf("file:///%s", uriPath),
		"content":   encodedContent,
	})
	if err != nil {
		return &pb.ToolRunResponse{Error: "failed to create resource request"}, nil
	}

	return &pb.ToolRunResponse{Result: structpb.NewStructValue(resourceRequest)}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *FileWriterTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "file_writer",
		Description: "Write text to file. Args: path (absolute), content.",
		InputSchema: schema,
	}
}

// StdioHandler now also returns a resource request for a 'write' operation.
func (t *FileWriterTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	path, ok := arguments["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("missing or invalid 'path' argument")
	}
	content, ok := arguments["content"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'content' argument")
	}

	encodedContent := base64.StdEncoding.EncodeToString([]byte(content))

	// Ensure forward slashes for URI compatibility, especially on Windows
	uriPath := strings.ReplaceAll(path, "\\", "/")

	return map[string]interface{}{
		"_type":     "resource_request",
		"operation": "write",
		"uri":       fmt.Sprintf("file:///%s", uriPath),
		"content":   encodedContent,
	}, nil
}

func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.Parse()

	toolcore.Start(&FileWriterTool{}, *transport, *port)
}
