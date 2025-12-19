package stdioutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Public structures for tools to use
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema string
}

type ToolCallHandler func(arguments map[string]interface{}) (interface{}, error)

// --- Private JSON-RPC and MCP structures ---
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *jsonError  `json:"error,omitempty"`
}
type jsonError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      serverInfo   `json:"serverInfo"`
	Capabilities    capabilities `json:"capabilities"`
}
type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type capabilities struct {
	Tools map[string]interface{} `json:"tools"`
}
type toolDescription struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"inputSchemaJson"`
}
type toolRunParams struct {
	ToolName  string                 `json:"toolName"`
	Arguments map[string]interface{} `json:"arguments"`
}

var logger *slog.Logger

// Run starts the STDIO tool loop with proper MCP Content-Length framing.
func Run(def ToolDefinition, handler ToolCallHandler) {
	logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	logger.Info(fmt.Sprintf("%s tool started in standard MCP STDIO mode", def.Name))
	runStdioLoop(def, handler)
}

func runStdioLoop(def ToolDefinition, handler ToolCallHandler) {
	reader := bufio.NewReader(os.Stdin)
	for {
		// 1. Read headers to find Content-Length
		var contentLength int = -1
		for {
			line, err := reader.ReadString('\n')
			if err == io.EOF {
				return // Clean exit
			}
			if err != nil {
				logger.Error("Failed to read header line", "error", err)
				return
			}
			line = strings.TrimSpace(line)
			if line == "" { // End of headers
				break
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == "Content-Length" {
				length, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err != nil {
					logger.Error("Invalid Content-Length value", "value", parts[1])
					continue
				}
				contentLength = length
			}
		}

		if contentLength == -1 {
			logger.Warn("Received message without Content-Length header, skipping.")
			continue
		}

		// 2. Read the body of the message
		body := make([]byte, contentLength)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			logger.Error("Failed to read message body", "error", err)
			continue
		}

		// 3. Process the request
		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			logger.Error("Failed to unmarshal request", "error", err)
			continue
		}
		logger.Info("Received request", "method", req.Method, "id", req.ID)

		// 4. Dispatch and handle the request in a goroutine to allow concurrent processing
		go func(r request) {
			switch r.Method {
			case "initialize":
				handleInitialize(r.ID, def)
			case "tools/list":
				handleToolsList(r.ID, def)
			case "tools/call":
				var params toolRunParams
				if err := json.Unmarshal(r.Params, &params); err != nil {
					sendErrorResponse(r.ID, -32602, "Invalid params")
					return
				}
				handleToolCall(r.ID, params, handler)
			default:
				sendErrorResponse(r.ID, -32601, "Method not found")
			}
		}(req)
	}
}


func handleInitialize(id interface{}, def ToolDefinition) {
	result := initializeResult{
		ProtocolVersion: "2025-06-18",
		ServerInfo:      serverInfo{Name: fmt.Sprintf("MCP-NG %s", def.Name), Version: "1.2.0"},
		Capabilities:    capabilities{Tools: make(map[string]interface{})},
	}
	res := response{JSONRPC: "2.0", ID: id, Result: result}
	sendResponse(res)
}

func handleToolsList(id interface{}, def ToolDefinition) {
	desc := toolDescription{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: def.InputSchema,
	}
	res := response{JSONRPC: "2.0", ID: id, Result: []toolDescription{desc}}
	sendResponse(res)
}

func handleToolCall(id interface{}, params toolRunParams, handler ToolCallHandler) {
	result, err := handler(params.Arguments)
	if err != nil {
		sendErrorResponse(id, -32603, fmt.Sprintf("Error executing tool: %v", err))
		return
	}
	res := response{JSONRPC: "2.0", ID: id, Result: result}
	sendResponse(res)
}

// sendResponse marshals and sends a response with proper Content-Length framing.
func sendResponse(res response) {
	body, err := json.Marshal(res)
	if err != nil {
		logger.Error("Failed to marshal response", "error", err)
		// Send a valid error response if marshaling the original response fails
		errRes := response{JSONRPC: "2.0", ID: res.ID, Error: &jsonError{-32603, "Internal server error"}}
		body, _ = json.Marshal(errRes)
	}

	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	_, err = os.Stdout.WriteString(frame)
	if err != nil {
		logger.Error("Failed to write response to stdout", "error", err)
	}
}


func sendErrorResponse(id interface{}, code int, message string) {
	res := response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonError{Code: code, Message: message},
	}
	sendResponse(res)
}
