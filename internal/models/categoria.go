package models

// CategoriaRelevancia representa uma categoria com sua relevância calculada
type CategoriaRelevancia struct {
	Nome               string  `json:"nome"`
	NomeNormalizado    string  `json:"nome_normalizado"`
	RelevanciaTotal    int     `json:"relevancia_total"`
	QuantidadeServicos int     `json:"quantidade_servicos"`
	RelevanciaMedia    float64 `json:"relevancia_media"`
}

// CategoriasRelevanciaResponse representa a resposta do endpoint de categorias por relevância
type CategoriasRelevanciaResponse struct {
	Categorias        []CategoriaRelevancia `json:"categorias"`
	TotalCategorias   int                   `json:"total_categorias"`
	UltimaAtualizacao string                `json:"ultima_atualizacao"`
} 