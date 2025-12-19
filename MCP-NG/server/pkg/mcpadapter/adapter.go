package mcpadapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"mcp/server/pkg/mcache"
	pb "mcp/shared/proto"

	"google.golang.org/protobuf/types/known/structpb"
)

// AgentManager defines an interface for components that manage agent connections.
type AgentManager interface {
	GetAgent(id string) (pb.ResourceAgentClient, bool)
	GetAgents() map[string]pb.ResourceAgentClient
	RemoveAgent(id string)
}

// ResourceOperationHandler is an interface that the main server can implement
// to handle resource operations triggered by tools (e.g., file I/O).
type ResourceOperationHandler interface {
	HandleResourceOperation(ctx context.Context, params *structpb.Struct) (interface{}, error)
}

// Adapter translates between JSON-RPC 2.0 and the internal gRPC server protocol.
type Adapter struct {
	grpcServer             pb.MCPServer
	agentManager           AgentManager
	resourceOpHandler      ResourceOperationHandler
	logger                 *slog.Logger
}

// NewAdapter creates a new adapter.
func NewAdapter(server pb.MCPServer, agentManager AgentManager, resourceHandler ResourceOperationHandler, logger *slog.Logger) *Adapter {
	return &Adapter{
		grpcServer:             server,
		agentManager:           agentManager,
		resourceOpHandler:      resourceHandler,
		logger:                 logger,
	}
}

// ServeStdio provides a compatibility layer for the existing stdio tests.
func (a *Adapter) ServeStdio(ctx context.Context, reader io.Reader, writer io.Writer, mcpServerAddr string) {
	bufReader := bufio.NewReader(reader)
	for {
		line, err := bufReader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				a.logger.Error("Error reading stdio request", "error", err)
			}
			break
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		respBytes := a.ProcessRequest(ctx, nil, line)
		if respBytes != nil {
			writer.Write(respBytes)
			writer.Write([]byte("\n"))
		}
	}
}

// ProcessRequest is the main entry point for handling a raw JSON-RPC request body.
func (a *Adapter) ProcessRequest(ctx context.Context, session *mcache.Session, body []byte) []byte {
	traceID, _ := ctx.Value("trace_id").(string)
	a.logger.Info("Processing request", "body_length", len(body), "has_session", session != nil, "trace_id", traceID)

	if len(body) == 0 {
		a.logger.Error("Received empty or invalid JSON-RPC request body", "trace_id", traceID)
		errResp := createErrorResponse(nil, -32700, "Empty request body")
		respBytes, _ := json.Marshal(errResp)
		return respBytes
	}

	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		a.logger.Error("Parse error", "error", err, "body", string(body), "trace_id", traceID)
		errResp := createErrorResponse(nil, -32700, "Parse error")
		respBytes, _ := json.Marshal(errResp)
		return respBytes
	}

	a.logger.Info("Parsed JSON-RPC request", "method", req.Method, "id", req.ID, "trace_id", traceID)

	if req.ID == nil {
		a.logger.Info("Received notification (no response expected)", "method", req.Method, "trace_id", traceID)
		return nil
	}

	resp := a.handleRequest(ctx, session, &req)
	if resp == nil {
		a.logger.Info("Asynchronous method - response will be sent via SSE", "method", req.Method, "trace_id", traceID)
		return nil
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		a.logger.Error("Internal error marshalling response", "error", err, "trace_id", traceID)
		errResp := createErrorResponse(req.ID, -32603, "Internal error marshalling response")
		respBytes, _ = json.Marshal(errResp)
		return respBytes
	}

	a.logger.Info("Returning synchronous response", "method", req.Method, "response_length", len(respBytes), "trace_id", traceID)
	return respBytes
}

// handleRequest routes the request to the appropriate handler.
func (a *Adapter) handleRequest(ctx context.Context, session *mcache.Session, req *Request) *Response {
	switch req.Method {
	case "initialize":
		return a.handleInitialize(ctx, session, req)
	case "tools/list":
		return a.handleListTools(ctx, session, req)
	case "tools/call":
		if session == nil {
			return a.handleExecuteToolSync(ctx, req)
		}
		go a.asyncExecuteTool(context.Background(), session, req)
		return nil
	case "resources/list":
		return a.handleListResources(ctx, session, req)
	case "resources/read":
		return a.handleReadResource(ctx, session, req)
	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			return nil
		}
		return createErrorResponse(req.ID, -32601, "Method not found: "+req.Method)
	}
}

// --- Handlers ---

