package testutil

import (
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
	// Note: We cannot directly construct config.Config because the inner structs
	// (app, http, log, pg, redis, session, tracing) are unexported.
	// We need to use environment variables and parse the config.
	// However, for testing we want to avoid environment variable dependencies.
	// We'll need to construct this differently or use a custom test config type.

	// Set required environment variables
	os.Setenv("APP_NAME", "test-app")
	os.Setenv("APP_VERSION", "test")
	os.Setenv("HTTP_PORT", "0")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("LOG_FOLDER", os.TempDir())
	os.Setenv("PG_POOL_MAX", "5")
	os.Setenv("PG_URL", postgresURL)
	os.Setenv("REDIS_ADDR", redisAddr)
	os.Setenv("REDIS_PASSWORD", redisPassword)
	os.Setenv("SESSION_TTL", "24h")
	os.Setenv("SESSION_COOKIE_SECURE", "true")
	os.Setenv("TRACING_ENABLED", "false")

	// Parse config from environment
	cfg, err := config.NewConfig()
	if err != nil {
		// This shouldn't fail with valid values, but if it does, panic since
		// we're in test setup and should fail fast
		panic("failed to create test config: " + err.Error())
	}

	return cfg
}
