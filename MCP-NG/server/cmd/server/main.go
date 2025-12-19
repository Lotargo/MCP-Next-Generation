package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"mcp/server/pkg/mcache"
	"mcp/server/pkg/mcpadapter"
	logging "mcp/shared/goutils/logging"
	pb "mcp/shared/proto"

	"encoding/base64"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"net/url"

	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

var logger *slog.Logger

type contextKey string

const sessionContextKey contextKey = "session"

// Custom header for internal admin communication
const adminHeader = "X-MCP-Admin-Key"

// --- Main Server Structure ---

type serverConfig struct {
	LogLevel       string `json:"log_level"`
	TracingEnabled bool   `json:"tracing_enabled"`
}

type server struct {
	pb.UnimplementedMCPServer
	mu         sync.RWMutex
	tools      map[string]*toolClient
	agents     map[string]*agentClient
	toolCmds   []*exec.Cmd
	isReady    bool
	shutdown   chan struct{}
	secrets    map[string]string
	tempFiles  []string
	adminToken string
	config     serverConfig
	configMu   sync.RWMutex
}

type toolClient struct {
	client       pb.ToolClient
	conn         *grpc.ClientConn
	healthClient grpc_health_v1.HealthClient
	description  *pb.ToolDescription
	status       grpc_health_v1.HealthCheckResponse_ServingStatus
}

type agentClient struct {
	client pb.ResourceAgentClient
	conn   *grpc.ClientConn
}

// --- AgentManager Interface Implementation ---

func (s *server) GetAgent(id string) (pb.ResourceAgentClient, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.agents[id]
	if !ok {
		return nil, false
	}
	return agent.client, true
}

func (s *server) GetAgents() map[string]pb.ResourceAgentClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agentClients := make(map[string]pb.ResourceAgentClient)
	for id, agent := range s.agents {
		agentClients[id] = agent.client
	}
	return agentClients
}

// --- ResourceOperationHandler Interface Implementation ---

func (s *server) HandleResourceOperation(ctx context.Context, params *structpb.Struct) (interface{}, error) {
	fields := params.AsMap()
	operation, _ := fields["operation"].(string)
	uri, _ := fields["uri"].(string)

	// Robust URI parsing for local file system operations
	var localPath string
	if strings.HasPrefix(uri, "file:///") {
		// Explicitly handle file:/// scheme to avoid url.Parse quirks with Windows paths
		localPath = strings.TrimPrefix(uri, "file:///")
		// If the path was URL-encoded (e.g. spaces), decode it.
		if decoded, err := url.PathUnescape(localPath); err == nil {
			localPath = decoded
		}
	} else {
		// Fallback for other schemes or malformed URIs
		parsedURL, err := url.Parse(uri)
		if err != nil {
			return nil, fmt.Errorf("invalid resource URI: %w", err)
		}
		localPath = parsedURL.Path
	}

	// Windows-specific fix: remove leading slash from "/C:/..." if present
	if runtime.GOOS == "windows" {
		// Check for pattern like "/C:" or "/c:"
		if len(localPath) >= 3 && localPath[0] == '/' && localPath[2] == ':' {
			localPath = localPath[1:]
		}
	}

	localPath = filepath.Clean(localPath)

	// Convert to absolute path to ensure we know exactly where we are operating
	absPath, err := filepath.Abs(localPath)
	if err == nil {
		localPath = absPath
	}

	switch operation {
	case "write":
		contentB64, ok := fields["content"].(string)
		if !ok {
			return nil, fmt.Errorf("missing or invalid 'content' for write operation")
		}
		content, err := base64.StdEncoding.DecodeString(contentB64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 content: %w", err)
		}

		// Create directory if it doesn't exist (improves UX significantly)
		dir := filepath.Dir(localPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory structure: %w", err)
		}

		if err := os.WriteFile(localPath, content, 0644); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}

		// Verify file existence to ensure operation success (critical for stability)
		if _, err := os.Stat(localPath); err != nil {
			return nil, fmt.Errorf("file write reported success but verification failed: %w", err)
		}

		logger.Info("Successfully wrote file via resource operation", "path", localPath)
		return fmt.Sprintf("File written successfully to %s", localPath), nil

	case "read":
		content, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		logger.Info("Successfully read file via resource operation", "path", localPath)
		// The protocol expects the content to be base64 encoded for 'read' as well,
		// to be consistent with how the security agent would transmit it.
		encodedContent := base64.StdEncoding.EncodeToString(content)
		return map[string]interface{}{
			"uri":      uri,
			"content":  encodedContent,
			"mimeType": "application/octet-stream",
		}, nil

	case "list":
		entries, err := os.ReadDir(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to list directory: %w", err)
		}
		var resources []map[string]interface{}
		for _, entry := range entries {
			resources = append(resources, map[string]interface{}{
				"uri":         fmt.Sprintf("file:///%s", filepath.Join(localPath, entry.Name())),
				"description": entry.Name(),
			})
		}
		logger.Info("Successfully listed directory via resource operation", "path", localPath)
		return map[string]interface{}{"resources": resources}, nil

	default:
		return nil, fmt.Errorf("unsupported resource operation: %s", operation)
	}
}

