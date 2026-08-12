package tests

import (
	"os"
	"testing"

	"github.com/financial-aggregator/ledger/internal/config"
)

func getBaseConnStr() string {
	if envUrl := os.Getenv("DATABASE_URL"); envUrl != "" {
		return envUrl
	}
	// Fallback to run tests without Docker
	return "postgres://admin:admin_password@localhost:5432/aggregator?sslmode=disable"
}

// SetupTestEnv sets up environment variables for testing
// and returns a cleanup function to restore original values
func SetupTestEnv(t *testing.T, env map[string]string) func() {
	t.Helper()

	// Store original values
	original := make(map[string]string)
	for key := range env {
		original[key] = os.Getenv(key)
	}

	// Set test environment
	for key, value := range env {
		os.Setenv(key, value)
	}

	// Return cleanup function
	return func() {
		for key, value := range original {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}
}

// NewTestConfig creates a config with sensible defaults for testing
func NewTestConfig(t *testing.T) *config.Config {
	t.Helper()

	return &config.Config{
		DatabaseURL:          getBaseConnStr(),
		JwtSigningKey:        "test-secret-key-256-bit-minimum-length-required-here-1234567890ab",
		JwtIssuer:            "test-issuer",
		JwtExpirationMinutes: 60,
		Port:                 "8080",
	}
}
