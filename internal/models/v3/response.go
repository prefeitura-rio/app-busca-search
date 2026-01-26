package v3

// SearchResponse representa a resposta de busca v3 (modo human)
// @Description Resposta completa da busca unificada v3. Retornada quando mode=human (padrão).
type SearchResponse struct {
	// Lista de documentos encontrados ordenados por relevância
	Results []Document `json:"results"`
	// Informações de paginação
	Pagination Pagination `json:"pagination"`
	// Metadados sobre a query processada
	Query QueryMeta `json:"query"`
	// Métricas de tempo de execução
	Timing TimingMeta `json:"timing"`
	// Análise da query pelo LLM (apenas para type=ai)
	AIAnalysis *AIAnalysis `json:"ai_analysis,omitempty"`
}

// AIAnalysis representa a análise da query pelo LLM (apenas para busca AI)
// @Description Análise inteligente da query usando LLM. Presente apenas quando type=ai.
type AIAnalysis struct {
	// Intenção identificada do usuário (ex: "buscar_servico", "informacao_geral")
	Intent string `json:"intent" example:"buscar_documento"`
	// Palavras-chave extraídas da query
	Keywords []string `json:"keywords" example:"certidao,nascimento"`
	// Categorias sugeridas baseadas na query
	Categories []string `json:"categories" example:"documentos,cidadania"`
	// Queries refinadas geradas pelo LLM para melhorar a busca
	RefinedQueries []string `json:"refined_queries" example:"certidao de nascimento,registro civil"`
	// Estratégia de busca escolhida (hybrid, semantic, keyword)
	SearchStrategy string `json:"search_strategy" example:"hybrid"`
	// Nível de confiança da análise (0.0 a 1.0)
	Confidence float64 `json:"confidence" example:"0.85"`
}

// Document representa um documento retornado pela busca
// @Description Documento completo com todos os campos e scores detalhados.
type Document struct {
	// Identificador único do documento
	ID string `json:"id" example:"srv_12345"`
	// Nome da collection de origem
	Collection string `json:"collection" example:"prefrio_services_base"`
	// Tipo do documento (service, info, etc)
	Type string `json:"type" example:"service"`
	// Título do serviço/documento
	Title string `json:"title" example:"Certidão de Nascimento"`
	// Descrição resumida do serviço
	Description string `json:"description" example:"Solicite sua certidão de nascimento online"`
	// Categoria principal do serviço
	Category string `json:"category,omitempty" example:"Documentos"`
	// Slug para URL amigável
	Slug string `json:"slug,omitempty" example:"certidao-nascimento"`
	// URL de acesso ao serviço
	URL string `json:"url,omitempty" example:"https://carioca.rio/servicos/certidao-nascimento"`
	// Informações detalhadas de score
	Score ScoreInfo `json:"score"`
	// Dados originais do documento (campos específicos da collection)
	Data map[string]interface{} `json:"data"`
}

// ScoreInfo contém informações detalhadas de score do documento
// @Description Scores normalizados (0.0 a 1.0) de cada componente da busca.
type ScoreInfo struct {
	// Score final combinado usado para ordenação (0.0 a 1.0)
	Final float64 `json:"final" example:"0.87"`
	// Score de busca textual BM25 normalizado (0.0 a 1.0)
	Text float64 `json:"text,omitempty" example:"0.72"`
	// Score de similaridade vetorial normalizado (0.0 a 1.0, onde 1.0 = mais similar)
	Vector float64 `json:"vector,omitempty" example:"0.91"`
	// Score híbrido combinando texto e vetor: (alpha * text) + ((1-alpha) * vector)
	Hybrid float64 `json:"hybrid,omitempty" example:"0.85"`
	// Boost de recência baseado na data de atualização (1.0 = recente, decai com o tempo)
	Recency float64 `json:"recency,omitempty" example:"0.95"`
	// Score de popularidade do serviço (0.0 a 1.0)
	Popularity float64 `json:"popularity,omitempty" example:"0.60"`
}

// Pagination contém informações de paginação
// @Description Metadados de paginação para navegação nos resultados.
type Pagination struct {
	// Página atual (começa em 1)
	Page int `json:"page" example:"1"`
	// Número de resultados por página
	PerPage int `json:"per_page" example:"10"`
	// Total de documentos encontrados
	Total int `json:"total" example:"42"`
	// Total de páginas disponíveis
	TotalPages int `json:"total_pages" example:"5"`
}

// QueryMeta contém metadados sobre a query processada
// @Description Informações sobre como a query foi processada internamente.
type QueryMeta struct {
	// Query original enviada pelo usuário
	Original string `json:"original" example:"certidao nascimento"`
	// Query normalizada (lowercase, sem acentos)
	Normalized string `json:"normalized,omitempty" example:"certidao nascimento"`
	// Termos expandidos via sinônimos (quando expand=true)
	Expanded []string `json:"expanded,omitempty" example:"certidao,nascimento,registro,civil"`
	// Intenção detectada (apenas para type=ai)
	Intent string `json:"intent,omitempty" example:"buscar_documento"`
}