func (s *server) RemoveAgent(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if agent, ok := s.agents[id]; ok {
		agent.conn.Close()
		delete(s.agents, id)
		logger.Info("Agent removed", "agent_id", id)
	}
}

// --- Core Application Logic ---

func main() {
	transport := flag.String("transport", "http", "The transport protocol: http or stdio")
	httpPort := flag.Int("http-port", 8080, "Port for HTTP server")
	grpcPort := flag.Int("grpc-port", 50050, "Port for gRPC agent server")
	logFile := flag.String("log-file", "", "Path to log file. Default is logs/server.json.")
	noAuth := flag.Bool("no-auth", false, "Disable OAuth middleware for HTTP server.")
	authToken := flag.String("auth-token", "", "Static authorization token for clients. If empty and --no-auth is false, a random one is generated.")
	flag.Parse()

	// Determine starting path for project root search.
	// We first try the directory of the executable, which is robust for compiled binaries
	// regardless of where they are invoked from (e.g. LM Studio, C:\Users\...).
	// If that fails (e.g. go run), we fallback to current working directory ".".
	startPath := "."
	exePath, err := os.Executable()
	if err == nil {
		startPath = filepath.Dir(exePath)
	}

	projectRoot, err := findProjectRoot(startPath)
	if err != nil {
		// Fallback to CWD if executable path method failed to find go.work
		// (useful for 'go run' scenarios where exe is in /tmp)
		if startPath != "." {
			projectRoot, err = findProjectRoot(".")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to find project root: %v\n", err)
			os.Exit(1)
		}
	}
	// Convert to absolute path to prevent issues when launching tool subprocesses,
	// especially on Windows where the working directory might not be what's expected.
	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to convert project root to absolute path: %v\n", err)
		os.Exit(1)
	}

	logPath := *logFile
	if logPath == "" {
		logPath = filepath.Join(projectRoot, "logs", "server.json")
	}

	cleanup, err := logging.SetupLogger(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	logger = slog.Default()
	logger.Info("Logger initialized", "path", logPath)
	logger.Info("Determined absolute project root", "path", projectRoot)

	startBanCleanup()

	switch *transport {
	case "http":
		runHttpMetaServer(projectRoot, *httpPort, *grpcPort, *noAuth, *authToken)
	case "stdio":
		runStdioServerLoop(projectRoot) // Use the new persistent loop mode.
	default:
		logger.Error("Invalid transport specified", "transport", *transport)
		os.Exit(1)
	}
}

// --- Mode 1: HTTP Meta-Server ---

