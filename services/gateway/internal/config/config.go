// Package config provides configuration management for the API Gateway service.
//
// It follows Google's configuration patterns with environment-based configuration,
// validation, and structured logging of configuration parameters.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the API Gateway service.
// It uses struct tags for validation and follows Google's naming conventions.
type Config struct {
	// Server configuration
	Server ServerConfig `validate:"required"`

	// Services configuration for upstream services
	Services ServicesConfig `validate:"required"`

	// Security configuration
	Security SecurityConfig `validate:"required"`

	// Observability configuration
	Observability ObservabilityConfig `validate:"required"`

	// Rate limiting configuration
	RateLimit RateLimitConfig `validate:"required"`
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	Port            string        `validate:"required"`
	Host            string        
	ReadTimeout     time.Duration `validate:"min=1s"`
	WriteTimeout    time.Duration `validate:"min=1s"`
	IdleTimeout     time.Duration `validate:"min=1s"`
	ShutdownTimeout time.Duration `validate:"min=1s"`
}

// ServicesConfig defines upstream service endpoints.
type ServicesConfig struct {
	AuthService         ServiceEndpoint `validate:"required"`
	URLService          ServiceEndpoint `validate:"required"`
	NotificationService ServiceEndpoint `validate:"required"`
}

// ServiceEndpoint represents a single upstream service configuration.
type ServiceEndpoint struct {
	BaseURL string        `validate:"required,url"`
	Timeout time.Duration `validate:"min=1s"`
	Retries int           `validate:"min=0,max=5"`
}

// SecurityConfig contains JWT and security-related configuration.
type SecurityConfig struct {
	JWTSecret     string `validate:"required,min=32"`
	AllowedOrigins []string
	TrustedProxies []string
}

// ObservabilityConfig contains monitoring and tracing configuration.
type ObservabilityConfig struct {
	MetricsEnabled bool
	TracingEnabled bool
	OTELEndpoint   string
	ServiceName    string `validate:"required"`
}

// RateLimitConfig defines rate limiting parameters.
type RateLimitConfig struct {
	Enabled     bool
	RequestsPerSecond int `validate:"min=1"`
	BurstSize   int `validate:"min=1"`
}

// Load reads configuration from environment variables with sensible defaults.
// This follows Google's practice of environment-based configuration with validation.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnvOrDefault("GATEWAY_PORT", "8080"),
			Host:            getEnvOrDefault("GATEWAY_HOST", "0.0.0.0"),
			ReadTimeout:     getDurationEnvOrDefault("GATEWAY_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getDurationEnvOrDefault("GATEWAY_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:     getDurationEnvOrDefault("GATEWAY_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getDurationEnvOrDefault("GATEWAY_SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		Services: ServicesConfig{
			AuthService: ServiceEndpoint{
				BaseURL: getEnvOrDefault("AUTH_SERVICE_URL", "http://hcaas_auth:8081"),
				Timeout: getDurationEnvOrDefault("AUTH_SERVICE_TIMEOUT", 10*time.Second),
				Retries: getIntEnvOrDefault("AUTH_SERVICE_RETRIES", 3),
			},
			URLService: ServiceEndpoint{
				BaseURL: getEnvOrDefault("URL_SERVICE_URL", "http://hcaas_web:8080"),
				Timeout: getDurationEnvOrDefault("URL_SERVICE_TIMEOUT", 10*time.Second),
				Retries: getIntEnvOrDefault("URL_SERVICE_RETRIES", 3),
			},
			NotificationService: ServiceEndpoint{
				BaseURL: getEnvOrDefault("NOTIFICATION_SERVICE_URL", "http://hcaas_notification:8082"),
				Timeout: getDurationEnvOrDefault("NOTIFICATION_SERVICE_TIMEOUT", 10*time.Second),
				Retries: getIntEnvOrDefault("NOTIFICATION_SERVICE_RETRIES", 3),
			},
		},
		Security: SecurityConfig{
			JWTSecret: getEnvOrDefault("JWT_SECRET", "your-super-secret-jwt-key-change-in-production-min-32-chars"),
			AllowedOrigins: getSliceEnvOrDefault("ALLOWED_ORIGINS", []string{"*"}),
			TrustedProxies: getSliceEnvOrDefault("TRUSTED_PROXIES", []string{}),
		},
		Observability: ObservabilityConfig{
			MetricsEnabled: getBoolEnvOrDefault("METRICS_ENABLED", true),
			TracingEnabled: getBoolEnvOrDefault("TRACING_ENABLED", true),
			OTELEndpoint:   getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "http://hcaas_jaeger_all_in_one:4317"),
			ServiceName:    getEnvOrDefault("OTEL_SERVICE_NAME", "hcaas_gateway_service"),
		},
		RateLimit: RateLimitConfig{
			Enabled:           getBoolEnvOrDefault("RATE_LIMIT_ENABLED", true),
			RequestsPerSecond: getIntEnvOrDefault("RATE_LIMIT_RPS", 100),
			BurstSize:         getIntEnvOrDefault("RATE_LIMIT_BURST", 200),
		},
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// validate performs basic validation on the configuration.
// In a production system, you might use a validation library like go-playground/validator.
func (c *Config) validate() error {
	if c.Security.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len(c.Security.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters long")
	}
	if c.Server.Port == "" {
		return fmt.Errorf("GATEWAY_PORT is required")
	}
	if c.Services.AuthService.BaseURL == "" {
		return fmt.Errorf("AUTH_SERVICE_URL is required")
	}
	if c.Services.URLService.BaseURL == "" {
		return fmt.Errorf("URL_SERVICE_URL is required")
	}
	if c.Services.NotificationService.BaseURL == "" {
		return fmt.Errorf("NOTIFICATION_SERVICE_URL is required")
	}
	return nil
}

// Helper functions for environment variable parsing

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDurationEnvOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getIntEnvOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getBoolEnvOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getSliceEnvOrDefault(key string, defaultValue []string) []string {
	// In a real implementation, you might parse comma-separated values
	// For now, return the default
	return defaultValue
}
