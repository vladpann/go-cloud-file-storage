package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type (
	Config struct {
		App     app
		HTTP    http
		Log     log
		PG      pg
		Redis   redis
		Tracing tracing
		Session session
	}

	app struct {
		Name    string `env:"APP_NAME,required"`
		Version string `env:"APP_VERSION,required"`
	}

	http struct {
		Port string `env:"HTTP_PORT,required"`
	}

	log struct {
		Level  string `env:"LOG_LEVEL,required"`
		Folder string `env:"LOG_FOLDER" envDefault:"out/logs"`
	}

	pg struct {
		PoolMax int    `env:"PG_POOL_MAX,required"`
		URL     string `env:"PG_URL,required"`
	}

	redis struct {
		Addr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
		Password string `env:"REDIS_PASSWORD,required"`
	}

	tracing struct {
		Enabled      bool    `env:"TRACING_ENABLED" envDefault:"false"`
		OTLPEndpoint string  `env:"TRACING_OTLP_ENDPOINT" evnDefault:"localhost:4317"`
		OTLPInsecure bool    `env:"TRACING_OTLP_INSECURE" envDefault:"true"`
		SampleRate   float64 `env:"TRACING_SAMPLE_RATE" envDefault:"0.1"`
	}

	session struct {
		TTL             time.Duration `env:"SESSION_TTL" envDefault:"24h"`
		CleanupInterval time.Duration `env:"SESSION_CLEANUP_INTERVAL" envDefault:"12h"`
	}
)

// NewConfig returns app config.
func NewConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}

	return cfg, nil
}
