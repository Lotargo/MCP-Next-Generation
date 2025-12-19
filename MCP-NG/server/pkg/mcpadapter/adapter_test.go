package mcpadapter

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"

	pb "mcp/shared/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/grpc/status"
)

// --- Mock Resource Agent ---
type mockResourceAgentClient struct {
	agentID string
}

func (c *mockResourceAgentClient) ListResources(ctx context.Context, in *pb.ListResourcesRequest, opts ...grpc.CallOption) (*pb.ListResourcesResponse, error) {
	return &pb.ListResourcesResponse{
		Resources: []*pb.Resource{
			{Uri: fmt.Sprintf("mcp://%s/file/agent_file.txt", c.agentID)},
		},
	}, nil
}

func (c *mockResourceAgentClient) ReadResource(ctx context.Context, in *pb.ReadResourceRequest, opts ...grpc.CallOption) (*pb.ReadResourceResponse, error) {
	if in.Uri == fmt.Sprintf("mcp://%s/file/agent_file.txt", c.agentID) {
		return &pb.ReadResourceResponse{Content: []byte("content from agent")}, nil
	}
	return nil, status.Errorf(codes.NotFound, "resource not found")
}

// --- Mock Agent Manager ---
type mockAgentManager struct {
	agents map[string]pb.ResourceAgentClient
	mu     sync.RWMutex
}

func newMockAgentManager() *mockAgentManager {
	return &mockAgentManager{
		agents: make(map[string]pb.ResourceAgentClient),
	}
}

func (m *mockAgentManager) RegisterAgent(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[id] = &mockResourceAgentClient{agentID: id}
}

func (m *mockAgentManager) GetAgent(id string) (pb.ResourceAgentClient, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, ok := m.agents[id]
	return agent, ok
}

func (m *mockAgentManager) GetAgents() map[string]pb.ResourceAgentClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Return a copy to prevent race conditions
	agentsCopy := make(map[string]pb.ResourceAgentClient)
	for k, v := range m.agents {
		agentsCopy[k] = v
	}
	return agentsCopy
}

func (m *mockAgentManager) RemoveAgent(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, id)
}

// ResourcesReadParams defines the structure for the params of a resources/read request.
// This is defined here because it's only used in tests.
type ResourcesReadParams struct {
	URI string `json:"uri"`
}

// --- Mock MCP Server (for non-agent RPCs) ---
type mockMCPServer struct {
	pb.UnimplementedMCPServer
	ListToolsFunc func(ctx context.Context, in *pb.ListToolsRequest) (*pb.ListToolsResponse, error)
}

func (s *mockMCPServer) ListTools(ctx context.Context, in *pb.ListToolsRequest) (*pb.ListToolsResponse, error) {
	if s.ListToolsFunc != nil {
		return s.ListToolsFunc(ctx, in)
	}
	return &pb.ListToolsResponse{Tools: []*pb.ToolDescription{}}, nil
}

func (s *mockMCPServer) ExecuteTool(ctx context.Context, in *pb.ExecuteToolRequest) (*pb.ExecuteToolResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ExecuteTool not implemented in mock")
}

// HandleResourceOperation is a mock implementation.
func (s *mockMCPServer) HandleResourceOperation(ctx context.Context, params *structpb.Struct) (interface{}, error) {
	return nil, fmt.Errorf("HandleResourceOperation not implemented in mock")
}

// --- Test Harness ---
type testHarness struct {
	Adapter      *Adapter
	AgentManager *mockAgentManager
	Server       *mockMCPServer
	StdinWriter  *os.File
	StdoutReader *os.File
}

func runTestWithPipes(t *testing.T, testFunc func(t *testing.T, h *testHarness)) {
	stdinReader, stdinWriter, _ := os.Pipe()
	stdoutReader, stdoutWriter, _ := os.Pipe()

	mockServer := &mockMCPServer{}
	mockAgents := newMockAgentManager()
	// Create a logger that discards output for clean test logs
	nullLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	// Pass mockServer as the ResourceOperationHandler. For these tests, it can be nil
	// or an empty implementation, as the resource operation logic is not directly tested here.
	adapter := NewAdapter(mockServer, mockAgents, mockServer, nullLogger)

	harness := &testHarness{
		Adapter:      adapter,
		AgentManager: mockAgents,
		Server:       mockServer,
		StdinWriter:  stdinWriter,
		StdoutReader: stdoutReader,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Test in DIRECT SERVER mode by passing an empty server address.
		adapter.ServeStdio(context.Background(), stdinReader, stdoutWriter, "")
	}()

	testFunc(t, harness)

	// Cleanup
	stdinWriter.Close()
	stdoutWriter.Close()
	wg.Wait()
}

