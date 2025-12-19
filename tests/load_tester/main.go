package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config holds the load test configuration
type Config struct {
	BaseURL     string
	Concurrency int
	TotalReqs   int
	Scenario    string
	AuthHeader  string
	PayloadSize int // in MB
}

// Result holds the timing of a single request
type Result struct {
	Duration time.Duration
	Error    error
	Status   int
	Tool     string
}

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "Base URL of the MCP server")
	concurrency := flag.Int("c", 10, "Number of concurrent clients")
	totalReqs := flag.Int("n", 100, "Total number of requests to send")
	scenario := flag.String("scenario", "calc", "Test scenario: calc, mixed, io_bound, saturation, large_payload")
	authHeader := flag.String("auth-header", "", "Custom Authorization header value")
	payloadSize := flag.Int("payload-size", 0, "Size of payload in MB for large_payload scenario")
	flag.Parse()

	config := Config{
		BaseURL:     *baseURL,
		Concurrency: *concurrency,
		TotalReqs:   *totalReqs,
		Scenario:    *scenario,
		AuthHeader:  *authHeader,
		PayloadSize: *payloadSize,
	}

	fmt.Printf("Starting Load Test:\n")
	fmt.Printf("  URL: %s\n", config.BaseURL)
	fmt.Printf("  Concurrency: %d\n", config.Concurrency)
	fmt.Printf("  Total Requests: %d\n", config.TotalReqs)
	fmt.Printf("  Scenario: %s\n", config.Scenario)
	if config.PayloadSize > 0 {
		fmt.Printf("  Payload Size: %d MB\n", config.PayloadSize)
	}
	fmt.Println("------------------------------------------------")

	start := time.Now()
	results := runLoadTest(config)
	totalDuration := time.Since(start)

	printReport(results, totalDuration, config)
}

func runLoadTest(cfg Config) []Result {
	var wg sync.WaitGroup
	results := make(chan Result, cfg.TotalReqs)

	t := &http.Transport{
		MaxIdleConns:        cfg.Concurrency + 20,
		MaxIdleConnsPerHost: cfg.Concurrency + 20,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{
		Transport: t,
		Timeout:   30 * time.Second,
	}

	jobs := make(chan int, cfg.TotalReqs)
	for i := 0; i < cfg.TotalReqs; i++ {
		jobs <- i
	}
	close(jobs)

	// Pre-generate large payload if needed to avoid CPU bottleneck during test
	var largePayload string
	if cfg.PayloadSize > 0 {
		largePayload = strings.Repeat("x", cfg.PayloadSize*1024*1024)
	}

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			// 1. Initialize Session
			// Pass auth header here if needed? Usually Initialize is open, but if Auth is on...
			// The server checks Auth on all requests if enabled.
			sessionID, err := initializeSession(client, cfg.BaseURL, cfg.AuthHeader)
			if err != nil {
				// If we are testing Auth Flood with invalid tokens, failure is EXPECTED.
				// But initializeSession returns error on non-200.
				// We should handle 401/403 gracefully if that's the test.
				// For simplicity, we log error.
				// But wait, if we want to test "Auth Flood", we just want to hit the endpoint.
				// If session init fails with 401, that counts as a "Success" for the test (Server rejected us).
				// We'll proceed to jobs anyway? No, we need session ID for tools/call.
				// Actually, Auth Flood targets `initialize` or `tools/call`.
				// If `initialize` fails, we can't do `tools/call`.
				// So for Auth Flood, we might just loop calling `initialize`?
				// Or we accept the error and record it.

				log.Printf("Worker %d init session result: %v", workerID, err)
				for range jobs {
					// Record the init failure as the result for each job?
					// Or just return one failure?
					// Let's record it.
					results <- Result{Error: err, Status: 0}
				}
				return
			}

			// 2. Process Jobs
			for range jobs {
				toolName, toolArgs := getNextJob(cfg.Scenario, rng)
				if largePayload != "" {
					toolArgs["dummy_data"] = largePayload
				}
				res := executeTool(client, cfg.BaseURL, sessionID, toolName, toolArgs, cfg.AuthHeader)
				res.Tool = toolName
				results <- res
			}
		}(i)
	}

	wg.Wait()
	close(results)

	var resSlice []Result
	for r := range results {
		resSlice = append(resSlice, r)
	}
	return resSlice
}