func runHttpMetaServer(projectRoot string, httpPort, grpcPort int, noAuth bool, authToken string) {
	logger.Info("Starting in HTTP Meta-Server mode...")
	mcpServer := newServer()

	// We no longer load ADMIN_TOKEN from file by default for protection.
	// It's set via /admin/init.
	// However, to support legacy/manual tool runs without gatekeeper, we can check.
	// But the new spec says gatekeeper claims it.
	// Let's leave the file loading as a fallback for now, but prioritize the CLAIM.

	adapter := mcpadapter.NewAdapter(mcpServer, mcpServer, mcpServer, logger)
	sessionCache := mcache.NewCache()

	go mcpServer.discoverAndRunTools(projectRoot)
	defer mcpServer.cleanup()

	go startGrpcAgentServer(grpcPort, mcpServer)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		mcpServer.mu.RLock()
		defer mcpServer.mu.RUnlock()
		if mcpServer.isReady {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Service Unavailable"))
		}
	})

	// NEW: Admin Claim Endpoint
	mux.HandleFunc("/admin/init", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		mcpServer.mu.Lock()
		defer mcpServer.mu.Unlock()

		if mcpServer.adminToken != "" {
			logger.Warn("Attempt to claim already claimed server")
			http.Error(w, "Server already claimed", http.StatusForbidden)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusInternalServerError)
			return
		}
		token := strings.TrimSpace(string(body))
		if token == "" {
			http.Error(w, "Empty token", http.StatusBadRequest)
			return
		}

		mcpServer.adminToken = token
		// Update secrets map in memory so tools can use it immediately
		mcpServer.secrets["ADMIN_TOKEN"] = token

		// Save to env/ADMIN_TOKEN for tool subprocesses to pick it up
		tokenPath := filepath.Join(projectRoot, "env", "ADMIN_TOKEN")
		if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
			logger.Error("Failed to persist admin token to file", "error", err)
			// We continue, but tools might fail if they restart
		} else {
			logger.Info("Persisted ADMIN_TOKEN for tools")
		}

		// Trigger hot reload to launch tools that were waiting for the token
		go mcpServer.discoverAndRunTools(projectRoot)

		logger.Info("Server successfully claimed by Gatekeeper")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Claimed"))
	})

	// NEW: Admin endpoint for tool deletion
	mux.HandleFunc("/admin/tools/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Authorization Check (X-MCP-Admin-Key)
		if !checkAdminToken(r, mcpServer.adminToken) {
			logger.Warn("Unauthorized attempt to access /admin/tools/delete", "remote_addr", r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		toolName := r.URL.Query().Get("name")
		if toolName == "" {
			http.Error(w, "Missing 'name' parameter", http.StatusBadRequest)
			return
		}

		// Path Traversal Protection
		// Allow only alphanumeric characters and underscores.
		validName := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
		if !validName.MatchString(toolName) {
			logger.Warn("Invalid tool name rejected", "tool_name", toolName)
			http.Error(w, "Invalid tool name", http.StatusBadRequest)
			return
		}

		if err := mcpServer.deleteTool(toolName, projectRoot); err != nil {
			logger.Error("Failed to delete tool", "tool", toolName, "error", err)
			http.Error(w, fmt.Sprintf("Failed to delete tool: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Tool '%s' deleted and unregistered.", toolName)))
	})

	// NEW: Admin Config
	mux.HandleFunc("/admin/config", func(w http.ResponseWriter, r *http.Request) {
		if !checkAdminToken(r, mcpServer.adminToken) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		mcpServer.configMu.Lock()
		defer mcpServer.configMu.Unlock()

		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mcpServer.config)
			return
		}

		if r.Method == http.MethodPost {
			var newConfig serverConfig
			if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}

			// Set Level
			var level slog.Level
			switch strings.ToUpper(newConfig.LogLevel) {
			case "DEBUG":
				level = slog.LevelDebug
			case "WARN":
				level = slog.LevelWarn
			case "ERROR":
				level = slog.LevelError
			default:
				level = slog.LevelInfo
				newConfig.LogLevel = "INFO"
			}
			logging.SetLevel(level)

			mcpServer.config.LogLevel = newConfig.LogLevel
			mcpServer.config.TracingEnabled = newConfig.TracingEnabled

			logger.Info("Configuration updated", "config", mcpServer.config)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mcpServer.config)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// NEW: Admin endpoint for tool scanning (Hot Reload)
	mux.HandleFunc("/admin/tools/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Authorization Check
		if !checkAdminToken(r, mcpServer.adminToken) {
			logger.Warn("Unauthorized attempt to access /admin/tools/scan", "remote_addr", r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		logger.Info("Triggering manual tool discovery via admin API")
		// We run discovery synchronously for simplicity.
		mcpServer.discoverAndRunTools(projectRoot)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Tool scan completed."))
	})

	// NEW: Admin Logs (Dynamic Polling)
	mux.HandleFunc("/admin/logs", func(w http.ResponseWriter, r *http.Request) {
		if !checkAdminToken(r, mcpServer.adminToken) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		level := r.URL.Query().Get("level")
		countStr := r.URL.Query().Get("count")
		count := 50
		if countStr != "" {
			fmt.Sscanf(countStr, "%d", &count)
		}

		logPath := filepath.Join(projectRoot, "logs", "server.json")

		logs, err := readLastLogs(logPath, count, level)
		if err != nil {
			logger.Warn("Failed to read logs", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logs)
	})

	// Combined handler for the root endpoint
	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleSseConnection(w, r, sessionCache)
		case http.MethodPost:
			// POST requests are wrapped in session management
			sessionMiddleware(sessionCache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlePostMessage(w, r, adapter)
			})).ServeHTTP(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	var finalHandler http.Handler = rootHandler
	if !noAuth {
		token := authToken
		if token == "" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				logger.Error("Failed to generate random token", "error", err)
				os.Exit(1)
			}
			token = hex.EncodeToString(b)
			logger.Info("Auth enabled but no token provided. Generated random token", "token", token)
		}
		finalHandler = oauthMiddleware(rootHandler, token)
		logger.Info("Basic Authentication is ENABLED.")
	} else {
		logger.Warn("Authentication is DISABLED (--no-auth).")
	}

	mux.Handle("/", finalHandler)

	httpAddr := fmt.Sprintf(":%d", httpPort)
	logger.Info("HTTP Meta-Server listening", "address", httpAddr)

	// Wrap the main mux with the request logging middleware
	loggingMux := requestLogger(mux, mcpServer)

	if err := http.ListenAndServe(httpAddr, loggingMux); err != nil {
		logger.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}

func checkAdminToken(r *http.Request, expectedToken string) bool {
	// If server is not claimed yet, block everything
	if expectedToken == "" {
		return false
	}
	token := r.Header.Get(adminHeader)
	return token == expectedToken
}

// --- Mode 2: Stdio Server Loop ---