func (a *Adapter) handleInitialize(_ context.Context, _ *mcache.Session, req *Request) *Response {
	var initParams struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	json.Unmarshal(req.Params, &initParams)

	supportedVersions := []string{"2024-11-05", "2025-03-26", "2025-06-18"}
	negotiatedVersion := "2024-11-05"

	for _, v := range supportedVersions {
		if v == initParams.ProtocolVersion {
			negotiatedVersion = v
			break
		}
	}

	a.logger.Info("Protocol version negotiated", "requested", initParams.ProtocolVersion, "negotiated", negotiatedVersion)

	capabilities := map[string]interface{}{"tools": map[string]interface{}{}, "resources": map[string]interface{}{}}
	if negotiatedVersion == "2024-11-05" {
		capabilities["tools"] = map[string]interface{}{"listChanged": true}
	}

	initResult := map[string]interface{}{
		"protocolVersion": negotiatedVersion,
		"serverInfo":      map[string]string{"name": "MCP-NG Meta-Server", "version": "2.1.0-http"},
		"capabilities":    capabilities,
	}

	return &Response{Version: "2.0", ID: req.ID, Result: initResult}
}

func (a *Adapter) handleListTools(ctx context.Context, _ *mcache.Session, req *Request) *Response {
	grpcResp, err := a.grpcServer.ListTools(ctx, &pb.ListToolsRequest{})
	if err != nil {
		return createErrorResponse(req.ID, -32000, "gRPC ListTools failed: "+err.Error())
	}

	var toolsForClient []map[string]interface{}
	for _, tool := range grpcResp.Tools {
		var schemaObj interface{}
		json.Unmarshal([]byte(tool.InputSchemaJson), &schemaObj)
		toolMap := map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": schemaObj,
		}
		toolsForClient = append(toolsForClient, toolMap)
	}

	return &Response{ID: req.ID, Version: "2.0", Result: map[string]interface{}{"tools": toolsForClient}}
}

func (a *Adapter) handleExecuteToolSync(ctx context.Context, req *Request) *Response {
	var params struct {
		Name      string          `json:"name"`
		ToolName  string          `json:"toolName"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return createErrorResponse(req.ID, -32602, "Invalid params for tools/call")
	}
	toolToCall := params.Name
	if toolToCall == "" {
		toolToCall = params.ToolName
	}
	argsStruct := &structpb.Struct{}
	if err := argsStruct.UnmarshalJSON(params.Arguments); err != nil {
		return createErrorResponse(req.ID, -32602, "Invalid arguments format")
	}

	grpcReq := &pb.ExecuteToolRequest{ToolName: toolToCall, Arguments: argsStruct}
	grpcResp, err := a.grpcServer.ExecuteTool(ctx, grpcReq)
	if err != nil {
		return createErrorResponse(req.ID, -32603, "Internal gRPC error: "+err.Error())
	}

	return a.processToolResult(ctx, req.ID, toolToCall, grpcResp.Result)
}

func (a *Adapter) handleListResources(ctx context.Context, _ *mcache.Session, req *Request) *Response {
	agents := a.agentManager.GetAgents()
	var allResources []map[string]interface{}

	for agentID, agentClient := range agents {
		resp, err := agentClient.ListResources(ctx, &pb.ListResourcesRequest{})
		if err != nil {
			a.logger.Warn("Failed to list resources from agent", "agent_id", agentID, "error", err)
			continue
		}
		for _, resource := range resp.Resources {
			allResources = append(allResources, map[string]interface{}{
				"uri":         resource.Uri,
				"description": resource.Description,
			})
		}
	}

	return &Response{ID: req.ID, Version: "2.0", Result: map[string]interface{}{"resources": allResources}}
}

func (a *Adapter) handleReadResource(ctx context.Context, _ *mcache.Session, req *Request) *Response {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return createErrorResponse(req.ID, -32602, "Invalid params for resources/read")
	}

	parts := strings.SplitN(strings.TrimPrefix(params.URI, "mcp://"), "/", 2)
	if len(parts) < 1 {
		return createErrorResponse(req.ID, -32602, "Invalid resource URI format")
	}
	agentID := parts[0]

	agent, ok := a.agentManager.GetAgent(agentID)
	if !ok {
		return createErrorResponse(req.ID, -32000, "Agent not found: "+agentID)
	}

	grpcResp, err := agent.ReadResource(ctx, &pb.ReadResourceRequest{Uri: params.URI})
	if err != nil {
		return createErrorResponse(req.ID, -32000, "Failed to read resource: "+err.Error())
	}

	contentBlock := map[string]interface{}{
		"uri":      params.URI,
		"mimeType": "application/octet-stream",
		"content":  grpcResp.Content,
	}

	return &Response{ID: req.ID, Version: "2.0", Result: map[string]interface{}{"contents": []interface{}{contentBlock}}}
}

// --- Asynchronous Handler ---

func (a *Adapter) asyncExecuteTool(ctx context.Context, session *mcache.Session, req *Request) {
	if session == nil {
		a.logger.Error("asyncExecuteTool called with nil session")
		return
	}

	var params struct {
		Name      string          `json:"name"`
		ToolName  string          `json:"toolName"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		a.sendSseResponse(session, createErrorResponse(req.ID, -32602, "Invalid params for tools/call"))
		return
	}
	toolToCall := params.Name
	if toolToCall == "" {
		toolToCall = params.ToolName
	}
	argsStruct := &structpb.Struct{}
	if err := argsStruct.UnmarshalJSON(params.Arguments); err != nil {
		a.sendSseResponse(session, createErrorResponse(req.ID, -32602, "Invalid arguments format"))
		return
	}

	a.logger.Info("Executing tool asynchronously", "tool", toolToCall)
	grpcReq := &pb.ExecuteToolRequest{ToolName: toolToCall, Arguments: argsStruct}
	grpcResp, err := a.grpcServer.ExecuteTool(ctx, grpcReq)

	if err != nil {
		a.logger.Error("Tool execution failed", "tool", toolToCall, "error", err)
		a.sendSseResponse(session, createErrorResponse(req.ID, -32603, "Internal gRPC error: "+err.Error()))
		return
	}

	finalResponse := a.processToolResult(ctx, req.ID, toolToCall, grpcResp.Result)
	a.sendSseResponse(session, finalResponse)
}

