package testutil

import (
	"fmt"
	"os"

	"github.com/vladpann/go-cloud-file-storage/config"
)

// NewTestConfig creates a configuration for testing purposes.
// It accepts connection strings for PostgreSQL and Redis and configures
// test-specific values that mirror production settings where appropriate.
//
// Configuration values:
//   - Session TTL: 24 hours (matches production)
//   - Password hashing: bcrypt with cost 10 (bcrypt.DefaultCost, matches production)
//   - Tracing: disabled (reduces test overhead)
//   - Logging: debug level (detailed output for test debugging)
//   - Connection pool: 5 connections (smaller than production)
//   - Cookie secure: true (can be overridden per test if needed)
func NewTestConfig(postgresURL, redisAddr, redisPassword string) *config.Config {
	env := map[string]string{
		"APP_NAME":              "test-app",
		"APP_VERSION":           "test",
		"HTTP_PORT":             "0",
		"LOG_LEVEL":             "debug",
		"LOG_FOLDER":            os.TempDir(),
		"PG_POOL_MAX":           "5",
		"PG_URL":                postgresURL,
		"REDIS_ADDR":            redisAddr,
		"REDIS_PASSWORD":        redisPassword,
		"SESSION_TTL":           "24h",
		"SESSION_COOKIE_SECURE": "true",
		"TRACING_ENABLED":       "false",
	}

	for key, value := range env {
		if err := os.Setenv(key, value); err != nil {
			// Fail fast: this is test setup.
			panic(fmt.Sprintf("failed to set env %s: %v", key, err))
		}
	}

	cfg, err := config.NewConfig()
	if err != nil {
		panic("failed to create test config: " + err.Error())
	}

	return cfg
}
