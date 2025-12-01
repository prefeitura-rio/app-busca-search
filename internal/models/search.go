package models

// SearchType define os tipos de busca disponíveis
type SearchType string

const (
	SearchTypeKeyword  SearchType = "keyword"
	SearchTypeSemantic SearchType = "semantic"
	SearchTypeHybrid   SearchType = "hybrid"
	SearchTypeAI       SearchType = "ai"
)

// ScoreThreshold representa thresholds mínimos de score por tipo de busca
type ScoreThreshold struct {
	Keyword  *float64 `form:"threshold_keyword" json:"keyword,omitempty"`
	Semantic *float64 `form:"threshold_semantic" json:"semantic,omitempty"`
	Hybrid   *float64 `form:"threshold_hybrid" json:"hybrid,omitempty"`
	AI       *float64 `form:"threshold_ai" json:"ai,omitempty"`
}

// AIScore estrutura multi-dimensional de scoring via LLM
type AIScore struct {
	ServiceID         string  `json:"service_id"`          // ID do serviço (para mapeamento batch)
	RelevanceCategory string  `json:"relevance_category"`  // Categoria de relevância (da AI)
	ConfidenceLevel   string  `json:"confidence_level"`    // Nível de confiança (da AI)
	Relevance         float64 `json:"relevance"`           // Score numérico (calculado por nós)
	Confidence        float64 `json:"confidence"`          // Score numérico (calculado por nós)
	ExactMatch        bool    `json:"exact_match"`         // Match exato com a query
	FinalScore        float64 `json:"final_score"`         // Score combinado (calculado por nós)
	Reasoning         string  `json:"reasoning,omitempty"` // Breve explicação
}

// Categorias de relevância (usadas pela AI)
const (
	RelevanceIrrelevant = "irrelevante"
	RelevanceLow        = "pouco_relevante"
	RelevanceModerate   = "relevante"
	RelevanceHigh       = "muito_relevante"
	RelevanceExact      = "match_exato"
)

// Níveis de confiança (usados pela AI)
const (
	ConfidenceLow      = "baixa"
	ConfidenceMedium   = "media"
	ConfidenceHigh     = "alta"
	ConfidenceVeryHigh = "muito_alta"
)

// ScoreInfo contém informações sobre os scores de relevância de um documento
type ScoreInfo struct {
	TextMatchNormalized *float64 `json:"text_match_normalized,omitempty"` // Score normalizado 0-1 do text_match
	VectorSimilarity    *float64 `json:"vector_similarity,omitempty"`     // Similaridade vetorial 0-1 (1 = idêntico)
	HybridScore         *float64 `json:"hybrid_score,omitempty"`          // Score híbrido combinado 0-1
	RecencyFactor       *float64 `json:"recency_factor,omitempty"`        // Fator de recência aplicado (1.0 = recente, decai com o tempo)
	FinalScore          *float64 `json:"final_score,omitempty"`           // Score final após aplicar recency boost
	ThresholdApplied    string   `json:"threshold_applied,omitempty"`     // Tipo de threshold aplicado: "keyword", "semantic", "hybrid", "none"
	ThresholdValue      *float64 `json:"threshold_value,omitempty"`       // Valor do threshold aplicado
	PassedThreshold     bool     `json:"passed_threshold"`                // Se passou no threshold
}

// SearchRequest representa uma requisição de busca
type SearchRequest struct {
	Query                 string          `form:"q" binding:"required"`
	Type                  SearchType      `form:"type" binding:"required"`
	Page                  int             `form:"page"`
	PerPage               int             `form:"per_page"`
	IncludeInactive       bool            `form:"include_inactive"`
	Alpha                 float64         `form:"alpha"` // Para hybrid (default 0.3)
	ScoreThreshold        *ScoreThreshold `form:"score_threshold,omitempty"`
	ExcludeAgentExclusive *bool           `form:"exclude_agent_exclusive"`
	GenerateScores        bool            `form:"generate_scores"` // Gerar AI scores via LLM (apenas para type=ai)
	RecencyBoost          bool            `form:"recency_boost"`   // Aplica boost por recência (docs recentes têm score maior)
}

// ServiceDocument representa um documento de serviço retornado pela busca
type ServiceDocument struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Subcategory *string                `json:"subcategory,omitempty"`
	Status      int32                  `json:"status"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// SearchResponse representa a resposta de uma busca
type SearchResponse struct {
	Results       []*ServiceDocument     `json:"results"`
	TotalCount    int                    `json:"total_count"`    // Total original do Typesense
	FilteredCount int                    `json:"filtered_count"` // Após aplicar thresholds
	Page          int                    `json:"page"`
	PerPage       int                    `json:"per_page"`
	SearchType    SearchType             `json:"search_type"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"` // Para AI search
}

// AISearchMetrics métricas do AI Agent Search
type AISearchMetrics struct {
	GeminiCalls    int     `json:"gemini_calls"`
	RerankExecuted bool    `json:"rerank_executed"`
	TotalTime      float64 `json:"total_time_ms"`
}

// QueryAnalysis análise estruturada da query pelo LLM
type QueryAnalysis struct {
	Intent         string   `json:"intent"`          // buscar_servico, listar_categoria, esclarecer_duvida
	Keywords       []string `json:"keywords"`        // palavras-chave extraídas
	Categories     []string `json:"categories"`      // categorias inferidas
	RefinedQueries []string `json:"refined_queries"` // max 2 variações da query
	SearchStrategy string   `json:"search_strategy"` // hybrid, semantic, keyword
	Confidence     float64  `json:"confidence"`      // 0-1
	PortalTags     []string `json:"portal_tags"`     // portal inferido
}

// ============================================================================
// v2 API Models - Multi-Collection Search
// ============================================================================

// UnifiedDocument represents a document from any collection (v2 API)
// Uses pure data passthrough - no field normalization
type UnifiedDocument struct {
	ID         string                 `json:"id"`
	Collection string                 `json:"collection"` // Which collection this document belongs to
	Type       string                 `json:"type"`       // Document type from collection config (service, course, job, etc.)
	Data       map[string]interface{} `json:"data"`       // Raw document data from Typesense
	ScoreInfo  *ScoreInfo             `json:"score_info,omitempty"`
}

// UnifiedSearchResponse represents multi-collection search response (v2 API)
type UnifiedSearchResponse struct {
	Results       []*UnifiedDocument     `json:"results"`
	TotalCount    int                    `json:"total_count"`    // Total original do Typesense (across all collections)
	FilteredCount int                    `json:"filtered_count"` // Após aplicar thresholds
	Page          int                    `json:"page"`
	PerPage       int                    `json:"per_page"`
	SearchType    SearchType             `json:"search_type"`
	Collections   []string               `json:"collections"`        // Which collections were searched
	Metadata      map[string]interface{} `json:"metadata,omitempty"` // Para AI search
}
