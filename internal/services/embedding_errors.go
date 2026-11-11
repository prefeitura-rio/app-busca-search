package services

import (
	"context"
	"errors"
	"log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Motivos de degradação registrados em log e no span
const (
	fallbackEmbeddingUnavailable   = "embedding_unavailable"
	fallbackEmbeddingNotConfigured = "embedding_service_not_configured"
)

// ErrEmbeddingUnavailable indica que o embedding da query não pôde ser gerado
// (Gemini não configurado, fora do ar ou recusando a chamada). A mensagem é
// segura para o cliente; o erro original do provedor fica só no log do servidor.
var ErrEmbeddingUnavailable = errors.New("busca semântica indisponível no momento")

// recordEmbeddingFallback registra, num formato estável para alerta em log
// (`search_degraded`) e no span ativo (`search.degraded`), que uma busca que
// dependia de embeddings foi executada como keyword.
func recordEmbeddingFallback(ctx context.Context, apiVersion string, requested string, reason string, cause error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.Bool("search.degraded", true),
		attribute.String("search.fallback_reason", reason),
	)
	if cause != nil {
		span.RecordError(cause)
	}
	log.Printf("WARN search_degraded api=%s requested=%s executed=keyword reason=%s error=%q",
		apiVersion, requested, reason, errorText(cause))
}

// logEmbeddingFailure registra a falha de embedding de uma busca semântica, que
// não tem fallback e responde ErrEmbeddingUnavailable ao cliente.
func logEmbeddingFailure(apiVersion string, cause error) {
	log.Printf("ERROR search_embedding_failed api=%s requested=semantic error=%q", apiVersion, errorText(cause))
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