func runStdioServerLoop(projectRoot string) {
	logger.Info("Starting in persistent stdio server mode...")

	mcpServer := newServer()
	adapter := mcpadapter.NewAdapter(mcpServer, mcpServer, mcpServer, logger)

	// --- CHANGE: Add channel to wait for readiness ---
	readyChan := make(chan struct{})

	// Pass the channel to the tool discovery function.
	go func() {
		mcpServer.discoverAndRunTools(projectRoot)
		// After tools are discovered, close the channel to signal readiness.
		close(readyChan)
	}()
	defer mcpServer.cleanup()

	// --- CHANGE: Block execution until signal received ---
	logger.Info("Waiting for tools to be discovered...")
	<-readyChan // Wait here until discoverAndRunTools closes the channel.
	logger.Info("Tools discovered. Ready to process stdio requests.")

	// Now that the server is ready, start processing requests.
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				logger.Info("Stdio client disconnected (EOF). Shutting down.")
			} else {
				logger.Error("Error reading from stdin", "error", err)
			}
			break
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// In stdio mode, we might need to handle visibility too, but typically stdio is single user/admin.
		// For now, we pass requests as is.
		respBytes := adapter.ProcessRequest(context.Background(), nil, line)
		if respBytes != nil {
			os.Stdout.Write(respBytes)
			os.Stdout.Write([]byte("\n"))
			os.Stdout.Sync() // Use Sync to send data immediately
		}
	}
}

// --- HTTP Handlers ---

