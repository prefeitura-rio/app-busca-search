package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
	"github.com/typesense/typesense-go/v3/typesense/api/pointer"
)

// CategoryService fornece funcionalidades de categorias
type CategoryService struct {
	client            *typesense.Client
	popularityService *PopularityService
}

// NewCategoryService cria um novo serviço de categorias
func NewCategoryService(client *typesense.Client, popularityService *PopularityService) *CategoryService {
	return &CategoryService{
		client:            client,
		popularityService: popularityService,
	}
}

// GetCategories retorna categorias com contadores e opcionalmente serviços filtrados
func (cs *CategoryService) GetCategories(ctx context.Context, req *models.CategoryRequest) (*models.CategoryResponse, error) {
	// Validações e defaults
	if req.SortBy == "" {
		req.SortBy = "popularity"
	}
	if req.Order == "" {
		req.Order = "desc"
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PerPage < 1 || req.PerPage > 100 {
		req.PerPage = 10
	}

	// 1. Buscar categorias via facet search
	categories, err := cs.fetchCategoriesWithFacets(ctx, req.IncludeInactive)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar categorias: %w", err)
	}

	// 2. Enriquecer com scores de popularidade
	for _, cat := range categories {
		cat.PopularityScore = cs.popularityService.GetCategoryPopularity(cat.Name)
	}

	// 3. Se include_empty, mesclar com categorias hardcoded que não vieram nos facets
	if req.IncludeEmpty {
		categories = cs.mergeWithKnownCategories(categories)
	} else {
		categories = cs.filterNonEmpty(categories)
	}

	// 4. Ordenar categorias
	cs.sortCategories(categories, req.SortBy, req.Order)

	// 5. Montar resposta base
	response := &models.CategoryResponse{
		Categories:      categories,
		TotalCategories: len(categories),
		Metadata: map[string]interface{}{
			"timestamp":         time.Now().Format(time.RFC3339),
			"popularity_source": "hardcoded",
			"note":              "Aguardando integração Google Analytics",
		},
	}

	// 6. Se filter_category fornecido, buscar serviços
	if req.FilterCategory != "" {
		services, total, err := cs.getServicesByCategory(ctx, req.FilterCategory, req.Page, req.PerPage, req.IncludeInactive)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar serviços da categoria: %w", err)
		}

		response.FilteredCategory = &models.FilteredCategoryResult{
			Name:          req.FilterCategory,
			Services:      services,
			TotalServices: total,
			Page:          req.Page,
			PerPage:       req.PerPage,
		}
	}

	return response, nil
}

// fetchCategoriesWithFacets busca categorias usando facet search do Typesense
func (cs *CategoryService) fetchCategoriesWithFacets(ctx context.Context, includeInactive bool) ([]*models.Category, error) {
	// Construir filtro dinamicamente baseado em includeInactive
	var filterBy string
	if includeInactive {
		// Sem filtro de status - retorna todas as categorias (publicados e rascunhos)
		filterBy = ""
	} else {
		// Apenas publicados (status = 1)
		filterBy = "status:=1"
	}

	// Query com facet em tema_geral
	searchParams := &api.SearchCollectionParams{
		Q:              pointer.String("*"),
		FacetBy:        pointer.String("tema_geral"),
		MaxFacetValues: pointer.Int(250),
		PerPage:        pointer.Int(0),
	}

	// Adicionar filtro apenas se não estiver vazio
	if filterBy != "" {
		searchParams.FilterBy = pointer.String(filterBy)
	}

	result, err := cs.client.Collection(CollectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, err
	}

	// Extrair categorias dos facets
	categories, err := cs.extractCategoriesFromFacets(result)
	if err != nil {
		return nil, err
	}

	return categories, nil
}

// extractCategoriesFromFacets extrai categorias dos resultados de facet search
func (cs *CategoryService) extractCategoriesFromFacets(result *api.SearchResult) ([]*models.Category, error) {
	categories := []*models.Category{}

	// Converter result para map para acessar facet_counts
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %w", err)
	}

	var resultMap map[string]interface{}
	if err := json.Unmarshal(resultBytes, &resultMap); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %w", err)
	}

	// Navegar até facet_counts
	facetCounts, ok := resultMap["facet_counts"].([]interface{})
	if !ok {
		return categories, nil // Sem facets, retorna vazio
	}

	// Processar cada facet
	for _, facetInterface := range facetCounts {
		facet, ok := facetInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Verificar se é o facet de tema_geral
		fieldName, _ := facet["field_name"].(string)
		if fieldName != "tema_geral" {
			continue
		}

		// Processar counts
		counts, ok := facet["counts"].([]interface{})
		if !ok {
			continue
		}

		for _, countInterface := range counts {
			countMap, ok := countInterface.(map[string]interface{})
			if !ok {
				continue
			}

			name, _ := countMap["value"].(string)
			count := 0
			if countFloat, ok := countMap["count"].(float64); ok {
				count = int(countFloat)
			}

			if name != "" {
				categories = append(categories, &models.Category{
					Name:            name,
					Count:           count,
					PopularityScore: 0, // Será preenchido depois
				})
			}
		}
	}

	return categories, nil
}

