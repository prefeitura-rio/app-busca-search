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

// getHardcodedCategoryScores retorna scores temporários de popularidade
// TODO: Substituir por dados do Google Analytics quando disponível
func getHardcodedCategoryScores() map[string]int {
	return map[string]int{
		"Cidade":     5000,
		"Transporte": 4500,
		"Saúde":      3500,
		"Educação":   3000,
		"Ambiente":   2800,
		"Taxas":      2500,
		"Cidadania":  2000,
		"Emergência": 1800,
		"Servidor":   1500,
		"Segurança":  1200,
		"Família":    1000,
		"Cultura":    900,
		"Esportes":   800,
		"Animais":    700,
		"Astronomia": 600,
	}
}