func handleSseConnection(w http.ResponseWriter, r *http.Request, cache *mcache.Cache) {
	sessionID := r.Header.Get("Mcp-Session-Id")

	var session *mcache.Session

	// If no session - create new one
	if sessionID == "" {
		session = cache.NewSession()
		w.Header().Set("Mcp-Session-Id", session.ID)
		logger.Info("Created new SSE session", "session_id", session.ID)
	} else {
		// If exists - use existing one
		var ok bool
		session, ok = cache.GetSession(sessionID)
		if !ok {
			http.Error(w, "Invalid or expired session ID", http.StatusForbidden)
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	traceID, _ := r.Context().Value("trace_id").(string)
	logger.Info("SSE client connected", "session_id", session.ID, "trace_id", traceID)
	defer logger.Info("SSE client disconnected", "session_id", session.ID, "trace_id", traceID)
	defer cache.DeleteSession(session.ID)

	ctx := r.Context()
	for {
		select {
		case msg, ok := <-session.SseChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func handlePostMessage(w http.ResponseWriter, r *http.Request, adapter *mcpadapter.Adapter) {
	sessionVal := r.Context().Value(sessionContextKey)
	session, ok := sessionVal.(*mcache.Session)
	if !ok {
		logger.Error("Could not find session in context")
		http.Error(w, "Could not find session in context", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Error reading request body in handlePostMessage", "error", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	traceID, _ := r.Context().Value("trace_id").(string)
	logger.Info("Processing POST message", "session_id", session.ID, "body_length", len(body), "trace_id", traceID)

	if len(body) == 0 {
		logger.Warn("Received empty body in handlePostMessage")
		http.Error(w, "Empty request body", http.StatusBadRequest)
		return
	}

	// Pass the request context, which might contain headers, down to the adapter
	// We need to inject the Admin Token status into the context for ListTools filtering
	ctx := r.Context()
	adminToken := r.Header.Get(adminHeader)
	if adminToken != "" {
		ctx = context.WithValue(ctx, "adminToken", adminToken)
	}

	// Check for Sync override
	useSync := r.Header.Get("X-Mcp-Sync") == "true"
	var sessionToUse *mcache.Session = session
	if useSync {
		sessionToUse = nil
		logger.Info("Forcing synchronous execution via X-Mcp-Sync header", "trace_id", traceID)
	}

	respBytes := adapter.ProcessRequest(ctx, sessionToUse, body)

	if respBytes == nil {
		// Asynchronous response - will be sent via SSE
		logger.Info("Asynchronous request - response will be sent via SSE", "session_id", session.ID)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
	logger.Info("Sent synchronous response", "session_id", session.ID, "response_length", len(respBytes))
}

// --- Server & Tool Management ---

func newServer() *server {
	return &server{
		tools:     make(map[string]*toolClient),
		agents:    make(map[string]*agentClient),
		toolCmds:  make([]*exec.Cmd, 0),
		shutdown:  make(chan struct{}),
		isReady:   false,
		secrets:   make(map[string]string),
		tempFiles: make([]string, 0),
		config: serverConfig{
			LogLevel:       "INFO",
			TracingEnabled: true,
		},
	}
}

func (s *server) cleanup() {
	logger.Info("Cleaning up tool subprocesses...")
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cmd := range s.toolCmds {
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				logger.Error("Failed to kill process", "pid", cmd.Process.Pid, "error", err)
			}
		}
	}

	logger.Info("Removing temporary secret files...")
	for _, f := range s.tempFiles {
		err := os.Remove(f)
		if err != nil {
			logger.Error("Failed to remove temporary file", "path", f, "error", err)
		}
	}
}

func startGrpcAgentServer(port int, mcpServer *server) {
	grpcAddr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Error("Failed to listen for gRPC", "address", grpcAddr, "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterMCPServer(grpcServer, mcpServer)
	reflection.Register(grpcServer)
	logger.Info("gRPC agent server listening", "address", grpcAddr)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Error("gRPC server failed", "error", err)
	}
}

type toolConfig struct {
	Command         []string `json:"command"`
	RequiredSecrets []string `json:"required_secrets"`
}

func (s *server) loadSecrets(projectRoot string) {
	secretsDir := filepath.Join(projectRoot, "env")
	files, err := os.ReadDir(secretsDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("Secrets directory 'env/' not found, skipping secret loading.")
			return
		}
		logger.Error("Failed to read secrets directory", "path", secretsDir, "error", err)
		return
	}

	for _, file := range files {
		if !file.IsDir() {
			secretName := file.Name()
			content, err := os.ReadFile(filepath.Join(secretsDir, secretName))
			if err != nil {
				logger.Error("Failed to read secret file", "file", secretName, "error", err)
				continue
			}
			s.secrets[secretName] = strings.TrimSpace(string(content))
			logger.Info("Loaded secret", "name", secretName)
		}
	}
}

func (s *server) launchTool(projectRoot, toolName, toolPath, configPath string) {
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		logger.Error("Failed to read tool config file", "path", configPath, "error", err)
		return
	}

	var config toolConfig
	if err := json.Unmarshal(configFile, &config); err != nil {
		logger.Error("Failed to parse tool config file", "path", configPath, "error", err)
		return
	}

	if len(config.Command) > 0 {
		executable := config.Command[0]
		args := config.Command[1:]
		freePort, _ := findFreePort()

		// Secret Handling
		for _, secretName := range config.RequiredSecrets {
			s.mu.RLock()
			secretValue, ok := s.secrets[secretName]
			s.mu.RUnlock()

			if !ok {
				logger.Error("Tool requires a secret that was not loaded, skipping tool", "tool", toolName, "secret", secretName)
				return
			}

			// Create temporary file for secret
			tmpFile, err := os.CreateTemp("", "secret-"+toolName+"-")
			if err != nil {
				logger.Error("Failed to create temporary file for secret", "tool", toolName, "error", err)
				return
			}
			defer tmpFile.Close() // Close file handle

			if _, err := tmpFile.WriteString(secretValue); err != nil {
				logger.Error("Failed to write secret to temporary file", "tool", toolName, "error", err)
				return
			}
			tmpFile.Close() // Close immediately so tool can read it

			s.mu.Lock()
			s.tempFiles = append(s.tempFiles, tmpFile.Name())
			s.mu.Unlock()

			// Add CLI argument for the tool
			secretArg := fmt.Sprintf("--%s-from-file=%s", strings.ToLower(secretName), tmpFile.Name())
			args = append(args, secretArg)
			logger.Info("Passing secret to tool via temporary file", "tool", toolName, "secret", secretName, "arg", secretArg)
		}

		// The command from config.json is now treated as the source of truth.
		// For Go tools, the path to the binary is constructed.
		// For Python tools, the command is expected to be fully formed (e.g., ["python", "-m", "poetry", ...]).
		isGoTool := !strings.Contains(filepath.ToSlash(toolPath), "/tools/python/")

		if isGoTool {
			executable = filepath.Join(projectRoot, "MCP-NG", "bin", executable)
			if runtime.GOOS == "windows" {
				executable += ".exe"
			}
		}

		args = append(args, fmt.Sprintf("--port=%d", freePort))
		cmd := exec.Command(executable, args...)
		// FINAL, UNIFIED FIX: Always run the tool from its own directory.
		cmd.Dir = toolPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			logger.Error("Failed to start tool", "tool", toolName, "error", err)
			return
		}

		s.mu.Lock()
		s.toolCmds = append(s.toolCmds, cmd)
		s.mu.Unlock()

		// Flag to track successful registration for cleanup logic
		registered := false
		defer func() {
			if !registered {
				if cmd.Process != nil {
					cmd.Process.Kill()
					// Spawn goroutine to wait/cleanup zombie if we failed to register
					go cmd.Wait()
				}
			}
		}()

		addr := fmt.Sprintf("127.0.0.1:%d", freePort)
		var conn *grpc.ClientConn
		var connErr error

		// Robust connection logic with retries and blocking
		// Set a total timeout of 15 seconds for all attempts.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Use WithBlock so Dial waits for the connection to be established.
		conn, connErr = grpc.DialContext(ctx, addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)

		if connErr != nil {
			logger.Error("Failed to connect to tool after multiple retries", "tool", toolName, "address", addr, "error", connErr)
			return
		}

		client := pb.NewToolClient(conn)
		healthClient := grpc_health_v1.NewHealthClient(conn)
		desc, err := client.GetDescription(context.Background(), &pb.GetDescriptionRequest{})
		if err != nil || desc == nil {
			logger.Error("Failed to get description from tool, cannot register.", "tool", toolName, "error", err)
			return
		}

		s.mu.Lock()
		s.tools[desc.Name] = &toolClient{
			client:       client,
			conn:         conn,
			healthClient: healthClient,
			description:  desc,
			status:       grpc_health_v1.HealthCheckResponse_SERVING,
		}
		s.mu.Unlock()
		registered = true
		logger.Info("Successfully registered tool", "tool", desc.Name)

		// Monitor process exit to prevent zombies and track failures
		go func() {
			err := cmd.Wait()
			logger.Error("Tool process terminated unexpectedly", "tool", desc.Name, "error", err)
			s.mu.Lock()
			if t, ok := s.tools[desc.Name]; ok {
				t.conn.Close()
				delete(s.tools, desc.Name)
			}
			s.mu.Unlock()
		}()
	}
}

func (s *server) discoverAndRunTools(projectRoot string) {
	s.loadSecrets(projectRoot) // Load secrets before starting tools

	toolDirs := []string{
		filepath.Join(projectRoot, "MCP-NG/tools/go"),
		filepath.Join(projectRoot, "MCP-NG/tools/python"),
	}
	logger.Info("Starting automatic tool discovery (Parallel)", "search_paths", toolDirs)

	var wg sync.WaitGroup

	for _, dir := range toolDirs {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip if access error
			}
			if info.IsDir() && filepath.Dir(path) == dir {
				toolName := filepath.Base(path)
				toolPath := path

				// Check if tool is already registered to prevent duplicate launches
				s.mu.RLock()
				_, exists := s.tools[toolName]
				s.mu.RUnlock()
				if exists {
					return nil // Skip already running tools
				}

				configPath := filepath.Join(path, "config.json")
				if _, err := os.Stat(configPath); os.IsNotExist(err) {
					return nil // Skip if no config.json
				}

				wg.Add(1)
				go func() {
					defer wg.Done()
					s.launchTool(projectRoot, toolName, toolPath, configPath)
				}()
			}
			return nil
		})
	}
	wg.Wait()

	s.mu.Lock()
	s.isReady = true
	s.mu.Unlock()
	logger.Info("Meta-server is ready to accept connections.")
}

