package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// ServerConfig holds configuration for the test server
type ServerConfig struct {
	Transport string
	Port      int
}

// Helper to locate the server binary
func getServerBinaryPath(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	projectRoot := wd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.work")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Fatal("Could not find project root (go.work)")
		}
		projectRoot = parent
	}

	binPath := filepath.Join(projectRoot, "MCP-NG", "bin", "server")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Fatalf("Server binary not found at %s. Did you run ./setup/build.sh?", binPath)
	}
	return binPath
}

// startServer launches the server process and waits for it to be ready.
func startServer(t *testing.T, cfg ServerConfig) (string, func()) {
	binPath := getServerBinaryPath(t)

	args := []string{
		fmt.Sprintf("--transport=%s", cfg.Transport),
	}
	if cfg.Transport == "http" {
		args = append(args, fmt.Sprintf("--http-port=%d", cfg.Port))
		args = append(args, "--no-auth")
	} else {
		args = append(args, fmt.Sprintf("--grpc-port=%d", cfg.Port))
	}

	cmd := exec.Command(binPath, args...)

	setupProcessGroup(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set working directory to project root
	wd, _ := os.Getwd()
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(wd)))
	if filepath.Base(projectRoot) == "integration" {
		projectRoot, _ = filepath.Abs(filepath.Join(wd, "../../.."))
	}
	cmd.Dir = projectRoot

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	t.Logf("Server started with PID %d (PGID %d) on port %d", cmd.Process.Pid, cmd.Process.Pid, cfg.Port)

	// Cleanup function
	cleanup := func() {
		killProcessGroup(cmd)
		if cmd.Process != nil {
			cmd.Wait()
		}

		if t.Failed() {
			t.Logf("--- Server Stdout ---\n%s", stdout.String())
			t.Logf("--- Server Stderr ---\n%s", stderr.String())
		}
	}

	// Wait for server to be ready (poll /health)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
	healthURL := fmt.Sprintf("%s/health", baseURL)
	ready := false
	for i := 0; i < 100; i++ { // 10 seconds (100 * 100ms)
		resp, err := http.Get(healthURL)
		if err == nil {
			if resp.StatusCode == 200 {
				resp.Body.Close()
				ready = true
				break
			}
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ready {
		cleanup()
		t.Fatalf("Server failed to start within timeout.\nStderr: %s", stderr.String())
	}

	return baseURL, cleanup
}

// JSON-RPC Types
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int    `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// TestHTTPServer_Calculator verifies the basic HTTP/SSE flow and tool execution.
func TestHTTPServer_Calculator(t *testing.T) {
	port := 8096 // Use a distinct port
	baseURL, stop := startServer(t, ServerConfig{Transport: "http", Port: port})
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Initialize
	t.Log("Step 1: Initializing...")
	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  map[string]interface{}{"client_name": "go-integration-test"},
		ID:      1,
	}

	reqBody, _ := json.Marshal(initReq)
	req, _ := http.NewRequest("POST", baseURL, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Initialize request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Initialize returned status %d: %s", resp.StatusCode, string(body))
	}

	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("Server did not return Mcp-Session-Id")
	}
	t.Logf("Session ID: %s", sessionID)

	// 2. List Tools
	t.Log("Step 2: Listing tools...")
	listReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		Params:  map[string]interface{}{},
		ID:      2,
	}
	reqBody, _ = json.Marshal(listReq)
	req, _ = http.NewRequest("POST", baseURL, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("List tools request failed: %v", err)
	}
	defer resp.Body.Close()

	var listResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to decode list tools response: %v", err)
	}

	if listResp.Error != nil {
		t.Fatalf("List tools returned error: %v", listResp.Error)
	}

	// Verify calculator is present
	var resultParsed map[string]interface{}
	if err := json.Unmarshal(listResp.Result, &resultParsed); err == nil {
		if tools, ok := resultParsed["tools"].([]interface{}); ok {
			found := false
			for _, tool := range tools {
				tEnv := tool.(map[string]interface{})
				if tEnv["name"] == "calculator" {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("Calculator tool not found in tools list")
			}
		} else {
			t.Logf("Result structure might differ: %s", string(listResp.Result))
		}
	}

	// 3. Call Calculator
	t.Log("Step 3: Calling calculator...")
	callReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "calculator",
			"arguments": map[string]interface{}{
				"expression": "10 + 32",
			},
		},
		ID: 3,
	}
	reqBody, _ = json.Marshal(callReq)
	req, _ = http.NewRequest("POST", baseURL, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	req.Header.Set("X-Mcp-Sync", "true")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Call tool request failed: %v", err)
	}
	defer resp.Body.Close()

	var callResp JSONRPCResponse
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &callResp); err != nil {
		t.Fatalf("Failed to decode call response: %v\nBody: %s", err, string(bodyBytes))
	}

	if callResp.Error != nil {
		t.Fatalf("Call tool returned error: %+v", callResp.Error)
	}

	// Verify result "42"
	var contentResult map[string]interface{}
	if err := json.Unmarshal(callResp.Result, &contentResult); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	content, ok := contentResult["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("Result content missing or empty: %s", string(callResp.Result))
	}

	item := content[0].(map[string]interface{})
	text := item["text"].(string)

	if text != "42" {
		t.Errorf("Expected '42', got '%s'", text)
	}
	t.Log("Success: Calculator returned 42")
}
