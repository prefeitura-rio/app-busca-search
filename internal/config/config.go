package config

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// CollectionConfig holds field mapping configuration for a Typesense collection
type CollectionConfig struct {
	Type        string `json:"type"`         // "service", "course", "job"
	TitleField  string `json:"title_field"`  // Field name for title
	DescField   string `json:"desc_field"`   // Field name for description
	FilterField string `json:"filter_field"` // Optional: field to filter by (e.g., "status")
	FilterValue string `json:"filter_value"` // Optional: value to filter for (e.g., "1")
}

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

	// Multi-collection search configuration (v2 API)
	SearchableCollections []string
	CollectionConfigs     map[string]*CollectionConfig
}

func LoadConfig() *Config {
	_ = godotenv.Load()

	cfg := &Config{
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

		CollectionConfigs: make(map[string]*CollectionConfig),
	}

	// Parse searchable collections (REQUIRED for v2 API)
	collectionsCSV := os.Getenv("SEARCHABLE_COLLECTIONS")
	if collectionsCSV == "" {
		log.Fatal("SEARCHABLE_COLLECTIONS environment variable is required but not set")
	}
	cfg.SearchableCollections = strings.Split(collectionsCSV, ",")
	for i := range cfg.SearchableCollections {
		cfg.SearchableCollections[i] = strings.TrimSpace(cfg.SearchableCollections[i])
	}

	// Parse collection configs JSON (REQUIRED for v2 API)
	configsJSON := os.Getenv("COLLECTION_CONFIGS")
	if configsJSON == "" {
		log.Fatal("COLLECTION_CONFIGS environment variable is required but not set")
	}

	if err := json.Unmarshal([]byte(configsJSON), &cfg.CollectionConfigs); err != nil {
		log.Fatalf("Failed to parse COLLECTION_CONFIGS JSON: %v", err)
	}

	// Validate that all searchable collections have configs
	for _, collName := range cfg.SearchableCollections {
		if _, exists := cfg.CollectionConfigs[collName]; !exists {
			log.Fatalf("Collection '%s' is in SEARCHABLE_COLLECTIONS but missing from COLLECTION_CONFIGS", collName)
		}
	}

	return cfg
}

// GetCollectionConfig returns the config for a specific collection
func (c *Config) GetCollectionConfig(name string) *CollectionConfig {
	return c.CollectionConfigs[name]
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