func TestToolsListInputSchemaIsObject(t *testing.T) {
	runTestWithPipes(t, func(t *testing.T, h *testHarness) {
		// Configure the mock server to return a tool with a stringified schema
		h.Server.ListToolsFunc = func(ctx context.Context, in *pb.ListToolsRequest) (*pb.ListToolsResponse, error) {
			return &pb.ListToolsResponse{
				Tools: []*pb.ToolDescription{
					{
						Name:            "test_tool",
						Description:     "A tool for testing.",
						InputSchemaJson: `{"type": "object", "properties": {"arg1": {"type": "string"}}}`,
					},
				},
			}, nil
		}

		// Write request for tools/list
		request := Request{Version: "2.0", ID: json.RawMessage(`"test-id-1"`), Method: "tools/list"}
		reqBytes, _ := json.Marshal(request)

		// Use newline-delimited JSON as it's simpler for testing
		h.StdinWriter.Write(reqBytes)
		h.StdinWriter.Write([]byte("\n"))

		// Read and verify response
		var response Response
		reader := bufio.NewReader(h.StdoutReader)

		// Read one line for the response
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("Failed to read response line: %v", err)
		}

		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("Failed to unmarshal response JSON: %v. Response was: %s", err, string(line))
		}

		if response.Error != nil {
			t.Fatalf("Expected success, got error: %+v", response.Error)
		}

		// Verify the structure of the Result
		// The `Result` field is already a map[string]interface{} after the initial unmarshal,
		// so we just need to assert its type.
		result, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected Result to be a map, but got %T", response.Result)
		}

		tools, ok := result["tools"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'tools' field to be a list, but it's not. Result: %v", string(response.Result.(json.RawMessage)))
		}

		if len(tools) != 1 {
			t.Fatalf("Expected 1 tool, got %d", len(tools))
		}

		tool, ok := tools[0].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected tool to be a map/object.")
		}

		inputSchema, ok := tool["inputSchema"]
		if !ok {
			t.Fatalf("Tool is missing 'inputSchema' field.")
		}

		// THIS IS THE KEY ASSERTION
		if _, isObject := inputSchema.(map[string]interface{}); !isObject {
			t.Fatalf("Expected 'inputSchema' to be a JSON object, but got type %T", inputSchema)
		}
	})
}

func TestServeStdioResourcesList(t *testing.T) {
	runTestWithPipes(t, func(t *testing.T, h *testHarness) {
		// Register a mock agent first
		h.AgentManager.RegisterAgent("agent1")

		// Write request
		request := Request{Version: "2.0", ID: json.RawMessage(`"4"`), Method: "resources/list"}
		reqBytes, _ := json.Marshal(request)
		h.StdinWriter.Write(reqBytes)
		h.StdinWriter.Write([]byte("\n"))

		// Read and verify response
		var response Response
		reader := bufio.NewReader(h.StdoutReader)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("Failed to read response line: %v", err)
		}
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v. Line: %s", err, string(line))
		}

		if response.Error != nil {
			t.Fatalf("Expected success, got error: %+v", response.Error)
		}

		resultMap, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("Result is not a map[string]interface{}, but %T", response.Result)
		}

		resources, ok := resultMap["resources"].([]interface{})
		if !ok {
			t.Fatalf("'resources' field is not a slice")
		}

		if len(resources) != 1 {
			t.Fatalf("Expected 1 resource, got %d", len(resources))
		}

		resource, ok := resources[0].(map[string]interface{})
		if !ok {
			t.Fatalf("Resource item is not a map")
		}

		if resource["uri"] != "mcp://agent1/file/agent_file.txt" {
			t.Errorf("Unexpected resource URI: %s", resource["uri"])
		}
	})
}

func TestServeStdioResourcesRead(t *testing.T) {
	runTestWithPipes(t, func(t *testing.T, h *testHarness) {
		// Register a mock agent
		h.AgentManager.RegisterAgent("agent1")

		// Write request
		uri := "mcp://agent1/file/agent_file.txt"
		params := ResourcesReadParams{URI: uri}
		paramsBytes, _ := json.Marshal(params)
		request := Request{Version: "2.0", ID: json.RawMessage(`"5"`), Method: "resources/read", Params: paramsBytes}
		reqBytes, _ := json.Marshal(request)
		h.StdinWriter.Write(reqBytes)
		h.StdinWriter.Write([]byte("\n"))

		// Read and verify response
		var response Response
		reader := bufio.NewReader(h.StdoutReader)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("Failed to read response line: %v", err)
		}
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v. Line: %s", err, string(line))
		}

		if response.Error != nil {
			t.Fatalf("Expected success, got error: %+v", response.Error)
		}

		resultMap, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("Result is not a map[string]interface{}, but %T", response.Result)
		}

		contents, ok := resultMap["contents"].([]interface{})
		if !ok {
			t.Fatalf("'contents' field is not a slice")
		}

		if len(contents) != 1 {
			t.Fatalf("Expected 1 content block, got %d", len(contents))
		}

		content, ok := contents[0].(map[string]interface{})
		if !ok {
			t.Fatalf("Content item is not a map")
		}

		// The content is returned as a base64 encoded string from the `json.Marshal` of a byte slice.
		expectedContent := base64.StdEncoding.EncodeToString([]byte("content from agent"))
		gotContent, _ := content["content"].(string)

		if gotContent != expectedContent {
			t.Errorf("Expected content '%s', got '%s'", expectedContent, gotContent)
		}
		if content["uri"] != uri {
			t.Errorf("Expected URI '%s', got '%s'", uri, content["uri"])
		}
	})
}
