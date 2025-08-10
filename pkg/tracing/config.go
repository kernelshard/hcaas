package tracing

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the tracing configuration
type Config struct {
	// Service configuration
	ServiceName    string
	ServiceVersion string
	Environment    string

	// OpenTelemetry configuration
	OTLPExporterEndpoint string
	OTLPExporterInsecure bool

	// Sampling configuration
	SamplingRatio float64
	SamplingType  string // "probabilistic", "rate_limiting", "always_on", "always_off"

	// Google Cloud specific
	GCPProjectID string
	CloudRegion  string
	CloudZone    string

	// Resource attributes
	InstanceID string
	Hostname   string
}

// NewConfig creates a new tracing configuration from environment variables
func NewConfig() *Config {
	hostName := getEnv("HOSTNAME", "")
	instanceID := getEnv("INSTANCE_ID", hostName)

	return &Config{
		ServiceName:          getEnv("OTEL_SERVICE_NAME", "unknown-service"),
		ServiceVersion:       getEnv("OTEL_SERVICE_VERSION", "1.0.0"),
		Environment:          getEnv("ENVIRONMENT", "development"),
		OTLPExporterEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		OTLPExporterInsecure: getEnvBool("OTEL_EXPORTER_OTLP_INSECURE", true),
		SamplingRatio:        getEnvFloat("OTEL_TRACE_SAMPLE_RATIO", 1.0),
		SamplingType:         getEnv("OTEL_TRACE_SAMPLER", "probabilistic"),
		GCPProjectID:         getEnv("GOOGLE_CLOUD_PROJECT", ""),
		CloudRegion:          getEnv("CLOUD_REGION", ""),
		CloudZone:            getEnv("CLOUD_ZONE", ""),
		InstanceID:           instanceID,
		Hostname:             hostName,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return &ConfigError{Field: "ServiceName", Message: "service name cannot be empty"}
	}
	if c.OTLPExporterEndpoint == "" {
		return &ConfigError{Field: "OTLPExporterEndpoint", Message: "OTLP exporter endpoint cannot be empty"}
	}
	if c.SamplingRatio < 0 || c.SamplingRatio > 1 {
		return &ConfigError{Field: "SamplingRatio", Message: "sampling ratio must be between 0 and 1"}
	}
	return nil
}

// ConfigError represents a configuration error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: %s: %s", e.Field, e.Message)
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
