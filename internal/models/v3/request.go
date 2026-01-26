package v3

import "strings"

// SearchRequest representa uma requisição de busca v3
// @Description Parâmetros de requisição para busca unificada v3.
type SearchRequest struct {
	// Query de busca (obrigatório). Termos a serem pesquisados.
	Query string `form:"q" binding:"required" example:"certidão de nascimento"`
	// Tipo de busca: keyword, semantic, hybrid, ai (obrigatório)
	Type SearchType `form:"type" binding:"required" example:"hybrid" enums:"keyword,semantic,hybrid,ai"`

	// Página de resultados (default: 1, mínimo: 1)
	Page int `form:"page" example:"1" minimum:"1"`
	// Resultados por página (default: 10, máximo: 100)
	PerPage int `form:"per_page" example:"10" minimum:"1" maximum:"100"`

	// Collections para buscar (comma-separated). Se vazio, busca em todas.
	Collections string `form:"collections" example:"prefrio_services_base,1746"`

	// Modo de resposta: human (completo) ou agent (compacto para chatbots)
	Mode SearchMode `form:"mode" example:"human" enums:"human,agent"`

	// Peso do score textual (0-1). Alpha=1 é 100% texto, Alpha=0 é 100% vetor. Default: 0.5
	Alpha float64 `form:"alpha" example:"0.5" minimum:"0" maximum:"1"`

	// Score mínimo para incluir resultado (0-1). Resultados abaixo são filtrados.
	Threshold float64 `form:"threshold" example:"0.3" minimum:"0" maximum:"1"`

	// Expandir query com sinônimos. Default: true (human), false (agent)
	Expand *bool `form:"expand" example:"true"`
	// Aplicar boost de recência. Default: true (human), false (agent)
	Recency *bool `form:"recency" example:"true"`
	// Tolerância a typos: 0=exato, 1=pouco, 2=muito. Default: 2 (human), 1 (agent)
	Typos *int `form:"typos" example:"2" minimum:"0" maximum:"2"`

	// Filtrar por status: 0=Rascunho, 1=Publicado
	Status *int `form:"status" example:"1" enums:"0,1"`
	// Filtrar por categoria (tema_geral)
	Category string `form:"category" example:"documentos"`
	// Filtrar por subcategoria
	SubCategory string `form:"sub_category" example:"certidoes"`
	// Filtrar por órgão gestor
	OrgaoGestor string `form:"orgao" example:"SMDC"`
	// Filtrar por tempo máximo: imediato, 1_dia, 2_a_5_dias, etc
	TempoMax string `form:"tempo_max" example:"imediato" enums:"imediato,1_dia,2_a_5_dias,6_a_10_dias,11_a_15_dias,16_a_30_dias,mais_de_30_dias"`
	// Filtrar apenas serviços gratuitos
	IsFree *bool `form:"is_free" example:"true"`
	// Filtrar serviços com canal digital disponível
	HasDigital *bool `form:"digital" example:"true"`

	// Campos a retornar (comma-separated). Reduz payload da resposta.
	// Valores: id,collection,type,title,description,category,slug,url,score,data,buttons
	Fields string `form:"fields" example:"title,description,score,buttons"`

	// Interno (preenchido pelo handler, não exposto na API)
	ParsedCollections []string `form:"-" json:"-" swaggerignore:"true"`
	ParsedFields      []string `form:"-" json:"-" swaggerignore:"true"`
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