// TimingMeta contém métricas de tempo de execução
// @Description Métricas de performance em milissegundos para debugging e monitoramento.
type TimingMeta struct {
	// Tempo total de processamento em ms
	TotalMs float64 `json:"total_ms" example:"127.5"`
	// Tempo de parsing e validação da query em ms
	ParsingMs float64 `json:"parsing_ms,omitempty" example:"2.3"`
	// Tempo de geração do embedding via Gemini em ms
	EmbeddingMs float64 `json:"embedding_ms,omitempty" example:"45.2"`
	// Tempo de busca no Typesense em ms
	SearchMs float64 `json:"search_ms" example:"68.1"`
	// Tempo de ranking e ordenação em ms
	RankingMs float64 `json:"ranking_ms,omitempty" example:"11.9"`
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
// @Description Botão de ação com link para acessar o serviço.
type Button struct {
	// Título do botão (ex: "Acessar serviço", "Agendar atendimento")
	Titulo string `json:"titulo" example:"Acessar serviço"`
	// Descrição opcional do botão
	Descricao string `json:"descricao,omitempty" example:"Clique para iniciar a solicitação"`
	// URL de destino do botão
	URLService string `json:"url_service" example:"https://carioca.rio/servicos/certidao-nascimento/solicitar"`
}

// AgentDocument representa um documento compacto para chatbots/agents
// @Description Documento simplificado otimizado para integração com chatbots e agentes IA. Retornado quando mode=agent.
type AgentDocument struct {
	// Identificador único do documento
	ID string `json:"id" example:"srv_12345"`
	// Título do serviço
	Title string `json:"title" example:"Certidão de Nascimento"`
	// Descrição resumida
	Description string `json:"description" example:"Solicite sua certidão de nascimento online"`
	// Categoria do serviço
	Category string `json:"category,omitempty" example:"Documentos"`
	// Slug para identificação
	Slug string `json:"slug,omitempty" example:"certidao-nascimento"`
	// Score final de relevância (0.0 a 1.0)
	Score float64 `json:"score" example:"0.87"`
	// Botões de ação disponíveis (apenas os habilitados)
	Buttons []Button `json:"buttons,omitempty"`
}

// AgentSearchResponse representa uma resposta compacta para agents
// @Description Resposta simplificada otimizada para chatbots e agentes IA. Retornada quando mode=agent. Contém menos metadados e documentos compactos.
type AgentSearchResponse struct {
	// Lista de documentos compactos
	Results []AgentDocument `json:"results"`
	// Total de documentos encontrados
	Total int `json:"total" example:"42"`
	// Query original
	Query string `json:"query" example:"certidao nascimento"`
	// Análise da query pelo LLM (apenas para type=ai)
	AIAnalysis *AIAnalysis `json:"ai_analysis,omitempty"`
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
// @Description Documento com apenas os campos solicitados via parâmetro 'fields'. Campos não solicitados são omitidos.
type FilteredDocument struct {
	// Identificador único (sempre incluído)
	ID string `json:"id,omitempty" example:"srv_12345"`
	// Nome da collection (se solicitado via fields=collection)
	Collection string `json:"collection,omitempty" example:"prefrio_services_base"`
	// Tipo do documento (se solicitado via fields=type)
	Type string `json:"type,omitempty" example:"service"`
	// Título do serviço (se solicitado via fields=title)
	Title string `json:"title,omitempty" example:"Certidão de Nascimento"`
	// Descrição resumida (se solicitado via fields=description)
	Description string `json:"description,omitempty" example:"Solicite sua certidão de nascimento online"`
	// Categoria do serviço (se solicitado via fields=category)
	Category string `json:"category,omitempty" example:"Documentos"`
	// Slug para URL amigável (se solicitado via fields=slug)
	Slug string `json:"slug,omitempty" example:"certidao-nascimento"`
	// URL de acesso (se solicitado via fields=url)
	URL string `json:"url,omitempty" example:"https://carioca.rio/servicos/certidao-nascimento"`
	// Informações de score (se solicitado via fields=score)
	Score *ScoreInfo `json:"score,omitempty"`
	// Dados originais completos (se solicitado via fields=data)
	Data map[string]interface{} `json:"data,omitempty"`
	// Botões de ação (se solicitado via fields=buttons)
	Buttons []Button `json:"buttons,omitempty"`
}

// FilteredSearchResponse representa uma resposta com campos filtrados
// @Description Resposta com documentos contendo apenas os campos solicitados via parâmetro 'fields'. Use para reduzir payload.
type FilteredSearchResponse struct {
	// Lista de documentos filtrados
	Results []FilteredDocument `json:"results"`
	// Informações de paginação
	Pagination Pagination `json:"pagination"`
	// Metadados sobre a query processada
	Query QueryMeta `json:"query"`
	// Métricas de tempo de execução
	Timing TimingMeta `json:"timing"`
	// Análise da query pelo LLM (apenas para type=ai)
	AIAnalysis *AIAnalysis `json:"ai_analysis,omitempty"`
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
