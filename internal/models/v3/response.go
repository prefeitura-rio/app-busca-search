package v3

// SearchResponse representa a resposta de busca v3
type SearchResponse struct {
	Results    []Document  `json:"results"`
	Pagination Pagination  `json:"pagination"`
	Query      QueryMeta   `json:"query"`
	Timing     TimingMeta  `json:"timing"`
	AIAnalysis *AIAnalysis `json:"ai_analysis,omitempty"`
}

// AIAnalysis representa a análise da query pelo LLM (apenas para busca AI)
type AIAnalysis struct {
	Intent         string   `json:"intent"`
	Keywords       []string `json:"keywords"`
	Categories     []string `json:"categories"`
	RefinedQueries []string `json:"refined_queries"`
	SearchStrategy string   `json:"search_strategy"`
	Confidence     float64  `json:"confidence"`
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
	Final      float64 `json:"final"`
	Text       float64 `json:"text,omitempty"`
	Vector     float64 `json:"vector,omitempty"`
	Recency    float64 `json:"recency,omitempty"`
	Popularity float64 `json:"popularity,omitempty"`
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

// Button representa um botão de ação do serviço
type Button struct {
	Titulo     string `json:"titulo"`
	Descricao  string `json:"descricao,omitempty"`
	URLService string `json:"url_service"`
}

// AgentDocument representa um documento compacto para chatbots/agents
type AgentDocument struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category,omitempty"`
	Slug        string   `json:"slug,omitempty"`
	Score       float64  `json:"score"`
	Buttons     []Button `json:"buttons,omitempty"`
}

// AgentSearchResponse representa uma resposta compacta para agents
type AgentSearchResponse struct {
	Results    []AgentDocument `json:"results"`
	Total      int             `json:"total"`
	Query      string          `json:"query"`
	AIAnalysis *AIAnalysis     `json:"ai_analysis,omitempty"`
}

// ToAgentDocument converte Document para AgentDocument
func (d *Document) ToAgentDocument() AgentDocument {
	agent := AgentDocument{
		ID:          d.ID,
		Title:       d.Title,
		Description: d.Description,
		Category:    d.Category,
		Slug:        d.Slug,
		Score:       d.Score.Final,
	}

	// Extrai botões do Data se disponível
	if buttonsRaw, ok := d.Data["buttons"]; ok {
		if buttonsList, ok := buttonsRaw.([]interface{}); ok {
			for _, b := range buttonsList {
				if btnMap, ok := b.(map[string]interface{}); ok {
					isEnabled := true
					if enabled, ok := btnMap["is_enabled"].(bool); ok {
						isEnabled = enabled
					}
					if isEnabled {
						btn := Button{}
						if titulo, ok := btnMap["titulo"].(string); ok {
							btn.Titulo = titulo
						}
						if url, ok := btnMap["url_service"].(string); ok {
							btn.URLService = url
						}
						if btn.Titulo != "" && btn.URLService != "" {
							agent.Buttons = append(agent.Buttons, btn)
						}
					}
				}
			}
		}
	}

	return agent
}

// ToAgentResponse converte SearchResponse para AgentSearchResponse
func (r *SearchResponse) ToAgentResponse() *AgentSearchResponse {
	agentDocs := make([]AgentDocument, 0, len(r.Results))
	for _, doc := range r.Results {
		agentDocs = append(agentDocs, doc.ToAgentDocument())
	}

	return &AgentSearchResponse{
		Results:    agentDocs,
		Total:      r.Pagination.Total,
		Query:      r.Query.Original,
		AIAnalysis: r.AIAnalysis,
	}
}

// FilteredDocument representa um documento com campos filtrados
type FilteredDocument struct {
	ID          string                 `json:"id,omitempty"`
	Collection  string                 `json:"collection,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Slug        string                 `json:"slug,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Score       *ScoreInfo             `json:"score,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Buttons     []Button               `json:"buttons,omitempty"`
}

// FilteredSearchResponse representa uma resposta com campos filtrados
type FilteredSearchResponse struct {
	Results    []FilteredDocument `json:"results"`
	Pagination Pagination         `json:"pagination"`
	Query      QueryMeta          `json:"query"`
	Timing     TimingMeta         `json:"timing"`
	AIAnalysis *AIAnalysis        `json:"ai_analysis,omitempty"`
}

// ToFilteredResponse converte SearchResponse para resposta filtrada
func (r *SearchResponse) ToFilteredResponse(fields []string) *FilteredSearchResponse {
	filtered := &FilteredSearchResponse{
		Results:    make([]FilteredDocument, 0, len(r.Results)),
		Pagination: r.Pagination,
		Query:      r.Query,
		Timing:     r.Timing,
		AIAnalysis: r.AIAnalysis,
	}

	fieldMap := make(map[string]bool)
	for _, f := range fields {
		fieldMap[f] = true
	}

	for _, doc := range r.Results {
		fd := FilteredDocument{}

		// Campos sempre incluídos
		fd.ID = doc.ID

		if fieldMap["collection"] {
			fd.Collection = doc.Collection
		}
		if fieldMap["type"] {
			fd.Type = doc.Type
		}
		if fieldMap["title"] {
			fd.Title = doc.Title
		}
		if fieldMap["description"] {
			fd.Description = doc.Description
		}
		if fieldMap["category"] {
			fd.Category = doc.Category
		}
		if fieldMap["slug"] {
			fd.Slug = doc.Slug
		}
		if fieldMap["url"] {
			fd.URL = doc.URL
		}
		if fieldMap["score"] {
			fd.Score = &doc.Score
		}
		if fieldMap["data"] {
			fd.Data = doc.Data
		}
		if fieldMap["buttons"] {
			// Extrai botões do Data
			if buttonsRaw, ok := doc.Data["buttons"]; ok {
				if buttonsList, ok := buttonsRaw.([]interface{}); ok {
					for _, b := range buttonsList {
						if btnMap, ok := b.(map[string]interface{}); ok {
							isEnabled := true
							if enabled, ok := btnMap["is_enabled"].(bool); ok {
								isEnabled = enabled
							}
							if isEnabled {
								btn := Button{}
								if titulo, ok := btnMap["titulo"].(string); ok {
									btn.Titulo = titulo
								}
								if url, ok := btnMap["url_service"].(string); ok {
									btn.URLService = url
								}
								if btn.Titulo != "" && btn.URLService != "" {
									fd.Buttons = append(fd.Buttons, btn)
								}
							}
						}
					}
				}
			}
		}

		filtered.Results = append(filtered.Results, fd)
	}

	return filtered
}
