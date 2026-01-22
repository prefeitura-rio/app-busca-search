package search

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
)

// SearchCache armazena resultados de busca em memória
type SearchCache struct {
	data    map[string]*CachedResult
	mu      sync.RWMutex
	ttl     time.Duration
	maxSize int
}

// CachedResult representa um resultado em cache
type CachedResult struct {
	Response  *v3.SearchResponse
	Timestamp time.Time
}

// NewSearchCache cria um novo cache de busca
func NewSearchCache(ttl time.Duration, maxSize int) *SearchCache {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	if maxSize <= 0 {
		maxSize = 500
	}
	return &SearchCache{
		data:    make(map[string]*CachedResult),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get busca um resultado no cache
func (c *SearchCache) Get(key string) *v3.SearchResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cached, ok := c.data[key]; ok {
		if time.Since(cached.Timestamp) < c.ttl {
			return cached.Response
		}
	}
	return nil
}

// Set armazena um resultado no cache
func (c *SearchCache) Set(key string, response *v3.SearchResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Limpa entradas expiradas se cache está cheio
	if len(c.data) >= c.maxSize {
		c.cleanup()
	}

	c.data[key] = &CachedResult{
		Response:  response,
		Timestamp: time.Now(),
	}
}

// GenerateKey gera uma chave única para a requisição
func (c *SearchCache) GenerateKey(req *v3.SearchRequest) string {
	// Cria uma string com os parâmetros relevantes
	keyData := fmt.Sprintf(
		"%s|%s|%d|%d|%s|%s|%.2f|%.2f|%v|%v|%d|%s",
		req.Query,
		req.Type,
		req.Page,
		req.PerPage,
		req.Collections,
		req.Mode,
		req.Alpha,
		req.Threshold,
		req.Expand,
		req.Recency,
		safeIntPtr(req.Status),
		req.Category,
	)

	hash := sha256.Sum256([]byte(keyData))
	return hex.EncodeToString(hash[:16])
}

// cleanup remove entradas expiradas
func (c *SearchCache) cleanup() {
	now := time.Now()
	for key, cached := range c.data {
		if now.Sub(cached.Timestamp) > c.ttl {
			delete(c.data, key)
		}
	}

	// Se ainda está cheio, remove as mais antigas
	if len(c.data) >= c.maxSize {
		oldest := time.Now()
		oldestKey := ""
		for key, cached := range c.data {
			if cached.Timestamp.Before(oldest) {
				oldest = cached.Timestamp
				oldestKey = key
			}
		}
		if oldestKey != "" {
			delete(c.data, oldestKey)
		}
	}
}

// Clear limpa todo o cache
func (c *SearchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]*CachedResult)
}

// Stats retorna estatísticas do cache
func (c *SearchCache) Stats() (size int, expired int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	size = len(c.data)
	now := time.Now()
	for _, cached := range c.data {
		if now.Sub(cached.Timestamp) > c.ttl {
			expired++
		}
	}
	return
}

func safeIntPtr(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}