func getNextJob(scenario string, rng *rand.Rand) (string, map[string]interface{}) {
	if scenario == "calc" || scenario == "large_payload" {
		return "calculator", map[string]interface{}{"expression": "10 + 20"}
	}

	if scenario == "io_bound" {
		if rng.Float64() < 0.5 {
			return "file_writer", map[string]interface{}{
				"path":    "/tmp/perf_test.txt",
				"content": fmt.Sprintf("Load test content %d", rng.Int()),
			}
		}
		return "file_reader", map[string]interface{}{"path": "/tmp/perf_test.txt"}
	}

	if scenario == "saturation" {
		// Use calculator with a more complex expression to simulate some load
		return "calculator", map[string]interface{}{"expression": "10 + 20 * 30 / 5 + 100"}
	}

	// Mixed
	r := rng.Float64()
	if r < 0.60 {
		return "calculator", map[string]interface{}{"expression": "10 + 20"}
	} else if r < 0.80 {
		return "file_writer", map[string]interface{}{
			"path":    "/tmp/perf_test.txt",
			"content": fmt.Sprintf("Load test content %d", rng.Int()),
		}
	} else {
		return "file_reader", map[string]interface{}{"path": "/tmp/perf_test.txt"}
	}
}

func initializeSession(client *http.Client, baseURL, authHeader string) (string, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      0,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", baseURL, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status: %d, body: %s", resp.StatusCode, string(b))
	}

	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return "", fmt.Errorf("server did not return Mcp-Session-Id header")
	}
	return sessionID, nil
}

func executeTool(client *http.Client, baseURL, sessionID, toolName string, args map[string]interface{}, authHeader string) Result {
	reqPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
		"id": 1,
	}

	bodyBytes, _ := json.Marshal(reqPayload)
	start := time.Now()

	req, _ := http.NewRequest("POST", baseURL, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	req.Header.Set("X-Mcp-Sync", "true")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return Result{Duration: duration, Error: err}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return Result{
		Duration: duration,
		Status:   resp.StatusCode,
		Error:    nil,
	}
}

func printReport(results []Result, totalDuration time.Duration, cfg Config) {
	var totalTime time.Duration
	var minTime = time.Hour
	var maxTime time.Duration
	var errors int
	var success int
	statusCodes := make(map[int]int)

	type toolStats struct {
		count int
		totalTime time.Duration
	}
	stats := make(map[string]*toolStats)

	for _, r := range results {
		statusCodes[r.Status]++
		if r.Error != nil || (r.Status != 200 && r.Status != 202) {
			// Count 401/403/500 as "errors" for general reporting,
			// but we will detail them in status codes.
			errors++
			// Continue to stats to see latency of errors too?
		} else {
			success++
		}

		// Always track time
		totalTime += r.Duration
		if r.Duration < minTime {
			minTime = r.Duration
		}
		if r.Duration > maxTime {
			maxTime = r.Duration
		}

		toolName := r.Tool
		if toolName == "" { toolName = "unknown" }

		if _, ok := stats[toolName]; !ok {
			stats[toolName] = &toolStats{}
		}
		stats[toolName].count++
		stats[toolName].totalTime += r.Duration
	}

	avg := totalTime / time.Duration(len(results))
	rps := float64(len(results)) / totalDuration.Seconds()

	fmt.Printf("\n--- Results ---\n")
	fmt.Printf("Total Requests:      %d\n", len(results))
	fmt.Printf("Successful (200):    %d\n", success)
	fmt.Printf("Failed / Other:      %d\n", errors)
	fmt.Printf("Status Codes:        %v\n", statusCodes)
	fmt.Printf("Total Duration:      %v\n", totalDuration)
	fmt.Printf("Throughput (RPS):    %.2f req/sec\n", rps)
	fmt.Printf("Latency (Min):       %v\n", minTime)
	fmt.Printf("Latency (Avg):       %v\n", avg)
	fmt.Printf("Latency (Max):       %v\n", maxTime)

	fmt.Printf("\n--- Breakdown by Tool ---\n")
	for name, s := range stats {
		toolAvg := s.totalTime / time.Duration(s.count)
		fmt.Printf("%-20s: %d reqs, Avg Latency: %v\n", name, s.count, toolAvg)
	}
}
