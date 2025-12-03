package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// CollectionConfig holds field mapping configuration for a Typesense collection
type CollectionConfig struct {
	Type          string   `json:"type"`                     // "service", "course", "job"
	TitleField    string   `json:"title_field"`              // Field name for title (used in response mapping)
	DescField     string   `json:"desc_field"`               // Field name for description (used in response mapping)
	FilterField   string   `json:"filter_field,omitempty"`   // Optional: field to filter by (e.g., "status")
	FilterValue   string   `json:"filter_value,omitempty"`   // Optional: value to filter for (e.g., "1")
	SearchFields  []string `json:"search_fields,omitempty"`  // Fields to search (query_by). Falls back to [title_field, desc_field]
	SearchWeights []int    `json:"search_weights,omitempty"` // Weights for search fields (query_by_weights). Falls back to [3, 1]
}

// GetSearchFields returns the fields to search, with fallback to title and desc
func (c *CollectionConfig) GetSearchFields() string {
	if len(c.SearchFields) > 0 {
		return strings.Join(c.SearchFields, ",")
	}
	return fmt.Sprintf("%s,%s", c.TitleField, c.DescField)
}

// GetSearchWeights returns the weights as a comma-separated string
func (c *CollectionConfig) GetSearchWeights() string {
	if len(c.SearchWeights) > 0 {
		weights := make([]string, len(c.SearchWeights))
		for i, w := range c.SearchWeights {
			weights[i] = fmt.Sprintf("%d", w)
		}
		return strings.Join(weights, ",")
	}
	return "3,1"
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
