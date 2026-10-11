package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"eka-dev.cloud/asynq-worker/config"
	"github.com/hibiken/asynq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestAsynqWorker_E2E_Testcontainers(t *testing.T) {
	// Skip gracefully if Docker provider is not healthy / running
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Start Redis container
	redisContainer, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}
	defer func() {
		_ = redisContainer.Terminate(context.Background())
	}()

	connStr, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get redis connection string: %v", err)
	}
	redisAddr := strings.TrimPrefix(connStr, "redis://")

	// 2. Setup mock target HTTP server
	var (
		mu             sync.Mutex
		receivedMethod string
		receivedAuth   string
		receivedToken  string
		receivedBody   string
		requestDone    = make(chan struct{})
	)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		receivedMethod = r.Method
		receivedAuth = r.Header.Get("Authorization")
		receivedToken = r.Header.Get("X-Internal-Token")
		bodyBytes, _ := io.ReadAll(r.Body)
		receivedBody = string(bodyBytes)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))

		select {
		case <-requestDone:
		default:
			close(requestDone)
		}
	}))
	defer targetServer.Close()

	// 3. Configure worker and start Asynq Server
	config.Config.InternalToken = "integration_internal_token"

	redisOpt := asynq.RedisClientOpt{
		Addr: redisAddr,
	}

	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 2,
			Queues: map[string]int{
				"default": 1,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc("task:http", handleHTTPTask)
	mux.HandleFunc("task:http_post", handleHTTPPostTask)

	go func() {
		if err := srv.Run(mux); err != nil && err != asynq.ErrServerClosed {
			t.Logf("asynq server stopped: %v", err)
		}
	}()
	defer srv.Shutdown()

	// 4. Enqueue task using Asynq Client
	client := asynq.NewClient(redisOpt)
	defer client.Close()

	payload := HTTPTaskPayload{
		URL:    targetServer.URL,
		Method: "POST",
		Headers: map[string]string{
			"Authorization": "Bearer container_test_token",
		},
		Body: `{"event":"order_created","id":999}`,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal task payload: %v", err)
	}

	task := asynq.NewTask("task:http", payloadBytes)
	_, err = client.Enqueue(task)
	if err != nil {
		t.Fatalf("failed to enqueue task: %v", err)
	}

	// 5. Wait for target server to receive the dispatch
	select {
	case <-requestDone:
		// Success, inspect received data
		mu.Lock()
		defer mu.Unlock()

		if receivedMethod != "POST" {
			t.Errorf("expected POST method, got %s", receivedMethod)
		}
		if receivedAuth != "Bearer container_test_token" {
			t.Errorf("expected Bearer container_test_token, got %s", receivedAuth)
		}
		if receivedToken != "integration_internal_token" {
			t.Errorf("expected integration_internal_token, got %s", receivedToken)
		}
		if receivedBody != `{"event":"order_created","id":999}` {
			t.Errorf("unexpected body received: %s", receivedBody)
		}

	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for task to be processed and dispatched by worker")
	}
}
