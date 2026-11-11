package config

import (
	"log"
	"strings"
)

// DefaultGeminiEmbeddingModel é o modelo de embeddings vigente na API do Gemini.
// Gera vetores de 768 dimensões quando chamado com OutputDimensionality=768,
// compatível com o campo `embedding` (num_dim 768) das collections do Typesense.
const DefaultGeminiEmbeddingModel = "gemini-embedding-001"

// retiredGeminiEmbeddingModels são modelos de embedding desligados pelo Google
// (https://ai.google.dev/gemini-api/docs/deprecations): qualquer chamada a eles
// responde 404. text-embedding-004 foi desligado em 14/01/2026.
var retiredGeminiEmbeddingModels = map[string]bool{
	"text-embedding-004":         true,
	"embedding-001":              true,
	"embedding-gecko-001":        true,
	"gemini-embedding-exp":       true,
	"gemini-embedding-exp-03-07": true,
}

// ResolveGeminiEmbeddingModel devolve o modelo de embeddings a usar a partir do
// valor configurado. Vazio usa o default; um modelo já desligado pelo Google é
// trocado pelo default com um aviso no log, porque mantê-lo só faria toda busca
// semântica e híbrida falhar com 404.
func ResolveGeminiEmbeddingModel(configured string) string {
	name := strings.TrimSpace(configured)
	if name == "" {
		return DefaultGeminiEmbeddingModel
	}

	if retiredGeminiEmbeddingModels[strings.TrimPrefix(name, "models/")] {
		log.Printf("WARN GEMINI_EMBEDDING_MODEL=%q foi desligado pela API do Gemini; usando %q", name, DefaultGeminiEmbeddingModel)
		return DefaultGeminiEmbeddingModel
	}

	return name
}