// --- gRPC Method Implementations ---

func (s *server) RegisterAgent(ctx context.Context, in *pb.AgentInfo) (*pb.RegisterAgentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// deleteTool handles the unregistration and deletion of a tool.
func (s *server) deleteTool(toolName, projectRoot string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Check if tool exists in memory
	_, ok := s.tools[toolName]
	if !ok {
		return fmt.Errorf("tool '%s' not found/loaded", toolName)
	}

	// 2. Stop the process (if we have a handle to it)
	// We store processes in s.toolCmds list, but it's a slice, not a map.
	// This is a design limitation. We have to iterate to find the matching process.
	// We can identify the process by matching the executable path or arguments?
	// Or better: We should store the *exec.Cmd in the toolClient struct!
	// Since we didn't refactor toolClient to hold cmd, we iterate s.toolCmds.
	// But wait, s.toolCmds holds *all* cmds. How do we know which one belongs to the tool?
	// We don't strictly know without refactoring.
	// However, since we are deleting the files, the next restart won't find it.
	// BUT, to support hot-unload, we MUST kill the specific process.
	//
	// Let's try to find it by matching the command working directory (cmd.Dir).
	// The discovery logic sets cmd.Dir = path to tool directory.
	// The tool directory name usually matches the tool name.

	foundCmd := -1
	for i, cmd := range s.toolCmds {
		if filepath.Base(cmd.Dir) == toolName {
			if cmd.Process != nil {
				logger.Info("Killing tool process", "tool", toolName, "pid", cmd.Process.Pid)
				if err := cmd.Process.Kill(); err != nil {
					logger.Error("Failed to kill process", "tool", toolName, "error", err)
					// Continue anyway to delete files
				}
			}
			foundCmd = i
			break
		}
	}

	// Remove from toolCmds slice if found
	if foundCmd != -1 {
		s.toolCmds = append(s.toolCmds[:foundCmd], s.toolCmds[foundCmd+1:]...)
	}

	// 3. Remove from memory map
	delete(s.tools, toolName)

	// 4. Delete files from disk
	// We need to check both Go and Python directories
	goPath := filepath.Join(projectRoot, "MCP-NG/tools/go", toolName)
	pyPath := filepath.Join(projectRoot, "MCP-NG/tools/python", toolName)

	// Check and remove Go path
	if _, err := os.Stat(goPath); err == nil {
		// Remove from go.work first to prevent build breakage
		relPath := fmt.Sprintf("./MCP-NG/tools/go/%s", toolName)
		logger.Info("Removing tool from go.work", "path", relPath)
		cmd := exec.Command("go", "work", "edit", fmt.Sprintf("-dropuse=%s", relPath))
		cmd.Dir = projectRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			logger.Error("Failed to update go.work", "error", err, "output", string(out))
			// We proceed anyway to ensure files are deleted
		}

		logger.Info("Removing tool directory (Go)", "path", goPath)
		if err := os.RemoveAll(goPath); err != nil {
			return fmt.Errorf("failed to remove directory %s: %w", goPath, err)
		}
	}

	// Check and remove Python path
	if _, err := os.Stat(pyPath); err == nil {
		logger.Info("Removing tool directory (Python)", "path", pyPath)
		if err := os.RemoveAll(pyPath); err != nil {
			return fmt.Errorf("failed to remove directory %s: %w", pyPath, err)
		}
	}

	// Also try to remove the binary from bin/ if it exists (for Go tools)
	binName := toolName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(projectRoot, "MCP-NG/bin", binName)
	if _, err := os.Stat(binPath); err == nil {
		logger.Info("Removing tool binary", "path", binPath)
		if err := os.Remove(binPath); err != nil {
			logger.Warn("Failed to remove binary (might be locked)", "path", binPath, "error", err)
			// Not critical if binary remains, but good to clean up.
		}
	}

	logger.Info("Tool successfully deleted", "tool", toolName)
	return nil
}

