// Package config gerencia configurações da aplicação via variáveis de ambiente.
//
// # Variáveis de Ambiente
//
// ## Typesense
//   - TYPESENSE_HOST: Host do servidor Typesense (default: localhost)
//   - TYPESENSE_PORT: Porta do servidor (default: 8108)
//   - TYPESENSE_API_KEY: Chave de API do Typesense
//   - TYPESENSE_PROTOCOL: Protocolo http/https (default: http)
//
// ## Gemini
//   - GEMINI_API_KEY: Chave da API Google Gemini
//   - GEMINI_EMBEDDING_MODEL: Modelo para embeddings (default: text-embedding-004)
//   - GEMINI_CHAT_MODEL: Modelo para chat/análise (default: gemini-2.0-flash)
//
// ## Search v3
//   - SEARCH_V3_DEFAULT_ALPHA: Peso texto vs vetor para hybrid (default: 0.3)
//   - SEARCH_V3_DEFAULT_TYPOS_HUMAN: Tolerância a typos para modo human (default: 2)
//   - SEARCH_V3_DEFAULT_TYPOS_AGENT: Tolerância a typos para modo agent (default: 1)
//   - SEARCH_V3_EMBEDDING_DIMENSIONS: Dimensões do embedding (default: 768)
//   - SEARCH_V3_MAX_EMBEDDING_TEXT_LENGTH: Tamanho máximo do texto para embedding (default: 10000)
//   - SEARCH_V3_EMBEDDING_CACHE_TTL_MINUTES: TTL do cache de embeddings em minutos (default: 30)
//   - SEARCH_V3_EMBEDDING_CACHE_MAX_SIZE: Tamanho máximo do cache de embeddings (default: 1000)
//   - SEARCH_V3_ENABLE_QUERY_EXPANSION: Habilita expansão de query no modo human (default: true)
//   - SEARCH_V3_MAX_QUERY_EXPANSION_TERMS: Máximo de termos na expansão (default: 5)
//   - SEARCH_V3_ENABLE_RECENCY_BOOST: Habilita boost por recência no modo human (default: true)
//   - SEARCH_V3_RECENCY_GRACE_PERIOD_DAYS: Dias sem penalidade de recência (default: 30)
//   - SEARCH_V3_RECENCY_DECAY_RATE: Taxa de decaimento de recência (default: 0.00207, ~0.5 em 1 ano)
//   - SEARCH_V3_DEFAULT_COLLECTION: Collection padrão para v3 (default: prefrio_services_base)
//   - SEARCH_V3_LOAD_SYNONYMS_ON_STARTUP: Carregar sinônimos na inicialização (default: true)
package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
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

	// Gemini configuration
	GeminiAPIKey         string
	GeminiEmbeddingModel string
	GeminiChatModel      string

	// Tracing configuration
	TracingEnabled  bool
	TracingEndpoint string

	// Gateway configuration for URL wrapping
	GatewayBaseURL string

	// Multi-collection search configuration (v2 API)
	SearchableCollections []string
	CollectionConfigs     map[string]*CollectionConfig

	// Search v3 configuration
	SearchV3 SearchV3Config
}

// SearchV3Config contains v3 search-specific configuration
type SearchV3Config struct {
	// Default alpha for hybrid search (0-1, default 0.3)
	DefaultAlpha float64

	// Default typos tolerance for human mode (0-2, default 2)
	DefaultTyposHuman int

	// Default typos tolerance for agent mode (0-2, default 1)
	DefaultTyposAgent int

	// Embedding dimensions (default 768)
	EmbeddingDimensions int

	// Max text length for embedding (default 10000)
	MaxEmbeddingTextLength int

	// Embedding cache TTL in minutes (default 30)
	EmbeddingCacheTTLMinutes int

	// Embedding cache max size (default 1000)
	EmbeddingCacheMaxSize int

	// Enable query expansion by default for human mode (default true)
	EnableQueryExpansion bool

	// Max query expansion terms (default 5)
	MaxQueryExpansionTerms int

	// Enable recency boost by default for human mode (default true)
	EnableRecencyBoost bool

	// Recency grace period in days (default 30)
	RecencyGracePeriodDays int

	// Recency decay rate (default 0.00207, ~0.5 after 1 year)
	RecencyDecayRate float64

	// Default collection for v3 search
	DefaultCollection string

	// Load synonyms on startup (default true)
	LoadSynonymsOnStartup bool
}

func LoadConfig() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		TypesenseHost:     getEnv("TYPESENSE_HOST", "localhost"),
		TypesensePort:     getEnv("TYPESENSE_PORT", "8108"),
		TypesenseAPIKey:   getEnv("TYPESENSE_API_KEY", ""),
		TypesenseProtocol: getEnv("TYPESENSE_PROTOCOL", "http"),

		ServerPort: getEnv("SERVER_PORT", "8080"),

		// Gemini configuration
		GeminiAPIKey:         getEnv("GEMINI_API_KEY", ""),
		GeminiEmbeddingModel: getEnv("GEMINI_EMBEDDING_MODEL", "text-embedding-004"),
		GeminiChatModel:      getEnv("GEMINI_CHAT_MODEL", "gemini-3-pro-preview"),

		// Tracing configuration
		TracingEnabled:  getEnv("TRACING_ENABLED", "false") == "true",
		TracingEndpoint: getEnv("TRACING_ENDPOINT", "localhost:4317"),

		// Gateway configuration
		GatewayBaseURL: getEnv("GATEWAY_BASE_URL", ""),

		CollectionConfigs: make(map[string]*CollectionConfig),

		// Search v3 configuration
		SearchV3: SearchV3Config{
			DefaultAlpha:             getEnvFloat("SEARCH_V3_DEFAULT_ALPHA", 0.3),
			DefaultTyposHuman:        getEnvInt("SEARCH_V3_DEFAULT_TYPOS_HUMAN", 2),
			DefaultTyposAgent:        getEnvInt("SEARCH_V3_DEFAULT_TYPOS_AGENT", 1),
			EmbeddingDimensions:      getEnvInt("SEARCH_V3_EMBEDDING_DIMENSIONS", 768),
			MaxEmbeddingTextLength:   getEnvInt("SEARCH_V3_MAX_EMBEDDING_TEXT_LENGTH", 10000),
			EmbeddingCacheTTLMinutes: getEnvInt("SEARCH_V3_EMBEDDING_CACHE_TTL_MINUTES", 30),
			EmbeddingCacheMaxSize:    getEnvInt("SEARCH_V3_EMBEDDING_CACHE_MAX_SIZE", 1000),
			EnableQueryExpansion:     getEnv("SEARCH_V3_ENABLE_QUERY_EXPANSION", "true") == "true",
			MaxQueryExpansionTerms:   getEnvInt("SEARCH_V3_MAX_QUERY_EXPANSION_TERMS", 5),
			EnableRecencyBoost:       getEnv("SEARCH_V3_ENABLE_RECENCY_BOOST", "true") == "true",
			RecencyGracePeriodDays:   getEnvInt("SEARCH_V3_RECENCY_GRACE_PERIOD_DAYS", 30),
			RecencyDecayRate:         getEnvFloat("SEARCH_V3_RECENCY_DECAY_RATE", 0.00207),
			DefaultCollection:        getEnv("SEARCH_V3_DEFAULT_COLLECTION", "prefrio_services_base"),
			LoadSynonymsOnStartup:    getEnv("SEARCH_V3_LOAD_SYNONYMS_ON_STARTUP", "true") == "true",
		},
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

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}
