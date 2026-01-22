package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"google.golang.org/genai"
)

// GeminiConfig configuração para o adapter Gemini
type GeminiConfig struct {
	EmbeddingModel       string
	ChatModel            string
	EmbeddingDimensions  int
	MaxTextLength        int
	CacheTTLMinutes      int
	CacheMaxSize         int
}

// DefaultGeminiConfig retorna configuração padrão
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		EmbeddingModel:      "text-embedding-004",
		ChatModel:           "gemini-2.0-flash",
		EmbeddingDimensions: 768,
		MaxTextLength:       10000,
		CacheTTLMinutes:     30,
		CacheMaxSize:        1000,
	}
}

// GeminiAdapter encapsula operações com Gemini API
type GeminiAdapter struct {
	client *genai.Client
	config GeminiConfig
	cache  *EmbeddingCache
}

// EmbeddingCache cache de embeddings
type EmbeddingCache struct {
	data    map[string]cachedEmbedding
	mu      sync.RWMutex
	ttl     time.Duration
	maxSize int
}

type cachedEmbedding struct {
	embedding []float32
	timestamp time.Time
}

// NewGeminiAdapter cria um novo adapter para Gemini
func NewGeminiAdapter(client *genai.Client, cfg GeminiConfig) *GeminiAdapter {
	if cfg.EmbeddingModel == "" {
		cfg.EmbeddingModel = DefaultGeminiConfig().EmbeddingModel
	}
	if cfg.ChatModel == "" {
		cfg.ChatModel = DefaultGeminiConfig().ChatModel
	}
	if cfg.EmbeddingDimensions == 0 {
		cfg.EmbeddingDimensions = DefaultGeminiConfig().EmbeddingDimensions
	}
	if cfg.MaxTextLength == 0 {
		cfg.MaxTextLength = DefaultGeminiConfig().MaxTextLength
	}
	if cfg.CacheTTLMinutes == 0 {
		cfg.CacheTTLMinutes = DefaultGeminiConfig().CacheTTLMinutes
	}
	if cfg.CacheMaxSize == 0 {
		cfg.CacheMaxSize = DefaultGeminiConfig().CacheMaxSize
	}

	return &GeminiAdapter{
		client: client,
		config: cfg,
		cache:  NewEmbeddingCache(time.Duration(cfg.CacheTTLMinutes)*time.Minute, cfg.CacheMaxSize),
	}
}

// NewEmbeddingCache cria um novo cache de embeddings
func NewEmbeddingCache(ttl time.Duration, maxSize int) *EmbeddingCache {
	return &EmbeddingCache{
		data:    make(map[string]cachedEmbedding),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// GenerateEmbedding gera embedding para um texto
func (g *GeminiAdapter) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if g.client == nil {
		return nil, fmt.Errorf("cliente Gemini não inicializado")
	}

	// Trunca texto longo
	if len(text) > g.config.MaxTextLength {
		text = text[:g.config.MaxTextLength]
	}

	// Verifica cache
	cacheKey := g.cacheKey(text)
	if cached := g.cache.Get(cacheKey); cached != nil {
		return cached, nil
	}

	// Gera embedding
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	content := genai.NewContentFromText(text, genai.RoleUser)
	outputDim := int32(g.config.EmbeddingDimensions)
	embedConfig := &genai.EmbedContentConfig{
		OutputDimensionality: &outputDim,
	}

	resp, err := g.client.Models.EmbedContent(ctx, g.config.EmbeddingModel, []*genai.Content{content}, embedConfig)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar embedding: %w", err)
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("nenhum embedding retornado")
	}

	embedding := resp.Embeddings[0].Values
	if len(embedding) != g.config.EmbeddingDimensions {
		return nil, fmt.Errorf("embedding com dimensões incorretas: %d (esperado: %d)", len(embedding), g.config.EmbeddingDimensions)
	}

	// Armazena no cache
	g.cache.Set(cacheKey, embedding)

	return embedding, nil
}

// IsAvailable verifica se o cliente está disponível
func (g *GeminiAdapter) IsAvailable() bool {
	return g.client != nil
}

// GetClient retorna o cliente Gemini para uso direto
func (g *GeminiAdapter) GetClient() *genai.Client {
	return g.client
}

// GetChatModel retorna o modelo de chat configurado
func (g *GeminiAdapter) GetChatModel() string {
	return g.config.ChatModel
}

// GetConfig retorna a configuração do adapter
func (g *GeminiAdapter) GetConfig() GeminiConfig {
	return g.config
}

func (g *GeminiAdapter) cacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

// Get retorna embedding do cache
func (c *EmbeddingCache) Get(key string) []float32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cached, ok := c.data[key]; ok {
		if time.Since(cached.timestamp) < c.ttl {
			return cached.embedding
		}
	}
	return nil
}

// Set armazena embedding no cache
func (c *EmbeddingCache) Set(key string, embedding []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Limpa entradas expiradas se cache está cheio
	if len(c.data) >= c.maxSize {
		c.cleanup()
	}

	c.data[key] = cachedEmbedding{
		embedding: embedding,
		timestamp: time.Now(),
	}
}

func (c *EmbeddingCache) cleanup() {
	now := time.Now()
	for key, cached := range c.data {
		if now.Sub(cached.timestamp) > c.ttl {
			delete(c.data, key)
		}
	}
}
