package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application settings loaded from environment variables.
type Config struct {
	SecretKey  string
	AuthExpiry time.Duration
	DBConfig   DBConfig
	AppCfg     AppConfig
	OTLPConfig OTLPConfig
}

// OTLPConfig holds OpenTelemetry tracing configuration.
type OTLPConfig struct {
	Endpoint string
	Protocol string
	Insecure bool
}

type AppConfig struct {
	Port string
}

// DBConfig holds the Postgres connection settings.
type DBConfig struct {
	URL         string
	MaxOpenConn int
	ConnMaxIdle time.Duration
}

// LoadConfig reads environment variables and returns a Config or an error.
func LoadConfig() (*Config, error) {
	var err error
	cfg := &Config{}

	// Helper closures
	getInt := func(key string, def int) (int, error) {
		if v := os.Getenv(key); v != "" {
			i, e := strconv.Atoi(v)
			if e != nil {
				return 0, fmt.Errorf("invalid %s: %w", key, e)
			}
			return i, nil
		}
		return def, nil
	}

	getDuration := func(key string, def time.Duration) (time.Duration, error) {
		if v := os.Getenv(key); v != "" {
			// Try to parse as duration first
			d, e := time.ParseDuration(v)
			if e != nil {
				// If parsing fails, try to parse as integer and assume hours
				if intVal, err := strconv.Atoi(v); err == nil {
					return time.Duration(intVal) * time.Hour, nil
				}
				return 0, fmt.Errorf("invalid %s: %w", key, e)
			}
			return d, nil
		}
		return def, nil
	}

	getString := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	// Auth settings
	cfg.SecretKey = os.Getenv("SECRET_KEY")
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("SECRET_KEY is required")
	}

	if cfg.AuthExpiry, err = getDuration("AUTH_EXPIRY", 24*time.Hour); err != nil {
		return nil, err
	}

	// DB settings
	cfg.DBConfig.URL = os.Getenv("DB_URL")
	if cfg.DBConfig.URL == "" {
		return nil, fmt.Errorf("DB_URL is required")
	}
	if cfg.DBConfig.MaxOpenConn, err = getInt("DB_MAX_OPEN_CONN", 10); err != nil {
		return nil, err
	}
	if cfg.DBConfig.ConnMaxIdle, err = getDuration("DB_CONN_MAX_IDLE", 5*time.Minute); err != nil {
		return nil, err
	}

	// App settings
	port, err := getInt("PORT", 8081)
	if err != nil {
		return nil, err
	}
	cfg.AppCfg.Port = strconv.Itoa(port)

	// OTLP tracing configuration - use standard OpenTelemetry environment variables
	cfg.OTLPConfig.Endpoint = getString("OTEL_EXPORTER_OTLP_ENDPOINT", "hcaas_jaeger_all_in_one:4317")
	cfg.OTLPConfig.Protocol = getString("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	cfg.OTLPConfig.Insecure = getString("OTEL_EXPORTER_OTLP_INSECURE", "true") == "true"

	return cfg, nil
}
