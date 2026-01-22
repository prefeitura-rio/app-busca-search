package v3

import "errors"

var (
	ErrQueryRequired       = errors.New("query é obrigatória")
	ErrInvalidSearchType   = errors.New("tipo de busca inválido (use: keyword, semantic, hybrid, ai)")
	ErrInvalidCollection   = errors.New("collection inválida")
	ErrEmbeddingFailed     = errors.New("falha ao gerar embedding")
	ErrSearchFailed        = errors.New("falha ao executar busca")
	ErrTypesenseConnection = errors.New("falha na conexão com Typesense")
	ErrNoResults           = errors.New("nenhum resultado encontrado")
)
