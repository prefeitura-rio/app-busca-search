package services

// PopularityService fornece scores de popularidade para categorias
// TODO: Substituir mapa hardcoded por integração com Google Analytics
type PopularityService struct {
	categoryScores map[string]int
}

// NewPopularityService cria um novo serviço de popularidade
func NewPopularityService() *PopularityService {
	return &PopularityService{
		categoryScores: getHardcodedCategoryScores(),
	}
}

// GetCategoryPopularity retorna o score de popularidade de uma categoria
// Retorna 0 se a categoria não tiver score definido
func (ps *PopularityService) GetCategoryPopularity(category string) int {
	if score, ok := ps.categoryScores[category]; ok {
		return score
	}
	return 0
}

// GetAllCategories retorna todas as categorias conhecidas com seus scores
func (ps *PopularityService) GetAllCategories() map[string]int {
	return ps.categoryScores
}

// getHardcodedCategoryScores retorna scores temporários de popularidade
// TODO: Substituir por dados do Google Analytics quando disponível
func getHardcodedCategoryScores() map[string]int {
	return map[string]int{
		"Cidade":                                5000,
		"Transporte":                            4500,
		"Saúde":                                 4000,
		"Educação":                              3500,
		"Tributos":                              3000,
		"Cidadania":                             2800,
		"Licenças":                              2600,
		"Meio Ambiente":                         2400,
		"Trânsito":                              2200,
		"Servidor":                              2000,
		"Segurança":                             1800,
		"Defesa Civil":                          1600,
		"Trabalho":                              1400,
		"Cultura":                               1200,
		"Cursos":                                1000,
		"Ordem Pública":                         900,
		"Obras":                                 800,
		"Animais":                               700,
		"Ouvidoria":                             600,
		"Peticionamentos":                       500,
		"Lei de Acesso à Informação (LAI)":      400,
		"Lei Geral de Proteção de Dados (LGPD)": 300,
		"Central Anticorrupção":                 200,
	}
}
