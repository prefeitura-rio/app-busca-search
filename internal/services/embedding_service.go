package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/genai"
)

// EmbeddingProvider é a interface para geração de embeddings
type EmbeddingProvider interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	GenerateBatch(ctx context.Context, texts []string) ([][]float32, error)
	GetDimensions() int
	GetModelName() string
}

// GeminiEmbeddingProvider implementa EmbeddingProvider usando Google Gemini
type GeminiEmbeddingProvider struct {
	client     *genai.Client
	modelName  string
	dimensions int
	timeout    time.Duration
	cache      Cache
	maxRetries int
}

// NewGeminiEmbeddingProvider cria um novo provider de embeddings Gemini
func NewGeminiEmbeddingProvider(client *genai.Client, modelName string, cache Cache) *GeminiEmbeddingProvider {
	// Dimensão sempre 768 para embeddings Gemini
	dimensions := 768

	return &GeminiEmbeddingProvider{
		client:     client,
		modelName:  modelName,
		dimensions: dimensions,
		timeout:    15 * time.Second,
		cache:      cache,
		maxRetries: 3,
	}
}

// GenerateEmbedding gera um embedding para um texto
func (g *GeminiEmbeddingProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Truncar texto se muito longo (limite do Gemini)
	const maxChars = 10000
	if len(text) > maxChars {
		text = text[:maxChars]
	}

	// Verificar cache primeiro
	cacheKey := g.getCacheKey(text)
	if cached := g.cache.Get(cacheKey); cached != nil {
		return cached.([]float32), nil
	}

	// Criar contexto com timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	// Tentar gerar embedding com retry
	var embedding []float32
	var lastErr error

	for attempt := 1; attempt <= g.maxRetries; attempt++ {
		embedding, lastErr = g.generateWithTimeout(ctxWithTimeout, text)
		if lastErr == nil {
			// Sucesso - armazenar no cache
			g.cache.Set(cacheKey, embedding, 30*time.Minute)
			return embedding, nil
		}

		// Se foi context canceled, não fazer retry
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context canceled: %w", ctx.Err())
		}

		// Log do erro e retry
		if attempt < g.maxRetries {
			log.Printf("Embedding generation failed (attempt %d/%d): %v, retrying...", attempt, g.maxRetries, lastErr)
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", g.maxRetries, lastErr)
}

// generateWithTimeout gera embedding com o contexto fornecido
func (g *GeminiEmbeddingProvider) generateWithTimeout(ctx context.Context, text string) ([]float32, error) {
	content := genai.NewContentFromText(text, genai.RoleUser)
	// Configurar para gerar embeddings com 768 dimensões
	outputDim := int32(768)
	config := &genai.EmbedContentConfig{
		OutputDimensionality: &outputDim,
	}

	resp, err := g.client.Models.EmbedContent(ctx, g.modelName, []*genai.Content{content}, config)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar embedding: %w", err)
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("nenhum embedding foi gerado")
	}

	embedding := resp.Embeddings[0].Values

	// Validar que embedding tem 768 dimensões
	if len(embedding) != 768 {
		return nil, fmt.Errorf("embedding retornou %d dimensões, esperado 768", len(embedding))
	}

	return embedding, nil
}

// GenerateBatch gera embeddings para múltiplos textos em lote
func (g *GeminiEmbeddingProvider) GenerateBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	embeddings := make([][]float32, len(texts))
	errors := make([]error, len(texts))

	// Processar em paralelo (máximo 5 concorrentes para não sobrecarregar a API)
	const maxConcurrent = 5
	semaphore := make(chan struct{}, maxConcurrent)

	for i, text := range texts {
		semaphore <- struct{}{} // Adquirir slot

		go func(index int, txt string) {
			defer func() { <-semaphore }() // Liberar slot

			embedding, err := g.GenerateEmbedding(ctx, txt)
			embeddings[index] = embedding
			errors[index] = err
		}(i, text)
	}

	// Aguardar todas as goroutines
	for i := 0; i < maxConcurrent; i++ {
		semaphore <- struct{}{}
	}

	// Verificar se houve erros
	var failedIndices []int
	for i, err := range errors {
		if err != nil {
			failedIndices = append(failedIndices, i)
		}
	}

	if len(failedIndices) > 0 {
		return embeddings, fmt.Errorf("falha ao gerar %d/%d embeddings (índices: %v)",
			len(failedIndices), len(texts), failedIndices)
	}

	return embeddings, nil
}

// GetDimensions retorna o número de dimensões dos embeddings
func (g *GeminiEmbeddingProvider) GetDimensions() int {
	return g.dimensions
}

// GetModelName retorna o nome do modelo usado
func (g *GeminiEmbeddingProvider) GetModelName() string {
	return g.modelName
}

// getCacheKey gera uma chave de cache a partir do texto
func (g *GeminiEmbeddingProvider) getCacheKey(text string) string {
	// Usar hash SHA256 para gerar chave única
	hash := sha256.Sum256([]byte(text))
	return "embedding:" + hex.EncodeToString(hash[:])
}

// FormatEmbeddingForTypesense formata um embedding para uso no Typesense
func FormatEmbeddingForTypesense(embedding []float32) string {
	// Converter []float32 para string no formato "[0.1,0.2,0.3,...]"
	strValues := make([]string, len(embedding))
	for i, v := range embedding {
		strValues[i] = fmt.Sprintf("%f", v)
	}
	return strings.Join(strValues, ",")
}

// ParseEmbeddingFromTypesense converte string do Typesense de volta para []float32
func ParseEmbeddingFromTypesense(embeddingStr string) ([]float32, error) {
	if embeddingStr == "" {
		return nil, fmt.Errorf("embedding string vazia")
	}

	parts := strings.Split(embeddingStr, ",")
	embedding := make([]float32, len(parts))

	for i, part := range parts {
		var val float32
		_, err := fmt.Sscanf(part, "%f", &val)
		if err != nil {
			return nil, fmt.Errorf("erro ao parsear embedding no índice %d: %w", i, err)
		}
		embedding[i] = val
	}

	return embedding, nil
}
