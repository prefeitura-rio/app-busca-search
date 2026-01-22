package search

import "errors"

var (
	ErrNoCollections     = errors.New("nenhuma collection configurada")
	ErrInvalidCollection = errors.New("collection não encontrada na configuração")
	ErrEmbeddingService  = errors.New("serviço de embedding não disponível")
	ErrSearchCanceled    = errors.New("busca cancelada")
	ErrTypesenseFailed   = errors.New("falha na comunicação com Typesense")
)
