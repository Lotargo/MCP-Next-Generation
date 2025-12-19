// File: MCP-NG/server/cmd/server/main_test.go
package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "mcp/shared/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestMain runs before any other tests in this package.
func TestMain(m *testing.M) {
	// Discard logs during tests to keep output clean.
	logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	os.Exit(m.Run())
}

// startTestServer is a helper function that starts the main MCP gRPC server.
func startTestServer(t *testing.T) (string, func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0") // Use explicit IP to avoid IPv6 issues
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	mcpServer := newServer()

	// CRITICAL FIX: Convert the relative path to the project root into an absolute one.
	// This ensures that when discoverAndRunTools constructs paths to tool scripts,
	// they are absolute and can be correctly executed by subprocesses regardless of CWD.
	relativeProjectRoot := "../../../.."
	absProjectRoot, err := filepath.Abs(relativeProjectRoot)
	if err != nil {
		t.Fatalf("Failed to create absolute path for project root: %v", err)
	}

	go mcpServer.discoverAndRunTools(absProjectRoot)

	pb.RegisterMCPServer(grpcServer, mcpServer)
	addr := lis.Addr().String()
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			if err != grpc.ErrServerStopped {
				t.Logf("server exited with error: %v", err)
			}
		}
	}()
	return addr, func() {
		grpcServer.GracefulStop()
		mcpServer.cleanup()
	}
}

// TestMCPServer is the main integration test for the server.
func TestMCPServer(t *testing.T) {
	addr, stopServer := startTestServer(t)
	defer stopServer()

	// Use DialContext for better timeout handling.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // Block until the connection is up.
	)
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewMCPClient(conn)

	// Wait for the server and its tools to be ready.
	var calculatorReady bool
	for i := 0; i < 25; i++ { // Wait up to 25 seconds
		listResp, err := client.ListTools(ctx, &pb.ListToolsRequest{})
		if err == nil {
			for _, tool := range listResp.Tools {
				if tool.Name == "calculator" {
					calculatorReady = true
					break
				}
			}
		}
		if calculatorReady {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !calculatorReady {
		t.Fatal("timed out waiting for the 'calculator' tool to become available")
	}

	// Sub-test for the successful execution of the refactored calculator.
	t.Run("ExecuteTool_CalculatorSuccess", func(t *testing.T) {
		argsMap := map[string]interface{}{"expression": "(20 + 2) * 2"}
		argsStruct, err := structpb.NewStruct(argsMap)
		if err != nil {
			t.Fatalf("failed to create arguments struct: %v", err)
		}
		req := &pb.ExecuteToolRequest{
			ToolName:  "calculator",
			Arguments: argsStruct,
		}
		res, err := client.ExecuteTool(ctx, req)
		if err != nil {
			t.Fatalf("ExecuteTool failed: %v", err)
		}

		// The raw tool result is now a string. The adapter wraps it.
		// Expected structure: {"result": "44"}
		if res.Result == nil || res.Result.Fields == nil {
			t.Fatal("Result or its fields are nil")
		}
		resultValue, ok := res.Result.Fields["result"]
		if !ok {
			t.Fatal("Result struct does not contain 'result' field")
		}
		if resultValue.GetStringValue() != "44" {
			t.Errorf("Expected result to be string '44', got '%v'", resultValue.GetStringValue())
		}
	})

	// Sub-test for a tool that doesn't exist.
	t.Run("ExecuteTool_NotFound", func(t *testing.T) {
		req := &pb.ExecuteToolRequest{ToolName: "non_existent_tool"}
		_, err := client.ExecuteTool(ctx, req)
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected gRPC status code 'NotFound', but got '%s'", status.Code(err))
		}
	})

}
