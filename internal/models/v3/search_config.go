package v3

// SearchConfig contém todas as configurações de busca
type SearchConfig struct {
	// Busca textual
	QueryBy       []string // campos para busca
	QueryWeights  []int    // pesos dos campos
	NumTypos      int      // tolerância a erros (0-2)
	TypoMinLength int      // tamanho mínimo para tolerar typos

	// Busca vetorial
	EmbeddingField string // campo do embedding
	VectorK        int    // quantos vizinhos buscar

	// Híbrida
	Alpha float64 // peso texto vs vetor (0-1)

	// Ranking
	RecencyBoost           bool    // boost por recência
	RecencyDecay           float64 // taxa de decaimento
	RecencyGracePeriodDays int     // dias sem penalidade

	// Title Match Boost
	TitleExactMatchBoost   float64 // boost quando query = título (default 1.3)
	TitlePartialMatchBoost float64 // boost quando query está no título (default 1.15)

	// Query Expansion
	EnableExpansion bool // expandir query
	MaxExpansions   int  // máximo de termos expandidos

	// Thresholds
	MinScore float64 // score mínimo

	// Collections
	Collections map[string]*CollectionConfig
}

// SearchConfigDefaults armazena valores padrão carregados do ambiente
type SearchConfigDefaults struct {
	Alpha                  float64
	TyposHuman             int
	TyposAgent             int
	EnableQueryExpansion   bool
	MaxQueryExpansionTerms int
	EnableRecencyBoost     bool
	RecencyGracePeriodDays int
	RecencyDecayRate       float64
}

// defaults armazena os valores configurados via ambiente
var defaults = SearchConfigDefaults{
	Alpha:                  0.5, // Balanceado: 50% texto, 50% vetor
	TyposHuman:             2,
	TyposAgent:             1,
	EnableQueryExpansion:   true,
	MaxQueryExpansionTerms: 5,
	EnableRecencyBoost:     true,
	RecencyGracePeriodDays: 30,
	RecencyDecayRate:       0.00207,
}

// SetDefaults configura os valores padrão (chamado durante inicialização)
func SetDefaults(d SearchConfigDefaults) {
	defaults = d
}

// GetDefaults retorna os valores padrão configurados
func GetDefaults() SearchConfigDefaults {
	return defaults
}

// DefaultHumanConfig retorna configuração otimizada para humanos
func DefaultHumanConfig() *SearchConfig {
	return &SearchConfig{
		QueryBy:                []string{"nome_servico", "resumo", "descricao_completa", "search_content"},
		QueryWeights:           []int{5, 4, 3, 2},
		NumTypos:               defaults.TyposHuman,
		TypoMinLength:          4,
		EmbeddingField:         "embedding",
		VectorK:                100,
		Alpha:                  defaults.Alpha,
		RecencyBoost:           defaults.EnableRecencyBoost,
		RecencyDecay:           defaults.RecencyDecayRate,
		RecencyGracePeriodDays: defaults.RecencyGracePeriodDays,
		TitleExactMatchBoost:   1.3,
		TitlePartialMatchBoost: 1.15,
		EnableExpansion:        defaults.EnableQueryExpansion,
		MaxExpansions:          defaults.MaxQueryExpansionTerms,
		MinScore:               0.0,
		Collections:            make(map[string]*CollectionConfig),
	}
}

// DefaultAgentConfig retorna configuração otimizada para agentes
func DefaultAgentConfig() *SearchConfig {
	return &SearchConfig{
		QueryBy:                []string{"nome_servico", "resumo", "descricao_completa", "search_content"},
		QueryWeights:           []int{5, 4, 3, 2},
		NumTypos:               defaults.TyposAgent,
		TypoMinLength:          4,
		EmbeddingField:         "embedding",
		VectorK:                100,
		Alpha:                  0.5,
		RecencyBoost:           false,
		RecencyDecay:           0.0,
		RecencyGracePeriodDays: defaults.RecencyGracePeriodDays,
		TitleExactMatchBoost:   1.3,
		TitlePartialMatchBoost: 1.15,
		EnableExpansion:        false,
		MaxExpansions:          0,
		MinScore:               0.0,
		Collections:            make(map[string]*CollectionConfig),
	}
}

// ConfigForMode retorna configuração baseada no modo
func ConfigForMode(mode SearchMode) *SearchConfig {
	if mode == SearchModeAgent {
		return DefaultAgentConfig()
	}
	return DefaultHumanConfig()
}

// ApplyRequest aplica configurações do request
func (c *SearchConfig) ApplyRequest(req *SearchRequest) {
	if req.Alpha > 0 && req.Alpha <= 1 {
		c.Alpha = req.Alpha
	}

	if req.Threshold > 0 {
		c.MinScore = req.Threshold
	}

	c.EnableExpansion = req.GetExpand()
	c.RecencyBoost = req.GetRecency()
	c.NumTypos = req.GetTypos()
}