func (s *server) ListTools(ctx context.Context, in *pb.ListToolsRequest) (*pb.ListToolsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var toolDescriptions []*pb.ToolDescription

	// Retrieve admin token from context
	clientToken, _ := ctx.Value("adminToken").(string)
	isAdmin := s.adminToken != "" && clientToken == s.adminToken

	for _, t := range s.tools {
		// Filter sensitive meta-tools if not admin
		if !isAdmin {
			if t.description.Name == "tool_manager" {
				continue
			}
		}
		toolDescriptions = append(toolDescriptions, t.description)
	}
	return &pb.ListToolsResponse{Tools: toolDescriptions}, nil
}

func (s *server) ExecuteTool(ctx context.Context, in *pb.ExecuteToolRequest) (*pb.ExecuteToolResponse, error) {
	s.mu.RLock()
	tool, ok := s.tools[in.ToolName]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "Tool '%s' not found", in.ToolName)
	}

	// Make the gRPC call to the actual tool microservice.
	runResp, err := tool.client.Run(ctx, &pb.ToolRunRequest{Arguments: in.Arguments})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "gRPC call to tool failed: %v", err)
	}

	// The tool returns a generic `Value`. We must wrap it in a `Struct`
	// with a "result" key. This creates a consistent structure that our
	// adapter is designed to handle.

	// Handle the case where the tool returns absolutely nothing.
	if runResp.Result == nil {
		// Return an empty struct to prevent nil pointers downstream.
		return &pb.ExecuteToolResponse{Result: &structpb.Struct{}}, nil
	}

	// Use the structpb helper to create a new struct: {"result": <value from tool>}
	resultStruct, err := structpb.NewStruct(map[string]interface{}{
		"result": runResp.Result.AsInterface(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to wrap tool result value: %v", err)
	}

	return &pb.ExecuteToolResponse{Result: resultStruct}, nil
}

// --- Utility and Middleware ---

func startBanCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			now := time.Now()
			banList.Range(func(key, value interface{}) bool {
				state := value.(*banState)
				state.mu.Lock()
				// Remove if ban expired OR if record is old (stale failures)
				if (!state.banExpiry.IsZero() && now.After(state.banExpiry)) ||
					(now.Sub(state.lastSeen) > 1*time.Hour) {
					banList.Delete(key)
				}
				state.mu.Unlock()
				return true
			})
		}
	}()
}

func findProjectRoot(startPath string) (string, error) {
	dir := startPath
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			return "", fmt.Errorf("go.work file not found")
		}
		dir = parentDir
	}
}

func findFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

type banState struct {
	failures  int
	banExpiry time.Time
	lastSeen  time.Time
	mu        sync.Mutex
}

var banList sync.Map // map[string]*banState

func oauthMiddleware(next http.Handler, expectedToken string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}

		// 1. Check Ban Status
		if v, ok := banList.Load(host); ok {
			state := v.(*banState)
			state.mu.Lock()
			if time.Now().Before(state.banExpiry) {
				state.mu.Unlock()
				// Reject immediately without parsing headers
				http.Error(w, "Too many failed attempts. Try again later.", http.StatusTooManyRequests)
				return
			}
			state.mu.Unlock()
		}

		// 2. Validate Token
		authHeader := r.Header.Get("Authorization")
		fail := false
		if authHeader == "" {
			fail = true
		} else {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			token = strings.TrimSpace(token)
			if token != expectedToken {
				fail = true
			}
		}

		if fail {
			// 3. Record Failure
			v, _ := banList.LoadOrStore(host, &banState{})
			state := v.(*banState)
			state.mu.Lock()
			state.lastSeen = time.Now()

			// Reset if previous ban expired or stale
			if !state.banExpiry.IsZero() && time.Now().After(state.banExpiry) {
				state.failures = 0
				state.banExpiry = time.Time{}
			}

			state.failures++
			if state.failures >= 10 {
				state.banExpiry = time.Now().Add(5 * time.Minute)
				logger.Warn("IP Banned due to Auth Flood", "ip", host)
			}
			state.mu.Unlock()

			if authHeader == "" {
				http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			} else {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
			}
			return
		}

		// Success - reset failures
		if v, ok := banList.Load(host); ok {
			state := v.(*banState)
			state.mu.Lock()
			state.failures = 0
			state.lastSeen = time.Now()
			state.mu.Unlock()
		}

		next.ServeHTTP(w, r)
	})
}

