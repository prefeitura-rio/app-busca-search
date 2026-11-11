package services

import (
	"errors"
	"log"
)

// ErrEmbeddingUnavailable indica que o embedding da query não pôde ser gerado
// (Gemini não configurado, fora do ar ou recusando a chamada). A mensagem é
// segura para o cliente; o erro original do provedor fica só no log do servidor.
var ErrEmbeddingUnavailable = errors.New("busca semântica indisponível no momento")

// logEmbeddingFallback registra, num formato estável para alerta em log
// (`search_degraded`), que uma busca que dependia de embeddings foi executada
// como keyword.
func logEmbeddingFallback(apiVersion string, requested string, cause error) {
	log.Printf("WARN search_degraded api=%s requested=%s executed=keyword reason=embedding_unavailable error=%q",
		apiVersion, requested, errorText(cause))
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
