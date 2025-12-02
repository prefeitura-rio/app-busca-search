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

// SubcategoryService fornece funcionalidades de subcategorias
type SubcategoryService struct {
	client            *typesense.Client
	popularityService *PopularityService
}

// NewSubcategoryService cria um novo serviço de subcategorias
func NewSubcategoryService(client *typesense.Client, popularityService *PopularityService) *SubcategoryService {
	return &SubcategoryService{
		client:            client,
		popularityService: popularityService,
	}
}

// GetSubcategories retorna subcategorias de uma categoria específica
func (scs *SubcategoryService) GetSubcategories(ctx context.Context, req *models.SubcategoryRequest) (*models.SubcategoryResponse, error) {
	// Validações e defaults
	if req.SortBy == "" {
		req.SortBy = "popularity"
	}
	if req.Order == "" {
		req.Order = "desc"
	}

	// 1. Buscar subcategorias via facet search filtradas por categoria
	subcategories, err := scs.fetchSubcategoriesWithFacets(ctx, req.Category, req.IncludeInactive)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar subcategorias: %w", err)
	}

	// 2. Enriquecer com scores de popularidade (TODO: implementar no PopularityService)
	for _, subcat := range subcategories {
		// Por enquanto, popularidade = 0 (pode ser implementado depois)
		subcat.PopularityScore = 0
	}

	// 3. Filtrar vazias se include_empty = false
	if !req.IncludeEmpty {
		subcategories = scs.filterNonEmpty(subcategories)
	}

	// 4. Ordenar subcategorias
	scs.sortSubcategories(subcategories, req.SortBy, req.Order)

	// 5. Montar resposta
	response := &models.SubcategoryResponse{
		Subcategories:      subcategories,
		TotalSubcategories: len(subcategories),
		Category:           req.Category,
		Metadata: map[string]interface{}{
			"timestamp":         time.Now().Format(time.RFC3339),
			"popularity_source": "hardcoded",
			"note":              "Aguardando integração Google Analytics",
		},
	}

	return response, nil
}

// GetServicesBySubcategory retorna serviços de uma subcategoria específica
func (scs *SubcategoryService) GetServicesBySubcategory(ctx context.Context, req *models.SubcategoryServicesRequest) (*models.SubcategoryServicesResponse, error) {
	// Validações e defaults
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PerPage < 1 || req.PerPage > 100 {
		req.PerPage = 10
	}

	// Construir filtro dinamicamente baseado em includeInactive
	var filterBy string
	if req.IncludeInactive {
		// Apenas filtrar por subcategoria, sem filtro de status
		filterBy = fmt.Sprintf("sub_categoria:=`%s`", req.Subcategory)
	} else {
		// Filtrar por subcategoria E status publicado
		filterBy = fmt.Sprintf("sub_categoria:=`%s` && status:=1", req.Subcategory)
	}

	searchParams := &api.SearchCollectionParams{
		Q:        pointer.String("*"),
		FilterBy: pointer.String(filterBy),
		Page:     pointer.Int(req.Page),
		PerPage:  pointer.Int(req.PerPage),
		SortBy:   pointer.String("last_update:desc"), // Mais recentes primeiro
	}

	result, err := scs.client.Collection(CollectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar serviços da subcategoria: %w", err)
	}

	// Transformar hits em ServiceDocuments
	docs := scs.transformHitsToDocuments(result)

	total := 0
	if result.Found != nil {
		total = int(*result.Found)
	}

	// Buscar categoria da subcategoria (primeiro serviço)
	category := ""
	if len(docs) > 0 {
		category = docs[0].Category
	}

	response := &models.SubcategoryServicesResponse{
		Subcategory:   req.Subcategory,
		Services:      docs,
		TotalServices: total,
		Page:          req.Page,
		PerPage:       req.PerPage,
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"category":  category,
		},
	}

	return response, nil
}

