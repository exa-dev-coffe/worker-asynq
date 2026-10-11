package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"eka-dev.cloud/asynq-worker/config"
	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"
)

// HTTPTaskPayload represents generic HTTP dispatch task inspired by Cloud Tasks
type HTTPTaskPayload struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`  // HTTP Method: POST, GET, PUT, PATCH, DELETE (default: POST)
	Headers map[string]string `json:"headers,omitempty"` // Custom HTTP headers
	Body    string            `json:"body,omitempty"`    // Request body
	Timeout int               `json:"timeout,omitempty"` // Timeout in seconds (default: 15)
}

// HTTPPostPayload alias for backward compatibility
type HTTPPostPayload = HTTPTaskPayload

var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	},
}

func initLogger() {
	// Initialize default slog handler to JSON
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
}

func main() {
	initLogger()

	// Load configuration
	config.LoadConfig()

	redisOpt := asynq.RedisClientOpt{
		Addr:     config.Config.RedisUrl,
		Username: config.Config.RedisUsername,
		Password: config.Config.RedisPassword,
	}

	// Setup Asynq Worker Server
	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 5,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc("task:http_post", handleHTTPTask)
	mux.HandleFunc("task:http", handleHTTPTask)

	slog.Info("Starting Asynq Server/Worker", "redis", config.Config.RedisUrl)

	// Run worker in a separate goroutine
	go func() {
		if err := srv.Run(mux); err != nil {
			slog.Error("Fatal error running Asynq server", "error", err)
			os.Exit(1)
		}
	}()

	// Setup Monitoring Dashboard HTTP Server using asynqmon
	// Mount WebUI at root URL path "/"
	dashboardHandler := asynqmon.New(asynqmon.Options{
		RedisConnOpt: redisOpt,
		RootPath:     "/",
	})

	rootMux := http.NewServeMux()

	// Health check endpoints for Kubernetes liveness & readiness probes
	rootMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"OK","service":"asynq-worker"}`))
	})
	rootMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"OK","service":"asynq-worker"}`))
	})

	// Mount asynqmon dashboard under root path
	rootMux.Handle("/", dashboardHandler)

	dashboardPort := config.Config.Port
	if dashboardPort == "" {
		dashboardPort = "8085"
	}

	slog.Info("Starting Asynq Monitoring Dashboard & Health Server", "port", dashboardPort)
	
	// Start HTTP server for monitoring and health check
	httpServer := &http.Server{
		Addr:    ":" + dashboardPort,
		Handler: rootMux,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start monitoring server", "error", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down Asynq Worker server...")
	
	// Shutdown worker
	srv.Shutdown()

	// Shutdown dashboard server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Error shutting down monitoring dashboard", "error", err)
	}

	slog.Info("Asynq Worker server gracefully stopped.")
}

func handleHTTPTask(ctx context.Context, t *asynq.Task) error {
	var payload HTTPTaskPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		slog.Error("Failed to parse HTTP Task payload", "error", err)
		return err
	}

	method := strings.ToUpper(strings.TrimSpace(payload.Method))
	if method == "" {
		method = http.MethodPost
	}

	timeout := 15 * time.Second
	if payload.Timeout > 0 {
		timeout = time.Duration(payload.Timeout) * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	slog.Info("Dispatching HTTP Task", "method", method, "target", payload.URL)

	req, err := http.NewRequestWithContext(reqCtx, method, payload.URL, bytes.NewBufferString(payload.Body))
	if err != nil {
		slog.Error("Failed to build HTTP request callback", "error", err)
		return err
	}

	// Default headers
	req.Header.Set("Content-Type", "application/json")

	internalToken := config.Config.InternalToken
	if internalToken == "" {
		internalToken = "your_internal_token"
	}
	req.Header.Set("X-Internal-Token", internalToken)

	// Apply custom headers from payload (can override default Content-Type or X-Internal-Token)
	for k, v := range payload.Headers {
		req.Header.Set(k, v)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		slog.Error("HTTP dispatch call failed", "method", method, "target", payload.URL, "error", err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode >= 400 {
		slog.Error("Dispatch request returned error status", "status", resp.StatusCode, "target", payload.URL, "response", string(respBody))
		return fmt.Errorf("HTTP error status: %d, response: %s", resp.StatusCode, string(respBody))
	}

	// Record task result in Asynq if writer is available
	if rw := t.ResultWriter(); rw != nil {
		_, _ = rw.Write([]byte(fmt.Sprintf(`{"status":%d}`, resp.StatusCode)))
	}

	slog.Info("HTTP Task successfully completed", "method", method, "target", payload.URL, "status", resp.StatusCode)
	return nil
}

// handleHTTPPostTask alias for backward compatibility
func handleHTTPPostTask(ctx context.Context, t *asynq.Task) error {
	return handleHTTPTask(ctx, t)
}
