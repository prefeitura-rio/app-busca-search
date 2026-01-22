package v3

// SearchType define os tipos de busca disponíveis
type SearchType string

const (
	SearchTypeKeyword  SearchType = "keyword"
	SearchTypeSemantic SearchType = "semantic"
	SearchTypeHybrid   SearchType = "hybrid"
	SearchTypeAI       SearchType = "ai"
)

// SearchMode define o modo de busca (afeta configurações padrão)
type SearchMode string

const (
	SearchModeHuman SearchMode = "human"
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
