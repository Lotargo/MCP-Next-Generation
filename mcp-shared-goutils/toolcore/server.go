package toolcore

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
)

// Tool defines the interface that each specific tool must implement.
type Tool interface {
	pb.ToolServer
	GetToolDefinition() stdioutil.ToolDefinition
	StdioHandler(args map[string]interface{}) (interface{}, error)
}

// Start launches the tool with the specified transport.
func Start(tool Tool, transport string, port int) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	switch transport {
	case "grpc":
		if port == 0 {
			logger.Error("A port must be specified for gRPC transport using --port")
			os.Exit(1)
		}
		runGrpcServer(port, tool, logger)
	case "stdio":
		def := tool.GetToolDefinition()
		stdioutil.Run(def, tool.StdioHandler)
	case "http":
		if port == 0 {
			logger.Error("A port must be specified for HTTP transport using --port")
			os.Exit(1)
		}
		runHttpServer(port, tool, logger)
	default:
		logger.Error("Invalid transport specified", "transport", transport)
		os.Exit(1)
	}
}

func runGrpcServer(port int, tool Tool, logger *slog.Logger) {
	address := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		logger.Error("gRPC: failed to listen", "error", err)
		os.Exit(1)
	}

	s := grpc.NewServer()
	pb.RegisterToolServer(s, tool)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("mcp.Tool", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s, healthServer)

	logger.Info("gRPC tool server listening", "address", address)
	if err := s.Serve(lis); err != nil {
		logger.Error("gRPC server failed", "error", err)
	}
}

func runHttpServer(port int, tool Tool, logger *slog.Logger) {
	// SSE logic will be implemented here in a future step.
	// For now, we'll just have a basic HTTP server.
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// Placeholder for SSE
		fmt.Fprintf(w, "SSE endpoint is not yet implemented.")
	})

	address := fmt.Sprintf(":%d", port)
	logger.Info("HTTP tool server listening", "address", address)
	if err := http.ListenAndServe(address, nil); err != nil {
		logger.Error("HTTP server failed", "error", err)
	}
}
