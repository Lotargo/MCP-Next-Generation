package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	logging "mcp/shared/goutils/logging"
)

const (
	targetServerURL = "http://127.0.0.1:8080"
	adminHeader     = "X-MCP-Admin-Key"
	tokenFile       = "env/ADMIN_TOKEN"
	envDir          = "env"
)

func generateToken() (string, error) {
	tokenBytes := make([]byte, 32) // 256-bit token
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(tokenBytes), nil
}

func getOrCreateToken() (string, error) {
	// 1. Try to read existing token
	if content, err := os.ReadFile(tokenFile); err == nil {
		token := string(bytes.TrimSpace(content))
		if token != "" {
			slog.Info("Loaded existing Admin Token from file", "path", tokenFile)
			return token, nil
		}
	}

	// 2. Generate new token
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	// 3. Ensure env directory exists
	if err := os.MkdirAll(envDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create env directory: %w", err)
	}

	// 4. Save token to file
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		return "", fmt.Errorf("failed to save token to file: %w", err)
	}

	slog.Info("Generated and saved new Admin Token", "path", tokenFile)
	return token, nil
}

func claimServer(token string) error {
	// Retry logic for claiming the server
	// The server might take a moment to start if launched simultaneously
	for i := 0; i < 10; i++ {
		resp, err := http.Post(targetServerURL+"/admin/init", "text/plain", bytes.NewBufferString(token))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				slog.Info("Successfully claimed MCP-NG server.")
				return nil
			} else if resp.StatusCode == 403 {
				// Already claimed.
				// If we reused the token from file, and it matches what the server has (if it persisted), we are good?
				// Actually, if the server is stateless regarding this or restarts, it will accept the new claim.
				// If the server is already running and claimed by US previously, calling /admin/init again with SAME token might be handled by server?
				// If calling with same token returns 200, great. If 403, we might need to assume it's us.
				// For now, let's just log warning on 403 but proceed if we loaded from file?
				// But we return error here. Let's stick to simple logic first.
				return fmt.Errorf("server already claimed (403 Forbidden)")
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("failed to connect to server at %s after retries", targetServerURL)
}

func main() {
	port := flag.Int("port", 8082, "Port to listen on")
	logFile := flag.String("log-file", "logs/gatekeeper.json", "Path to log file")
	flag.Parse()

	cleanup, err := logging.SetupLogger(*logFile)
	if err != nil {
		log.Fatalf("Failed to setup logger: %v", err)
	}
	defer cleanup()
	slog.Info("Logger initialized", "path", *logFile)

	// 1. Get or Generate Token
	token, err := getOrCreateToken()
	if err != nil {
		slog.Error("Failed to get admin token", "error", err)
		os.Exit(1)
	}
	slog.Info("Admin Token ready", "token_prefix", token[:8])

	// 2. Claim Server
	if err := claimServer(token); err != nil {
		slog.Error("Failed to claim server", "error", err, "target", targetServerURL)
		os.Exit(1)
	}

	// 3. Setup Proxy
	targetURL, err := url.Parse(targetServerURL)
	if err != nil {
		slog.Error("Failed to parse target URL", "error", err)
		os.Exit(1)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Customize the Director to inject the header
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set(adminHeader, token)

		// Trace ID
		traceID := req.Header.Get("X-Trace-Id")
		if traceID == "" {
			b := make([]byte, 16)
			rand.Read(b)
			traceID = hex.EncodeToString(b)
			req.Header.Set("X-Trace-Id", traceID)
		}

		// Log that we are proxying
		slog.Info("Proxying request", "method", req.Method, "path", req.URL.Path, "trace_id", traceID)
	}

	// 4. Start Server
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}

	slog.Info("Gatekeeper listening", "address", fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port))
	slog.Info("Connect your Admin Client (e.g. tool_manager) to THIS address.")

	if err := http.Serve(listener, proxy); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
