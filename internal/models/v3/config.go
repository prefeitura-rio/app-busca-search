package v3

// CollectionConfig define a configuração de uma collection para busca
type CollectionConfig struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"` // service, course, job, etc
	TitleField     string   `json:"title_field"`
	DescField      string   `json:"desc_field"`
	CategoryField  string   `json:"category_field,omitempty"`
	SlugField      string   `json:"slug_field,omitempty"`
	StatusField    string   `json:"status_field,omitempty"`
	StatusValue    string   `json:"status_value,omitempty"`
	EmbeddingField string   `json:"embedding_field"`
	SearchFields   []string `json:"search_fields"`
	SearchWeights  []int    `json:"search_weights"`
}

// GetSearchFieldsString retorna campos como string comma-separated
func (c *CollectionConfig) GetSearchFieldsString() string {
	if len(c.SearchFields) == 0 {
		return c.TitleField + "," + c.DescField
	}
	result := ""
	for i, f := range c.SearchFields {
		if i > 0 {
			result += ","
		}
		result += f
	}
	return result
}

// GetSearchWeightsString retorna pesos como string comma-separated
func (c *CollectionConfig) GetSearchWeightsString() string {
	if len(c.SearchWeights) == 0 {
		return "3,1"
	}
	result := ""
	for i, w := range c.SearchWeights {
		if i > 0 {
			result += ","
		}
		result += string(rune('0' + w))
	}
	return result
}

// DefaultPrefRioConfig retorna configuração padrão para prefrio_services_base
func DefaultPrefRioConfig() *CollectionConfig {
	return &CollectionConfig{
		Name:           "prefrio_services_base",
		Type:           "service",
		TitleField:     "nome_servico",
		DescField:      "resumo",
		CategoryField:  "tema_geral",
		SlugField:      "slug",
		StatusField:    "status",
		StatusValue:    "1",
		EmbeddingField: "embedding",
		SearchFields:   []string{"nome_servico", "resumo", "descricao_completa", "search_content", "documentos_necessarios"},
		SearchWeights:  []int{5, 4, 3, 2, 1},
	}
}
