package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"eka-dev.cloud/asynq-worker/config"
	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"
)

type HTTPPostPayload struct {
	URL  string `json:"url"`
	Body string `json:"body"`
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
	mux.HandleFunc("task:http_post", handleHTTPPostTask)

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

	dashboardPort := config.Config.Port
	if dashboardPort == "" {
		dashboardPort = "8085"
	}

	slog.Info("Starting Asynq Monitoring Dashboard", "port", dashboardPort)
	
	// Start HTTP server for monitoring
	httpServer := &http.Server{
		Addr:    ":" + dashboardPort,
		Handler: dashboardHandler,
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

func handleHTTPPostTask(ctx context.Context, t *asynq.Task) error {
	var payload HTTPPostPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		slog.Error("Failed to parse HTTP Post Task payload", "error", err)
		return err
	}

	slog.Info("Dispatching HTTP Task callback", "target", payload.URL, "body", payload.Body)

	req, err := http.NewRequestWithContext(ctx, "POST", payload.URL, bytes.NewBufferString(payload.Body))
	if err != nil {
		slog.Error("Failed to build HTTP request callback", "error", err)
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	
	internalToken := config.Config.InternalToken
	if internalToken == "" {
		internalToken = "your_internal_token"
	}
	req.Header.Set("X-Internal-Token", internalToken)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("HTTP dispatch call failed", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Error("Dispatch request returned error status", "status", resp.StatusCode)
		return fmt.Errorf("HTTP error status: %d", resp.StatusCode)
	}

	slog.Info("HTTP Task successfully completed", "status", resp.StatusCode)
	return nil
}
