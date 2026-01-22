package v3

// SearchResponse representa a resposta de busca v3
type SearchResponse struct {
	Results    []Document `json:"results"`
	Pagination Pagination `json:"pagination"`
	Query      QueryMeta  `json:"query"`
	Timing     TimingMeta `json:"timing"`
}

// Document representa um documento retornado pela busca
type Document struct {
	ID         string `json:"id"`
	Collection string `json:"collection"`
	Type       string `json:"type"`

	// Campos padronizados
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Category    string  `json:"category,omitempty"`
	Slug        string  `json:"slug,omitempty"`
	URL         string  `json:"url,omitempty"`

	// Score
	Score ScoreInfo `json:"score"`

	// Dados originais
	Data map[string]interface{} `json:"data"`
}

// ScoreInfo contém informações de score do documento
type ScoreInfo struct {
	Final   float64 `json:"final"`
	Text    float64 `json:"text,omitempty"`
	Vector  float64 `json:"vector,omitempty"`
	Recency float64 `json:"recency,omitempty"`
}

// Pagination contém informações de paginação
type Pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// QueryMeta contém metadados sobre a query processada
type QueryMeta struct {
	Original   string   `json:"original"`
	Normalized string   `json:"normalized,omitempty"`
	Expanded   []string `json:"expanded,omitempty"`
	Intent     string   `json:"intent,omitempty"`
}

// TimingMeta contém métricas de tempo
type TimingMeta struct {
	TotalMs     float64 `json:"total_ms"`
	ParsingMs   float64 `json:"parsing_ms,omitempty"`
	EmbeddingMs float64 `json:"embedding_ms,omitempty"`
	SearchMs    float64 `json:"search_ms"`
	RankingMs   float64 `json:"ranking_ms,omitempty"`
}

// NewPagination cria uma estrutura de paginação
func NewPagination(page, perPage, total int) Pagination {
	totalPages := total / perPage
	if total%perPage > 0 {
		totalPages++
	}
	return Pagination{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}
}
