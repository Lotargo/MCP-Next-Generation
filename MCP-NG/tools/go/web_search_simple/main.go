package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"

	"google.golang.org/protobuf/types/known/structpb"
)

const serperAPIURL = "https://google.serper.dev/search"

// SerperSearchTool implements the toolcore.Tool interface.
type SerperSearchTool struct {
	pb.UnimplementedToolServer
	apiKey string
}

// GetDescription returns the tool's description.
func (t *SerperSearchTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "web_search_simple",
		Description:     "Simple web search (Serper). Good for quick facts. Args: query.",
		InputSchemaJson: `{"type":"object","properties":{"query":{"type":"string","description":"The search query."}},"required":["query"]}`,
	}, nil
}

// Run executes the web_search_simple tool's logic for gRPC.
func (t *SerperSearchTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	query, ok := in.Arguments.Fields["query"].AsInterface().(string)
	if !ok {
		return &pb.ToolRunResponse{Error: "Invalid or missing 'query' argument"}, nil
	}

	results, err := t.performSearch(ctx, query)
	if err != nil {
		return &pb.ToolRunResponse{Error: err.Error()}, nil
	}

	resultValue, err := structpb.NewValue(results)
	if err != nil {
		return &pb.ToolRunResponse{Error: fmt.Sprintf("Error creating result value: %v", err)}, nil
	}

	return &pb.ToolRunResponse{Result: resultValue}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *SerperSearchTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "web_search_simple",
		Description: "Simple web search (Serper). Good for quick facts. Args: query.",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
	}
}

// StdioHandler handles the logic for the stdio transport.
func (t *SerperSearchTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	query, ok := arguments["query"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid or missing 'query' argument")
	}

	results, err := t.performSearch(context.Background(), query)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"result": results}, nil
}

// performSearch is the shared business logic.
func (t *SerperSearchTool) performSearch(ctx context.Context, query string) (interface{}, error) {
	if t.apiKey == "" {
		return nil, fmt.Errorf("SERPER_API_KEY is not set")
	}

	requestBody := map[string]interface{}{
		"q": query,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serperAPIURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", t.apiKey)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("Serper API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var serperResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&serperResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Serper returns "organic" for standard results.
	results, ok := serperResponse["organic"]
	if !ok {
		// Fallback to "answer" or empty
		if answer, ok := serperResponse["answerBox"]; ok {
			return answer, nil
		}
		return []interface{}{}, nil
	}
	return results, nil
}

func main() {
	// Define a flag for the API key file path. The key in the flag name must be lowercase.
	apiKeyFile := flag.String("serper_api_key-from-file", "", "Path to a file containing the Serper API key.")

	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")

	flag.Parse()

	apiKey := ""
	if *apiKeyFile != "" {
		keyBytes, err := os.ReadFile(*apiKeyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: could not read API key file specified by --serper_api_key-from-file: %v\n", err)
			os.Exit(1)
		}
		apiKey = strings.TrimSpace(string(keyBytes))
	}

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "WARNING: Serper API key not provided. Tool will start but fail on execution.")
	}

	// Create an instance of the tool with the API key (potentially empty).
	tool := &SerperSearchTool{
		apiKey: apiKey,
	}

	// Start the tool's server.
	toolcore.Start(tool, *transport, *port)
}
