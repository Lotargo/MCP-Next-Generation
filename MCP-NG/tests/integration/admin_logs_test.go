package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminLogs(t *testing.T) {
	port := 8097
	baseURL, stop := startServer(t, ServerConfig{Transport: "http", Port: port})
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Claim Server
	token := "test-admin-token"
	claimResp, err := client.Post(baseURL+"/admin/init", "text/plain", strings.NewReader(token))
	if err != nil {
		t.Fatalf("Failed to claim server: %v", err)
	}
	defer claimResp.Body.Close()
	if claimResp.StatusCode != 200 {
		t.Fatalf("Claim server failed: %d", claimResp.StatusCode)
	}

	// 2. Fetch Logs (Authorized)
	req, _ := http.NewRequest("GET", baseURL+"/admin/logs?count=10", nil)
	req.Header.Set("X-MCP-Admin-Key", token)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Get logs failed status: %d", resp.StatusCode)
	}

	var logs []string
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		t.Fatalf("Failed to decode logs: %v", err)
	}

	t.Logf("Received %d log lines", len(logs))

	// 3. Fetch Logs (Unauthorized)
	reqUnauth, _ := http.NewRequest("GET", baseURL+"/admin/logs", nil)
	respUnauth, err := client.Do(reqUnauth)
	if err != nil {
		t.Fatalf("Unauth request failed: %v", err)
	}
	defer respUnauth.Body.Close()
	if respUnauth.StatusCode != 401 {
		t.Errorf("Expected 401 for unauthorized logs request, got %d", respUnauth.StatusCode)
	}
}
