package models

// CategoriaRelevancia representa uma categoria com sua relevância calculada
// DEPRECATED: Manter para compatibilidade, usar Category para novos endpoints
type CategoriaRelevancia struct {
	Nome               string  `json:"nome"`
	NomeNormalizado    string  `json:"nome_normalizado"`
	RelevanciaTotal    int     `json:"relevancia_total"`
	QuantidadeServicos int     `json:"quantidade_servicos"`
	RelevanciaMedia    float64 `json:"relevancia_media"`
}

// CategoriasRelevanciaResponse representa a resposta do endpoint de categorias por relevância
// DEPRECATED: Manter para compatibilidade, usar CategoryResponse para novos endpoints
type CategoriasRelevanciaResponse struct {
	Categorias        []CategoriaRelevancia `json:"categorias"`
	TotalCategorias   int                   `json:"total_categorias"`
	UltimaAtualizacao string                `json:"ultima_atualizacao"`
}

// ========================================
// NOVO SISTEMA DE CATEGORIAS
// ========================================

// Category representa uma categoria com contador e score de popularidade
type Category struct {
	Name            string `json:"name"`
	Count           int    `json:"count"`
	PopularityScore int    `json:"popularity_score"`
}

// CategoryRequest representa requisição de categorias
type CategoryRequest struct {
	SortBy          string `form:"sort_by"`           // popularity, count, alpha
	Order           string `form:"order"`             // asc, desc
	IncludeEmpty    bool   `form:"include_empty"`     // incluir categorias sem serviços
	IncludeInactive bool   `form:"include_inactive"`  // incluir serviços inativos (status != 1)
	FilterCategory  string `form:"filter_category"`   // filtrar serviços por categoria
	Page            int    `form:"page"`              // página para serviços filtrados
	PerPage         int    `form:"per_page"`          // resultados por página
}

// FilteredCategoryResult resultado de serviços filtrados por categoria
type FilteredCategoryResult struct {
	Name          string             `json:"name"`
	Services      []*ServiceDocument `json:"services"`
	TotalServices int                `json:"total_services"`
	Page          int                `json:"page"`
	PerPage       int                `json:"per_page"`
}

// CategoryResponse resposta do endpoint de categorias
type CategoryResponse struct {
	Categories       []*Category             `json:"categories"`
	TotalCategories  int                     `json:"total_categories"`
	FilteredCategory *FilteredCategoryResult `json:"filtered_category,omitempty"`
	Metadata         map[string]interface{}  `json:"metadata"`
} 