// getServicesByCategory busca serviços de uma categoria específica
func (cs *CategoryService) getServicesByCategory(ctx context.Context, category string, page, perPage int, includeInactive bool) ([]*models.ServiceDocument, int, error) {
	// Construir filtro dinamicamente baseado em includeInactive
	// Backticks são necessários para escapar caracteres especiais como parênteses
	var filterBy string
	if includeInactive {
		// Apenas filtrar por categoria, sem filtro de status
		filterBy = fmt.Sprintf("tema_geral:=`%s`", category)
	} else {
		// Filtrar por categoria E status publicado
		filterBy = fmt.Sprintf("tema_geral:=`%s` && status:=1", category)
	}

	searchParams := &api.SearchCollectionParams{
		Q:        pointer.String("*"),
		FilterBy: pointer.String(filterBy),
		Page:     pointer.Int(page),
		PerPage:  pointer.Int(perPage),
		SortBy:   pointer.String("last_update:desc"), // Mais recentes primeiro
	}

	result, err := cs.client.Collection(CollectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, 0, err
	}

	// Transformar hits em ServiceDocuments
	docs := cs.transformHitsToDocuments(result)

	total := 0
	if result.Found != nil {
		total = int(*result.Found)
	}

	return docs, total, nil
}

// transformHitsToDocuments converte hits do Typesense em ServiceDocuments
func (cs *CategoryService) transformHitsToDocuments(result *api.SearchResult) []*models.ServiceDocument {
	docs := []*models.ServiceDocument{}

	if result.Hits == nil {
		return docs
	}

	for _, hit := range *result.Hits {
		if hit.Document == nil {
			continue
		}

		// Converter document para map para fazer mapeamento manual dos campos
		docBytes, err := json.Marshal(*hit.Document)
		if err != nil {
			continue
		}

		var tsDoc map[string]interface{}
		if err := json.Unmarshal(docBytes, &tsDoc); err != nil {
			continue
		}

		// Transformar documento com mapeamento correto dos campos
		doc := cs.transformDocument(tsDoc)
		docs = append(docs, doc)
	}

	return docs
}

// transformDocument transforma um documento Typesense em ServiceDocument
func (cs *CategoryService) transformDocument(tsDoc map[string]interface{}) *models.ServiceDocument {
	// Extrair campos principais com mapeamento correto
	id := getString(tsDoc, "id")
	title := getString(tsDoc, "nome_servico")
	description := getString(tsDoc, "resumo")
	category := getString(tsDoc, "tema_geral")
	subcategory := getStringPtr(tsDoc, "sub_categoria")
	status := getInt32(tsDoc, "status")
	createdAt := getInt64(tsDoc, "created_at")
	updatedAt := getInt64(tsDoc, "last_update")

	// Todos os outros campos vão para metadata
	metadata := make(map[string]interface{})
	excludeFields := map[string]bool{
		"id": true, "nome_servico": true, "resumo": true,
		"tema_geral": true, "sub_categoria": true, "status": true, "created_at": true,
		"last_update": true, "embedding": true, // não retornar embedding
		"search_content": true, // não retornar search_content
	}

	for key, value := range tsDoc {
		if !excludeFields[key] {
			metadata[key] = value
		}
	}

	return &models.ServiceDocument{
		ID:          id,
		Title:       title,
		Description: description,
		Category:    category,
		Subcategory: subcategory,
		Status:      status,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Metadata:    metadata,
	}
}

// filterNonEmpty remove categorias sem serviços
func (cs *CategoryService) filterNonEmpty(categories []*models.Category) []*models.Category {
	filtered := []*models.Category{}
	for _, cat := range categories {
		if cat.Count > 0 {
			filtered = append(filtered, cat)
		}
	}
	return filtered
}

// mergeWithKnownCategories adiciona categorias da lista de popularidade que não vieram nos facets
func (cs *CategoryService) mergeWithKnownCategories(categories []*models.Category) []*models.Category {
	existing := make(map[string]bool)
	for _, cat := range categories {
		existing[cat.Name] = true
	}

	for name, score := range cs.popularityService.GetAllCategories() {
		if !existing[name] {
			categories = append(categories, &models.Category{
				Name:            name,
				Count:           0,
				PopularityScore: score,
			})
		}
	}

	return categories
}

// sortCategories ordena categorias conforme critério
func (cs *CategoryService) sortCategories(categories []*models.Category, sortBy, order string) {
	sort.Slice(categories, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "alpha":
			less = categories[i].Name < categories[j].Name
		case "count":
			less = categories[i].Count < categories[j].Count
		default: // "popularity"
			less = categories[i].PopularityScore < categories[j].PopularityScore
		}

		// Inverter se ordem descendente
		if order == "desc" {
			return !less
		}
		return less
	})
}
