package v3

// SearchType define os tipos de busca disponíveis
// @Description Algoritmo de busca a ser utilizado.
// @Description - keyword: Busca textual BM25, ideal para termos exatos
// @Description - semantic: Busca vetorial com embeddings, ideal para linguagem natural
// @Description - hybrid: Combinação de keyword + semantic (recomendado)
// @Description - ai: Busca com análise LLM e reranking inteligente
type SearchType string

const (
	// SearchTypeKeyword - Busca textual BM25 pura
	SearchTypeKeyword SearchType = "keyword"
	// SearchTypeSemantic - Busca vetorial usando embeddings Gemini
	SearchTypeSemantic SearchType = "semantic"
	// SearchTypeHybrid - Combina BM25 + vetorial com peso configurável
	SearchTypeHybrid SearchType = "hybrid"
	// SearchTypeAI - Busca com análise LLM, expansão e reranking
	SearchTypeAI SearchType = "ai"
)

// SearchMode define o modo de resposta (afeta formato e configurações padrão)
// @Description Modo de resposta que determina o formato e configurações padrão.
// @Description - human: Resposta completa com todos os metadados
// @Description - agent: Resposta compacta otimizada para chatbots
type SearchMode string

const (
	// SearchModeHuman - Resposta completa para interfaces humanas
	SearchModeHuman SearchMode = "human"
	// SearchModeAgent - Resposta compacta para chatbots e agentes IA
	SearchModeAgent SearchMode = "agent"
)

// IsValid verifica se o tipo de busca é válido
func (t SearchType) IsValid() bool {
	switch t {
	case SearchTypeKeyword, SearchTypeSemantic, SearchTypeHybrid, SearchTypeAI:
		return true
	}
	return false
}

// IsValid verifica se o modo é válido
func (m SearchMode) IsValid() bool {
	switch m {
	case SearchModeHuman, SearchModeAgent:
		return true
	}
	return false
}
