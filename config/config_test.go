package config

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear any existing env variables for clean test
	_ = os.Unsetenv("REDIS_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("INTERNAL_TOKEN")
	_ = os.Unsetenv("REDIS_USERNAME")
	_ = os.Unsetenv("REDIS_PASSWORD")

	LoadConfig()

	if Config.RedisUrl == "" {
		t.Error("expected RedisUrl to have a default value")
	}
	if Config.Port != "8085" {
		t.Errorf("expected default port 8085, got %s", Config.Port)
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	_ = os.Setenv("REDIS_URL", "redis.custom.local:6380")
	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("INTERNAL_TOKEN", "custom_secret_123")
	defer func() {
		_ = os.Unsetenv("REDIS_URL")
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("INTERNAL_TOKEN")
	}()

	LoadConfig()

	if Config.RedisUrl != "redis.custom.local:6380" {
		t.Errorf("expected redis.custom.local:6380, got %s", Config.RedisUrl)
	}
	if Config.Port != "9090" {
		t.Errorf("expected 9090, got %s", Config.Port)
	}
	if Config.InternalToken != "custom_secret_123" {
		t.Errorf("expected custom_secret_123, got %s", Config.InternalToken)
	}
}
