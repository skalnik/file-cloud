package main

import (
	"os"
	"strings"
	"testing"
)

func TestLookupEnvDefault(t *testing.T) {
	// Test with env var not set - should return default
	result := LookupEnvDefault("NONEXISTENT_VAR_12345", "default_value")
	if result != "default_value" {
		t.Errorf(`Expected "default_value", but got %s`, result)
	}
}

func TestLookupEnvDefaultWithEnvSet(t *testing.T) {
	// Set an env var and test
	os.Setenv("TEST_VAR_FOR_FILE_CLOUD", "env_value")
	defer os.Unsetenv("TEST_VAR_FOR_FILE_CLOUD")

	result := LookupEnvDefault("TEST_VAR_FOR_FILE_CLOUD", "default_value")
	if result != "env_value" {
		t.Errorf(`Expected "env_value", but got %s`, result)
	}
}

func TestLookupEnvDefaultEmptyString(t *testing.T) {
	// Set an env var to empty string - should still return the empty string, not default
	os.Setenv("TEST_VAR_EMPTY", "")
	defer os.Unsetenv("TEST_VAR_EMPTY")

	result := LookupEnvDefault("TEST_VAR_EMPTY", "default_value")
	if result != "" {
		t.Errorf(`Expected empty string, but got %s`, result)
	}
}

// ValidateConfig tests

func TestValidateConfigValid(t *testing.T) {
	err := ValidateConfig("my-bucket", "secret", "key", "", "8080", "", "")
	if err != nil {
		t.Errorf("Expected no error for valid config, got %v", err)
	}
}

func TestValidateConfigValidWithCDN(t *testing.T) {
	err := ValidateConfig("my-bucket", "secret", "key", "https://cdn.example.com", "8080", "", "")
	if err != nil {
		t.Errorf("Expected no error for valid config with CDN, got %v", err)
	}
}

func TestValidateConfigValidWithAuth(t *testing.T) {
	err := ValidateConfig("my-bucket", "secret", "key", "", "8080", "user", "pass")
	if err != nil {
		t.Errorf("Expected no error for valid config with auth, got %v", err)
	}
}

func TestValidateConfigEmptyBucket(t *testing.T) {
	err := ValidateConfig("", "secret", "key", "", "8080", "", "")
	if err == nil {
		t.Error("Expected error for empty bucket")
	}
	if !strings.Contains(err.Error(), "bucket") {
		t.Errorf("Expected error to mention bucket, got: %v", err)
	}
}

func TestValidateConfigEmptySecret(t *testing.T) {
	err := ValidateConfig("bucket", "", "key", "", "8080", "", "")
	if err == nil {
		t.Error("Expected error for empty secret")
	}
	if !strings.Contains(err.Error(), "secret") {
		t.Errorf("Expected error to mention secret, got: %v", err)
	}
}

func TestValidateConfigEmptyKey(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "", "", "8080", "", "")
	if err == nil {
		t.Error("Expected error for empty key")
	}
	if !strings.Contains(err.Error(), "key") {
		t.Errorf("Expected error to mention key, got: %v", err)
	}
}

func TestValidateConfigInvalidPort(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "key", "", "not-a-number", "", "")
	if err == nil {
		t.Error("Expected error for invalid port")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("Expected error to mention port, got: %v", err)
	}
}

func TestValidateConfigPortTooLow(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "key", "", "0", "", "")
	if err == nil {
		t.Error("Expected error for port 0")
	}
}

func TestValidateConfigPortTooHigh(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "key", "", "65536", "", "")
	if err == nil {
		t.Error("Expected error for port > 65535")
	}
}

func TestValidateConfigInvalidCDNURL(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "key", "not-a-url", "8080", "", "")
	if err == nil {
		t.Error("Expected error for invalid CDN URL")
	}
}

func TestValidateConfigCDNWrongScheme(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "key", "ftp://cdn.example.com", "8080", "", "")
	if err == nil {
		t.Error("Expected error for non-http(s) CDN URL")
	}
	if !strings.Contains(err.Error(), "http") {
		t.Errorf("Expected error to mention http scheme, got: %v", err)
	}
}

func TestValidateConfigUserWithoutPass(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "key", "", "8080", "user", "")
	if err == nil {
		t.Error("Expected error when username provided without password")
	}
	if !strings.Contains(err.Error(), "username") || !strings.Contains(err.Error(), "password") {
		t.Errorf("Expected error to mention username and password, got: %v", err)
	}
}

func TestValidateConfigPassWithoutUser(t *testing.T) {
	err := ValidateConfig("bucket", "secret", "key", "", "8080", "", "pass")
	if err == nil {
		t.Error("Expected error when password provided without username")
	}
}