// requestLogger is a middleware that logs details about each incoming HTTP request.
func requestLogger(next http.Handler, s *server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.configMu.RLock()
		tracingEnabled := s.config.TracingEnabled
		s.configMu.RUnlock()

		var traceID string
		ctx := r.Context()

		if tracingEnabled {
			traceID = r.Header.Get("X-Trace-Id")
			if traceID == "" {
				traceID = generateTraceID()
			}
			w.Header().Set("X-Trace-Id", traceID)
			ctx = context.WithValue(ctx, "trace_id", traceID)
		}

		logger.Info("Received HTTP request", "method", r.Method, "url", r.URL.String(), "remote_addr", r.RemoteAddr, "trace_id", traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func readLastLogs(path string, count int, minLevel string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	fileSize := stat.Size()
	// Read last 100KB to ensure we get enough context
	var offset int64 = 0
	const maxBytes = 100 * 1024
	if fileSize > maxBytes {
		offset = fileSize - maxBytes
	}

	_, err = f.Seek(offset, io.SeekStart)
	if err != nil {
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(f)

	first := true
	levels := map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}
	targetLvl := 0
	if minLevel != "" {
		targetLvl = levels[strings.ToUpper(minLevel)]
	}

	for scanner.Scan() {
		if first && offset > 0 {
			first = false
			continue
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "" { continue }

		// Basic filtering
		if minLevel != "" {
			var j map[string]interface{}
			if err := json.Unmarshal([]byte(line), &j); err == nil {
				if lvl, ok := j["level"].(string); ok {
					if levels[strings.ToUpper(lvl)] < targetLvl {
						continue
					}
				}
			}
		}
		lines = append(lines, line)
	}

	// Return last 'count' lines
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return lines, nil
}

func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// sessionMiddleware manages the MCP session lifecycle for POST requests.
func sessionMiddleware(cache *mcache.Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Enforce 10MB limit on request body to prevent OOM
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

		// First read the body to determine the request method.
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			if err.Error() == "http: request body too large" {
				logger.Warn("Rejected large payload", "remote_addr", r.RemoteAddr)
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			logger.Error("Failed to read request body in sessionMiddleware", "error", err)
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		// Restore the body so the next handler can read it.
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var req struct {
			Method string `json:"method"`
		}

		if len(bodyBytes) > 0 {
			if err := json.Unmarshal(bodyBytes, &req); err != nil {
				logger.Error("Invalid JSON in request body", "error", err, "body", string(bodyBytes))
				http.Error(w, fmt.Sprintf("Invalid JSON in request body: %v", err), http.StatusBadRequest)
				return
			}
		} else {
			// Empty body for POST is an error.
			logger.Warn("Received empty request body in POST")
			http.Error(w, "Empty request body", http.StatusBadRequest)
			return
		}

		sessionID := r.Header.Get("Mcp-Session-Id")
		var session *mcache.Session
		var ok bool

		// If this is the initialize method, it has special session creation logic.
		if req.Method == "initialize" {
			// If the client already came with an ID (e.g. after GET), use that session.
			if sessionID != "" {
				session, ok = cache.GetSession(sessionID)
			}

			// If no session (ok == false), create a new one.
			// This covers the case where initialize is the very first request.
			if !ok {
				session = cache.NewSession()
				w.Header().Set("Mcp-Session-Id", session.ID)
				logger.Info("Created new session via initialize POST", "session_id", session.ID)
			} else {
				logger.Info("Reusing existing session for initialize POST", "session_id", session.ID)
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// For all other methods, we find the session or create a new one to be compatible with simpler clients.
		session, ok = cache.GetSession(sessionID)
		if !ok {
			// This branch handles both missing and invalid/expired session IDs.
			if sessionID != "" {
				logger.Warn("Invalid or expired session ID provided for POST, creating new session.", "provided_id", sessionID)
			}
			session = cache.NewSession()
			w.Header().Set("Mcp-Session-Id", session.ID)
			logger.Info("Created new session for POST request", "session_id", session.ID, "method", req.Method)
		} else {
			logger.Info("Using existing session for POST request", "session_id", session.ID, "method", req.Method)
		}
		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
