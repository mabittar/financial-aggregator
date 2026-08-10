package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config holds environment-driven application settings.
type Config struct {
	DatabaseURL          string
	JwtSigningKey        string
	JwtIssuer            string
	JwtExpirationMinutes int
	Port                 string
}

// Load reads configuration from the environment, applying defaults where appropriate.
func Load() (*Config, error) {
	databaseURL := os.Getenv("LEDGER_DB_URL")
	if databaseURL == "" {
		postgresDB := os.Getenv("POSTGRES_DB")
		postgresUser := os.Getenv("POSTGRES_USER")
		postgresPassword := os.Getenv("POSTGRES_PASSWORD")
		if postgresDB == "" || postgresUser == "" || postgresPassword == "" {
			return nil, errors.New("LEDGER_DB_URL or Postgres env vars must be set")
		}
		databaseURL = fmt.Sprintf("postgresql://%s:%s@db/%s?sslmode=disable", postgresUser, postgresPassword, postgresDB)
	}

	jwtSigningKey := os.Getenv("JWT_SIGNING_KEY")
	if jwtSigningKey == "" {
		return nil, errors.New("JWT_SIGNING_KEY is required")
	}

	jwtIssuer := os.Getenv("JWT_ISSUER")
	if jwtIssuer == "" {
		jwtIssuer = "financial-aggregator"
	}

	expirationMinutes := 60
	if raw := os.Getenv("JWT_EXPIRATION_MINUTES"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid JWT_EXPIRATION_MINUTES: %w", err)
		}
		expirationMinutes = parsed
	}

	port := os.Getenv("LEDGER_PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		DatabaseURL:          databaseURL,
		JwtSigningKey:        jwtSigningKey,
		JwtIssuer:            jwtIssuer,
		JwtExpirationMinutes: expirationMinutes,
		Port:                 port,
	}, nil
}

// Validate performs a final check to ensure all required environment variables are present.
// Panics with a clear error message if any required variables are missing.
// This prevents the application from starting with incomplete configuration.
func (c *Config) Validate() error {
	var missingVars []string

	// Check required environment variables
	if os.Getenv("JWT_SIGNING_KEY") == "" {
		missingVars = append(missingVars, "JWT_SIGNING_KEY")
	}

	if os.Getenv("LEDGER_DB_URL") == "" {
		// If LEDGER_DB_URL is not set, check for Postgres components
		if os.Getenv("POSTGRES_DB") == "" {
			missingVars = append(missingVars, "POSTGRES_DB")
		}
		if os.Getenv("POSTGRES_USER") == "" {
			missingVars = append(missingVars, "POSTGRES_USER")
		}
		if os.Getenv("POSTGRES_PASSWORD") == "" {
			missingVars = append(missingVars, "POSTGRES_PASSWORD")
		}
	}

	if len(missingVars) > 0 {
		errorMsg := fmt.Sprintf("FATAL: Missing required environment variables: %v\n", missingVars)
		errorMsg += "Please set the following variables in infra/.env or as system environment variables:\n"
		for _, v := range missingVars {
			errorMsg += fmt.Sprintf("  - %s\n", v)
		}
		errorMsg += "\nRefer to infra/.env.example for documentation."
		panic(errorMsg)
	}

	return nil
}
