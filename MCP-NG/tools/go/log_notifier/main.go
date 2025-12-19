package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"

	"google.golang.org/protobuf/types/known/structpb"
)

// LogNotifierTool implements the toolcore.Tool interface.
type LogNotifierTool struct {
	pb.UnimplementedToolServer
	logger *slog.Logger
}

// GetDescription returns the tool's description.
func (t *LogNotifierTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "log_notifier",
		Description:     "Log a status message/notification. Args: message, level (INFO/ERROR).",
		InputSchemaJson: `{"type":"object","properties":{"message":{"type":"string","description":"Message text."},"level":{"type":"string","description":"Log level (INFO/WARN/ERROR)."}},"required":["message"]}`,
	}, nil
}

// Run executes the log_notifier tool's logic for gRPC.
func (t *LogNotifierTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	message, ok := in.Arguments.Fields["message"].AsInterface().(string)
	if !ok {
		return &pb.ToolRunResponse{Error: "Invalid or missing 'message' argument"}, nil
	}
	levelStr := "INFO"
	if l, ok := in.Arguments.Fields["level"].AsInterface().(string); ok {
		levelStr = strings.ToUpper(l)
	}

	result, err := t.logNotification(ctx, message, levelStr)
	if err != nil {
		return &pb.ToolRunResponse{Error: err.Error()}, nil
	}

	resultValue, err := structpb.NewValue(result)
	if err != nil {
		return &pb.ToolRunResponse{Error: fmt.Sprintf("Error creating success message: %v", err)}, nil
	}

	return &pb.ToolRunResponse{Result: resultValue}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *LogNotifierTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "log_notifier",
		Description: "Log a status message/notification. Args: message, level (INFO/ERROR).",
		InputSchema: `{"type":"object","properties":{"message":{"type":"string"},"level":{"type":"string"}},"required":["message"]}`,
	}
}

// StdioHandler handles the logic for the stdio transport.
func (t *LogNotifierTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	message, ok := arguments["message"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid or missing 'message' argument")
	}
	levelStr := "INFO"
	if l, ok := arguments["level"].(string); ok {
		levelStr = strings.ToUpper(l)
	}

	result, err := t.logNotification(context.Background(), message, levelStr)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"result": result}, nil
}

// logNotification is the shared business logic.
func (t *LogNotifierTool) logNotification(ctx context.Context, message, levelStr string) (string, error) {
	var level slog.Level
	switch levelStr {
	case "INFO":
		level = slog.LevelInfo
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		t.logger.Warn("Invalid log level provided, defaulting to INFO", "provided_level", levelStr)
		level = slog.LevelInfo
	}

	t.logger.Log(ctx, level, message)
	return "Notification successfully logged.", nil
}

func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := &LogNotifierTool{logger: logger}
	toolcore.Start(tool, *transport, *port)
}