// processToolResult is the centralized logic for handling the outcome of a tool execution.
func (a *Adapter) processToolResult(ctx context.Context, id json.RawMessage, toolName string, result *structpb.Struct) *Response {
	if result == nil {
		a.logger.Warn("Tool returned a nil result, treating as functional error.", "tool", toolName)
		return createErrorResponse(id, -32001, "Tool returned no result for the given arguments.")
	}

	// Check for a functional error returned by the tool itself.
	if errVal, ok := result.Fields["error"]; ok {
		errorMsg := errVal.GetStringValue()
		a.logger.Warn("Tool returned a functional error", "tool", toolName, "error", errorMsg)
		return createErrorResponse(id, -32001, errorMsg)
	}

	// NEW: Check if the result is a resource operation request.
	if resultVal, ok := result.Fields["result"]; ok {
		if resultStruct := resultVal.GetStructValue(); resultStruct != nil {
			if typeVal, ok := resultStruct.Fields["_type"]; ok && typeVal.GetStringValue() == "resource_request" {
				a.logger.Info("Handling resource operation request from tool", "tool", toolName)
				opResult, err := a.resourceOpHandler.HandleResourceOperation(ctx, resultStruct)
				if err != nil {
					a.logger.Error("Resource operation failed", "tool", toolName, "error", err)
					return createErrorResponse(id, -32002, "Resource operation failed: "+err.Error())
				}

				// The operation result is now the final result.
				var textContent string

				// Helper to safely decode potential base64 content from resource operations (like file_reader)
				if opMap, ok := opResult.(map[string]interface{}); ok {
					if content, ok := opMap["content"].(string); ok {
						// The content is expected to be Base64 encoded by the main server.
						// We decode it to get the actual text for the LLM.
						decodedBytes, err := base64.StdEncoding.DecodeString(content)
						if err == nil {
							// Successfully decoded. Use this as the text.
							// Note: This assumes the file content is UTF-8 text.
							textContent = string(decodedBytes)
						} else {
							// If decoding fails, or it's not base64, fall back to the raw content.
							textContent = content
						}
					}
				}

				// If we successfully extracted and decoded text content, use it.
				// Otherwise, fall back to the generic string formatting.
				if textContent == "" {
					textContent = fmt.Sprintf("%v", opResult)
				}

				finalResult := map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": textContent}},
				}
				return &Response{ID: id, Version: "2.0", Result: finalResult}
			}
		}
	}

	// If it's not a resource operation, format it as a standard text response.
	resultText := formatToolResult(result)
	finalResult := map[string]interface{}{
		"content": []map[string]interface{}{{"type": "text", "text": resultText}},
	}
	return &Response{ID: id, Version: "2.0", Result: finalResult}
}

// --- Helper Functions ---

func (a *Adapter) sendSseResponse(session *mcache.Session, resp *Response) {
	if session == nil {
		a.logger.Error("Cannot send SSE response: session is nil")
		return
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		a.logger.Error("Failed to marshal SSE response", "error", err)
		return
	}

	a.logger.Debug("SSE Payload", "session_id", session.ID, "payload", string(respBytes))

	select {
	case session.SseChan <- respBytes:
		a.logger.Info("SSE response sent", "session_id", session.ID)
	default:
		a.logger.Error("SSE channel full or closed, could not send response", "session_id", session.ID)
	}
}

func createErrorResponse(id json.RawMessage, code int, message string) *Response {
	return &Response{ID: id, Version: "2.0", Error: &Error{Code: code, Message: message}}
}

func formatToolResult(result *structpb.Struct) string {
	if result == nil {
		return ""
	}
	if len(result.Fields) == 1 {
		for _, value := range result.Fields {
			switch v := value.GetKind().(type) {
			case *structpb.Value_StringValue:
				return v.StringValue
			case *structpb.Value_NumberValue:
				return fmt.Sprintf("%g", v.NumberValue)
			case *structpb.Value_BoolValue:
				return fmt.Sprintf("%t", v.BoolValue)
			default:
				jsonBytes, _ := value.MarshalJSON()
				return string(jsonBytes)
			}
		}
	}
	jsonBytes, _ := result.MarshalJSON()
	return string(jsonBytes)
}
