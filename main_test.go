package main

import (
	"os"
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
