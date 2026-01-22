package v3

import "strings"

// SearchRequest representa uma requisição de busca v3
type SearchRequest struct {
	// Obrigatórios
	Query string     `form:"q" binding:"required"`
	Type  SearchType `form:"type" binding:"required"`

	// Paginação
	Page    int `form:"page"`
	PerPage int `form:"per_page"`

	// Collections
	Collections string `form:"collections"` // comma-separated

	// Modo (human ou agent)
	Mode SearchMode `form:"mode"`

	// Configuração híbrida
	Alpha float64 `form:"alpha"` // peso texto vs vetor (0-1)

	// Thresholds
	Threshold float64 `form:"threshold"` // score mínimo

	// Features
	Expand  *bool `form:"expand"`  // expandir query
	Recency *bool `form:"recency"` // boost por recência
	Typos   *int  `form:"typos"`   // tolerância a typos (0-2)

	// Filtros
	Status      *int   `form:"status"`       // filtrar por status
	Category    string `form:"category"`     // filtrar por categoria
	SubCategory string `form:"sub_category"` // filtrar por subcategoria
	OrgaoGestor string `form:"orgao"`        // filtrar por órgão gestor
	TempoMax    string `form:"tempo_max"`    // filtrar por tempo máximo (imediato, 1_dia, etc)
	IsFree      *bool  `form:"is_free"`      // filtrar por gratuidade
	HasDigital  *bool  `form:"digital"`      // filtrar por canal digital disponível

	// Campos a retornar (comma-separated)
	Fields string `form:"fields"`

	// Interno (preenchido pelo handler)
	ParsedCollections []string `form:"-" json:"-"`
	ParsedFields      []string `form:"-" json:"-"`
}

// Validate valida e aplica defaults à requisição
func (r *SearchRequest) Validate() error {
	if r.Query == "" {
		return ErrQueryRequired
	}

	if !r.Type.IsValid() {
		return ErrInvalidSearchType
	}

	// Defaults de paginação
	if r.Page < 1 {
		r.Page = 1
	}
	if r.PerPage < 1 {
		r.PerPage = 10
	}
	if r.PerPage > 100 {
		r.PerPage = 100
	}

	// Default de modo
	if r.Mode == "" {
		r.Mode = SearchModeHuman
	}
	if !r.Mode.IsValid() {
		r.Mode = SearchModeHuman
	}

	// Default de alpha
	if r.Alpha <= 0 || r.Alpha > 1 {
		r.Alpha = 0.3
	}

	return nil
}

// GetExpand retorna se query expansion está habilitado
func (r *SearchRequest) GetExpand() bool {
	if r.Expand != nil {
		return *r.Expand
	}
	return r.Mode == SearchModeHuman
}

// GetRecency retorna se recency boost está habilitado
func (r *SearchRequest) GetRecency() bool {
	if r.Recency != nil {
		return *r.Recency
	}
	return r.Mode == SearchModeHuman
}

// GetTypos retorna tolerância a typos
func (r *SearchRequest) GetTypos() int {
	if r.Typos != nil {
		if *r.Typos < 0 {
			return 0
		}
		if *r.Typos > 2 {
			return 2
		}
		return *r.Typos
	}
	if r.Mode == SearchModeAgent {
		return 1
	}
	return 2
}

// ParseFields parsea o parâmetro fields em uma lista
func (r *SearchRequest) ParseFields() {
	if r.Fields == "" {
		r.ParsedFields = nil
		return
	}
	parts := strings.Split(r.Fields, ",")
	r.ParsedFields = make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			r.ParsedFields = append(r.ParsedFields, p)
		}
	}
}

// HasField verifica se um campo específico foi solicitado
func (r *SearchRequest) HasField(field string) bool {
	if len(r.ParsedFields) == 0 {
		return true // Se não especificou, retorna todos
	}
	for _, f := range r.ParsedFields {
		if f == field {
			return true
		}
	}
	return false
}
