package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	TypesenseHost     string
	TypesensePort     string
	TypesenseAPIKey   string
	TypesenseProtocol string

	ServerPort string

	GeminiAPIKey         string
	GeminiEmbeddingModel string

	// Tracing configuration
	TracingEnabled  bool
	TracingEndpoint string

	// Gateway configuration for URL wrapping
	GatewayBaseURL string
}

func LoadConfig() *Config {
	_ = godotenv.Load()

	return &Config{
		TypesenseHost:     getEnv("TYPESENSE_HOST", "localhost"),
		TypesensePort:     getEnv("TYPESENSE_PORT", "8108"),
		TypesenseAPIKey:   getEnv("TYPESENSE_API_KEY", ""),
		TypesenseProtocol: getEnv("TYPESENSE_PROTOCOL", "http"),

		ServerPort: getEnv("SERVER_PORT", "8080"),

		GeminiAPIKey:         getEnv("GEMINI_API_KEY", ""),
		GeminiEmbeddingModel: getEnv("GEMINI_EMBEDDING_MODEL", "gemini-embedding-001"),

		// Tracing configuration
		TracingEnabled:  getEnv("TRACING_ENABLED", "false") == "true",
		TracingEndpoint: getEnv("TRACING_ENDPOINT", "localhost:4317"),

		// Gateway configuration
		GatewayBaseURL: getEnv("GATEWAY_BASE_URL", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
