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
)

func TestHandleHTTPTask_DefaultPostAndHeaders(t *testing.T) {
	config.Config.InternalToken = "secret_env_token"

	var (
		receivedMethod      string
		receivedContentType string
		receivedToken       string
		receivedBody        string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		receivedToken = r.Header.Get("X-Internal-Token")

		bodyBytes, _ := io.ReadAll(r.Body)
		receivedBody = string(bodyBytes)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	payload := HTTPTaskPayload{
		URL:  server.URL,
		Body: `{"name":"test_default"}`,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	task := asynq.NewTask("task:http", payloadBytes)
	err = handleHTTPTask(context.Background(), task)
	if err != nil {
		t.Fatalf("expected handleHTTPTask to succeed, got error: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("expected POST method, got %s", receivedMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected application/json content-type, got %s", receivedContentType)
	}
	if receivedToken != "secret_env_token" {
		t.Errorf("expected secret_env_token, got %s", receivedToken)
	}
	if receivedBody != `{"name":"test_default"}` {
		t.Errorf("unexpected body: %s", receivedBody)
	}
}

func TestHandleHTTPTask_FallbackTokenWhenEmpty(t *testing.T) {
	config.Config.InternalToken = "" // Empty token in config

	var receivedToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("X-Internal-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payloadBytes, _ := json.Marshal(HTTPTaskPayload{URL: server.URL})
	task := asynq.NewTask("task:http", payloadBytes)

	if err := handleHTTPTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedToken != "your_internal_token" {
		t.Errorf("expected fallback token 'your_internal_token', got '%s'", receivedToken)
	}
}

func TestHandleHTTPTask_CustomMethodAndHeaders(t *testing.T) {
	config.Config.InternalToken = "default_token"

	var (
		receivedMethod string
		receivedAuth   string
		receivedCustom string
		receivedToken  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedAuth = r.Header.Get("Authorization")
		receivedCustom = r.Header.Get("X-Custom-Header")
		receivedToken = r.Header.Get("X-Internal-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payload := HTTPTaskPayload{
		URL:    server.URL,
		Method: "put", // lowercase should be normalized to uppercase PUT
		Headers: map[string]string{
			"Authorization":   "Bearer custom_jwt_token",
			"X-Custom-Header": "custom_value",
			"X-Internal-Token": "override_token", // override default internal token
		},
		Body: `{"update": true}`,
	}
	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask("task:http", payloadBytes)

	if err := handleHTTPTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedMethod != "PUT" {
		t.Errorf("expected PUT method, got %s", receivedMethod)
	}
	if receivedAuth != "Bearer custom_jwt_token" {
		t.Errorf("expected Bearer custom_jwt_token, got %s", receivedAuth)
	}
	if receivedCustom != "custom_value" {
		t.Errorf("expected custom_value, got %s", receivedCustom)
	}
	if receivedToken != "override_token" {
		t.Errorf("expected override_token, got %s", receivedToken)
	}
}

func TestHandleHTTPTask_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request payload"}`))
	}))
	defer server.Close()

	payloadBytes, _ := json.Marshal(HTTPTaskPayload{URL: server.URL})
	task := asynq.NewTask("task:http", payloadBytes)

	err := handleHTTPTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for HTTP 400 status, got nil")
	}

	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error message to contain 400, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bad request payload") {
		t.Errorf("expected error message to contain response preview, got: %v", err)
	}
}

func TestHandleHTTPTask_InvalidJSONPayload(t *testing.T) {
	task := asynq.NewTask("task:http", []byte(`{invalid-json`))
	err := handleHTTPTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error on invalid JSON payload, got nil")
	}
}

func TestHandleHTTPTask_Timeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wg.Done()
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payload := HTTPTaskPayload{
		URL:     server.URL,
		Timeout: 1, // 1 second timeout
	}
	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask("task:http", payloadBytes)

	start := time.Now()
	err := handleHTTPTask(context.Background(), task)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if elapsed > 1800*time.Millisecond {
		t.Errorf("expected handler to timeout around 1s, took %v", elapsed)
	}
}

func TestHandleHTTPPostTask_BackwardCompatibility(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payloadBytes, _ := json.Marshal(HTTPPostPayload{URL: server.URL, Body: "test"})
	task := asynq.NewTask("task:http_post", payloadBytes)

	err := handleHTTPPostTask(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected server to be called")
	}
}
