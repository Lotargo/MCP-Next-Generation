package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	pb "mcp/shared/proto"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"

	"google.golang.org/protobuf/types/known/structpb"
)

const tavilyAPIURL = "https://api.tavily.com/search"

// WebSearchTool implements the toolcore.Tool interface.
type WebSearchTool struct {
	pb.UnimplementedToolServer
	apiKey string
}

// GetDescription returns the tool's description.
func (t *WebSearchTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "web_search_tavily", // Renamed as requested
		Description:     "High-quality web search (Tavily). Best for complex questions. Use English queries only.",
		InputSchemaJson: `{"type":"object","properties":{"query":{"type":"string","description":"The search query (English only)."},"max_results":{"type":"number","description":"Max results (default 5)."}},"required":["query"]}`,
	}, nil
}

// Run executes the web_search tool's logic for gRPC.
func (t *WebSearchTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	query, ok := in.Arguments.Fields["query"].AsInterface().(string)
	if !ok {
		return &pb.ToolRunResponse{Error: "Invalid or missing 'query' argument"}, nil
	}

	maxResults := 5.0 // Default value
	if val, ok := in.Arguments.Fields["max_results"]; ok {
		maxResults = val.GetNumberValue()
	}

	results, err := t.performSearch(ctx, query, maxResults)
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
func (t *WebSearchTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "web_search_tavily",
		Description: "High-quality web search (Tavily). Best for complex questions. Use English queries only.",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string"},"max_results":{"type":"number"}},"required":["query"]}`,
	}
}

// StdioHandler handles the logic for the stdio transport.
func (t *WebSearchTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	query, ok := arguments["query"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid or missing 'query' argument")
	}

	maxResults := 5.0 // Default value
	if val, ok := arguments["max_results"].(float64); ok {
		maxResults = val
	}

	results, err := t.performSearch(context.Background(), query, maxResults)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"result": results}, nil
}

// performSearch is the shared business logic.
func (t *WebSearchTool) performSearch(ctx context.Context, query string, maxResults float64) (interface{}, error) {
	if t.apiKey == "" {
		return nil, fmt.Errorf("TAVILY_API_KEY is not set")
	}

	requestBody := map[string]interface{}{
		"api_key":      t.apiKey,
		"query":        query,
		"search_depth": "advanced",
		"max_results":  int(maxResults),
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tavilyAPIURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Tavily API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tavilyResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	results, ok := tavilyResponse["results"]
	if !ok {
		return []interface{}{}, nil
	}
	return results, nil
}

func main() {
	// Define a flag for the API key file path. The key in the flag name must be lowercase.
	apiKeyFile := flag.String("tavily_api_key-from-file", "", "Path to a file containing the Tavily API key.")

	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")

	flag.Parse()

	apiKey := ""
	if *apiKeyFile != "" {
		keyBytes, err := os.ReadFile(*apiKeyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: could not read API key file specified by --tavily_api_key-from-file: %v\n", err)
			os.Exit(1)
		}
		apiKey = strings.TrimSpace(string(keyBytes))
	}

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "WARNING: Tavily API key not provided. Tool will start but fail on execution.")
	}

	// Create an instance of the tool with the API key (potentially empty).
	tool := &WebSearchTool{
		apiKey: apiKey,
	}

	// Start the tool's server.
	toolcore.Start(tool, *transport, *port)
}