// fetchSubcategoriesWithFacets busca subcategorias usando facet search do Typesense
func (scs *SubcategoryService) fetchSubcategoriesWithFacets(ctx context.Context, category string, includeInactive bool) ([]*models.Subcategory, error) {
	// Construir filtro: categoria específica + opcionalmente status
	var filterBy string
	if includeInactive {
		// Apenas filtrar por categoria
		filterBy = fmt.Sprintf("tema_geral:=`%s`", category)
	} else {
		// Filtrar por categoria E status publicado
		filterBy = fmt.Sprintf("tema_geral:=`%s` && status:=1", category)
	}

	// Query com facet em sub_categoria
	searchParams := &api.SearchCollectionParams{
		Q:              pointer.String("*"),
		FacetBy:        pointer.String("sub_categoria"),
		MaxFacetValues: pointer.Int(250),
		PerPage:        pointer.Int(0),
		FilterBy:       pointer.String(filterBy),
	}

	result, err := scs.client.Collection(CollectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, err
	}

	// Extrair subcategorias dos facets
	subcategories, err := scs.extractSubcategoriesFromFacets(result, category)
	if err != nil {
		return nil, err
	}

	return subcategories, nil
}

// extractSubcategoriesFromFacets extrai subcategorias dos resultados de facet search
func (scs *SubcategoryService) extractSubcategoriesFromFacets(result *api.SearchResult, category string) ([]*models.Subcategory, error) {
	subcategories := []*models.Subcategory{}

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
		return subcategories, nil // Sem facets, retorna vazio
	}

	// Processar cada facet
	for _, facetInterface := range facetCounts {
		facet, ok := facetInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Verificar se é o facet de sub_categoria
		fieldName, _ := facet["field_name"].(string)
		if fieldName != "sub_categoria" {
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
				subcategories = append(subcategories, &models.Subcategory{
					Name:            name,
					Category:        category,
					Count:           count,
					PopularityScore: 0, // Será preenchido depois se implementarmos
				})
			}
		}
	}

	return subcategories, nil
}

// transformHitsToDocuments converte hits do Typesense em ServiceDocuments
func (scs *SubcategoryService) transformHitsToDocuments(result *api.SearchResult) []*models.ServiceDocument {
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

		// Transformar documento
		doc := scs.transformDocument(tsDoc)
		docs = append(docs, doc)
	}

	return docs
}

// transformDocument transforma um documento Typesense em ServiceDocument
func (scs *SubcategoryService) transformDocument(tsDoc map[string]interface{}) *models.ServiceDocument {
	// Extrair campos principais com mapeamento correto
	id := getString(tsDoc, "id")
	title := getString(tsDoc, "nome_servico")
	description := getString(tsDoc, "resumo")
	category := getString(tsDoc, "tema_geral")
	subcategory := getStringPtr(tsDoc, "sub_categoria")
	slug := getString(tsDoc, "slug")
	status := getInt32(tsDoc, "status")
	createdAt := getInt64(tsDoc, "created_at")
	updatedAt := getInt64(tsDoc, "last_update")

	// Todos os outros campos vão para metadata
	metadata := make(map[string]interface{})
	excludeFields := map[string]bool{
		"id": true, "nome_servico": true, "resumo": true,
		"tema_geral": true, "sub_categoria": true, "slug": true, "status": true, "created_at": true,
		"last_update": true, "embedding": true, // não retornar embedding
		"search_content": true, // não retornar search_content
		"slug_history": true,   // não retornar histórico de slugs
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
		Slug:        slug,
		Status:      status,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Metadata:    metadata,
	}
}

// filterNonEmpty remove subcategorias sem serviços
func (scs *SubcategoryService) filterNonEmpty(subcategories []*models.Subcategory) []*models.Subcategory {
	filtered := []*models.Subcategory{}
	for _, subcat := range subcategories {
		if subcat.Count > 0 {
			filtered = append(filtered, subcat)
		}
	}
	return filtered
}

// sortSubcategories ordena subcategorias conforme critério
func (scs *SubcategoryService) sortSubcategories(subcategories []*models.Subcategory, sortBy, order string) {
	sort.Slice(subcategories, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "alpha":
			less = subcategories[i].Name < subcategories[j].Name
		case "count":
			less = subcategories[i].Count < subcategories[j].Count
		default: // "popularity"
			less = subcategories[i].PopularityScore < subcategories[j].PopularityScore
		}

		// Inverter se ordem descendente
		if order == "desc" {
			return !less
		}
		return less
	})
}
