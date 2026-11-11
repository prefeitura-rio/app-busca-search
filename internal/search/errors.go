package search

import (
	"context"
	"errors"
	"log"

	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrNoCollections     = errors.New("nenhuma collection configurada")
	ErrInvalidCollection = errors.New("collection não encontrada na configuração")
	ErrEmbeddingService  = errors.New("serviço de embedding não disponível")
	ErrSearchCanceled    = errors.New("busca cancelada")
	ErrTypesenseFailed   = errors.New("falha na comunicação com Typesense")
)

// recordEmbeddingFallback registra, num formato estável para alerta em log
// (`search_degraded`) e no span ativo (`search.degraded`), que uma busca que
// dependia de embeddings foi executada como keyword.
func recordEmbeddingFallback(ctx context.Context, requested v3.SearchType, reason string, cause error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.Bool("search.degraded", true),
		attribute.String("search.fallback_reason", reason),
	)
	errText := ""
	if cause != nil {
		span.RecordError(cause)
		errText = cause.Error()
	}
	log.Printf("WARN search_degraded api=v3 requested=%s executed=keyword reason=%s error=%q", requested, reason, errText)
}
