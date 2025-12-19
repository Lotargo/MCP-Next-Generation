package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"mcp/shared/goutils/stdioutil"
	"mcp/shared/goutils/toolcore"
	pb "mcp/shared/proto"
)

// APICallerTool implements the toolcore.Tool interface.
type APICallerTool struct {
	pb.UnimplementedToolServer
}

// GetDescription returns the tool's description.
func (t *APICallerTool) GetDescription(ctx context.Context, in *pb.GetDescriptionRequest) (*pb.ToolDescription, error) {
	return &pb.ToolDescription{
		Name:            "api_caller",
		Description:     "Make HTTP requests (GET/POST). Args: url, method, json_body.",
		InputSchemaJson: `{"type":"object","properties":{"url":{"type":"string","description":"The full URL of the endpoint."},"method":{"type":"string","description":"HTTP method (GET, POST, etc)."},"headers":{"type":"object","description":"Headers map."},"json_body":{"type":"object","description":"JSON body for POST/PUT."}},"required":["url"]}`,
	}, nil
}

// Run executes the api_caller tool's logic for gRPC.
func (t *APICallerTool) Run(ctx context.Context, in *pb.ToolRunRequest) (*pb.ToolRunResponse, error) {
	args := in.Arguments.AsMap()
	resultData, err := t.performRequest(ctx, args)
	if err != nil {
		return &pb.ToolRunResponse{Error: err.Error()}, nil
	}
	resultValue, err := structpb.NewValue(resultData)
	if err != nil {
		return &pb.ToolRunResponse{Error: fmt.Sprintf("Error creating result value: %v", err)}, nil
	}
	return &pb.ToolRunResponse{Result: resultValue}, nil
}

// GetToolDefinition returns the definition for the stdio transport.
func (t *APICallerTool) GetToolDefinition() stdioutil.ToolDefinition {
	return stdioutil.ToolDefinition{
		Name:        "api_caller",
		Description: "Make HTTP requests (GET/POST). Args: url, method, json_body.",
		InputSchema: `{"type":"object","properties":{"url":{"type":"string"},"method":{"type":"string"},"headers":{"type":"object"},"json_body":{"type":"object"}},"required":["url"]}`,
	}
}

// StdioHandler handles the logic for the stdio transport.
func (t *APICallerTool) StdioHandler(arguments map[string]interface{}) (interface{}, error) {
	resultData, err := t.performRequest(context.Background(), arguments)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"result": resultData}, nil
}

// performRequest is the shared business logic.
func (t *APICallerTool) performRequest(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid or missing 'url' argument")
	}

	method := "GET"
	if m, ok := args["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	var reqBody []byte
	if jsonBody, ok := args["json_body"].(map[string]interface{}); ok {
		var err error
		reqBody, err = json.Marshal(jsonBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal json_body: %v", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	if headers, ok := args["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if vStr, ok := v.(string); ok {
				req.Header.Set(k, vStr)
			}
		}
	}
	if len(reqBody) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network or HTTP error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP Error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var resultData interface{}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if err := json.Unmarshal(bodyBytes, &resultData); err != nil {
		resultData = string(bodyBytes)
	}
	return resultData, nil
}

func main() {
	port := flag.Int("port", 0, "Port for gRPC server")
	transport := flag.String("transport", "grpc", "Transport protocol (grpc, stdio, http)")
	flag.Parse()

	toolcore.Start(&APICallerTool{}, *transport, *port)
}
