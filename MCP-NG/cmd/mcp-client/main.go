package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Shared JSON-RPC structures
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	runHttpClient()
}

func runHttpClient() {
	log.Println("--- Starting HTTP Client ---")
	endpoint := "http://127.0.0.1:8080"
	client := &http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Send initialize request
	initReq := Request{ID: 1, Method: "initialize"}
	respBody, sessionID := sendHttpRequest(ctx, client, endpoint, initReq, "")
	if respBody == nil {
		log.Fatalf("Initialize request failed")
	}

	var initResp Response
	if err := json.Unmarshal(respBody, &initResp); err != nil {
		log.Fatalf("Failed to unmarshal initialize response: %v\nBody: %s", err, string(respBody))
	}
	log.Printf("Initialize successful. Session ID: %s", sessionID)


	// 2. Send initialized notification
	notifReq := Request{Method: "notifications/initialized"} // No ID for notifications
	sendHttpRequest(ctx, client, endpoint, notifReq, sessionID)
	log.Println("Sent initialized notification.")


	// 3. Call calculator tool
	callParams := map[string]interface{}{"name": "calculator", "arguments": map[string]interface{}{"a": 40, "b": 2, "operation": "add"}}
	callReq := Request{JSONRPC: "2.0", ID: 4, Method: "tools/call", Params: callParams}
	respBody, _ = sendHttpRequest(ctx, client, endpoint, callReq, sessionID)
	if respBody == nil {
		log.Fatalf("tools/call request failed")
	}

	log.Printf("Raw tools/call response: %s", string(respBody))

	var callResp Response
	if err := json.Unmarshal(respBody, &callResp); err != nil {
		log.Fatalf("Failed to unmarshal tools/call response: %v\nBody: %s", err, string(respBody))
	}

	if callResp.Error != nil {
		log.Fatalf("Tool call failed with error: %+v", callResp.Error)
	}

	log.Printf("Tool call successful. Result: %s", string(callResp.Result))

	fmt.Println("HTTP client test finished.")
}

func sendHttpRequest(ctx context.Context, client *http.Client, endpoint string, req Request, sessionID string) ([]byte, string) {
	req.JSONRPC = "2.0" // Ensure version is set

	reqBytes, err := json.Marshal(req)
	if err != nil {
		log.Printf("Failed to marshal request for %s: %v", req.Method, err)
		return nil, ""
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Printf("Failed to create HTTP request for %s: %v", req.Method, err)
		return nil, ""
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer dummy-e2e-test-token")
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("Failed to send request for %s: %v", req.Method, err)
		return nil, ""
	}
	defer resp.Body.Close()

	newSessionID := resp.Header.Get("Mcp-Session-Id")
	if newSessionID == "" {
		newSessionID = sessionID // Preserve session ID if not returned
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body for %s: %v", req.Method, err)
		return nil, newSessionID
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// notification is a special case that returns 202 Accepted with no body
		if req.Method == "notifications/initialized" && resp.StatusCode == http.StatusAccepted {
			log.Println("Received 202 Accepted for notification, this is expected.")
			return nil, newSessionID
		}
		log.Printf("Received non-2xx status code for %s: %s\nBody: %s", req.Method, resp.Status, string(body))
		return nil, newSessionID
	}

	return body, newSessionID
}
