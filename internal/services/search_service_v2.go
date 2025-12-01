package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
	"github.com/typesense/typesense-go/v3/typesense/api/pointer"
)

// SearchServiceV2 provides multi-collection search (v2 API)
type SearchServiceV2 struct {
	client           *typesense.Client
	embeddingService EmbeddingProvider
	config           *config.Config
}

// NewSearchServiceV2 creates a new v2 search service
func NewSearchServiceV2(
	client *typesense.Client,
	embeddingService EmbeddingProvider,
	cfg *config.Config,
) *SearchServiceV2 {
	return &SearchServiceV2{
		client:           client,
		embeddingService: embeddingService,
		config:           cfg,
	}
}

// Search routes to specific search type
func (ss *SearchServiceV2) Search(ctx context.Context, req *models.SearchRequest) (*models.UnifiedSearchResponse, error) {
	// Validations
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PerPage < 1 || req.PerPage > 100 {
		req.PerPage = 10
	}

	switch req.Type {
	case models.SearchTypeKeyword:
		return ss.KeywordSearch(ctx, req)
	case models.SearchTypeSemantic:
		return ss.SemanticSearch(ctx, req)
	case models.SearchTypeHybrid:
		return ss.HybridSearch(ctx, req)
	default:
		return nil, fmt.Errorf("tipo de busca inválido: %s (AI search not yet implemented for v2)", req.Type)
	}
}

// KeywordSearch executes text-based search across multiple collections
func (ss *SearchServiceV2) KeywordSearch(ctx context.Context, req *models.SearchRequest) (*models.UnifiedSearchResponse, error) {
	collections := ss.config.SearchableCollections

	// Build search parameters for each collection
	searches := make([]api.MultiSearchCollectionParameters, 0, len(collections))
	for _, collName := range collections {
		collConfig := ss.config.GetCollectionConfig(collName)
		params := ss.buildKeywordSearchParams(collName, collConfig, req)
		searches = append(searches, params)
	}

	// Execute MultiSearch
	searchParams := api.MultiSearchSearchesParameter{
		Searches: searches,
	}

	result, err := ss.client.MultiSearch.Perform(ctx, &api.MultiSearchParams{}, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar MultiSearch: %w", err)
	}

	// Transform results to UnifiedDocuments
	docs, totalCount := ss.transformMultiSearchResults(result, collections)

	// Apply thresholds if specified
	filtered := docs
	if req.ScoreThreshold != nil && req.ScoreThreshold.Keyword != nil {
		filtered = ss.applyKeywordThreshold(docs, *req.ScoreThreshold.Keyword)
	}

	// Manual pagination
	paged := ss.paginateDocuments(filtered, req.Page, req.PerPage)

	return &models.UnifiedSearchResponse{
		Results:       paged,
		TotalCount:    totalCount,
		FilteredCount: len(filtered),
		Page:          req.Page,
		PerPage:       req.PerPage,
		SearchType:    models.SearchTypeKeyword,
		Collections:   collections,
	}, nil
}

// SemanticSearch executes vector-based search across multiple collections
func (ss *SearchServiceV2) SemanticSearch(ctx context.Context, req *models.SearchRequest) (*models.UnifiedSearchResponse, error) {
	if ss.embeddingService == nil {
		return nil, fmt.Errorf("serviço de embedding não disponível")
	}

	// Generate embedding for query
	embedding, err := ss.embeddingService.GenerateEmbedding(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar embedding: %w", err)
	}

	collections := ss.config.SearchableCollections

	// Build vector query string
	vectorQuery := buildVectorQueryString(embedding, 1.0) // alpha=1.0 for pure semantic

	// Build search parameters for each collection
	searches := make([]api.MultiSearchCollectionParameters, 0, len(collections))
	for _, collName := range collections {
		collConfig := ss.config.GetCollectionConfig(collName)
		params := ss.buildSemanticSearchParams(collName, collConfig, req, vectorQuery)
		searches = append(searches, params)
	}

	// Execute MultiSearch
	searchParams := api.MultiSearchSearchesParameter{
		Searches: searches,
	}

	result, err := ss.client.MultiSearch.Perform(ctx, &api.MultiSearchParams{}, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar MultiSearch: %w", err)
	}

	// Transform results
	docs, totalCount := ss.transformMultiSearchResults(result, collections)

	// Apply thresholds if specified
	filtered := docs
	if req.ScoreThreshold != nil && req.ScoreThreshold.Semantic != nil {
		filtered = ss.applySemanticThreshold(docs, *req.ScoreThreshold.Semantic)
	}

	// Manual pagination
	paged := ss.paginateDocuments(filtered, req.Page, req.PerPage)

	return &models.UnifiedSearchResponse{
		Results:       paged,
		TotalCount:    totalCount,
		FilteredCount: len(filtered),
		Page:          req.Page,
		PerPage:       req.PerPage,
		SearchType:    models.SearchTypeSemantic,
		Collections:   collections,
	}, nil
}

// HybridSearch executes combined text+vector search across multiple collections
func (ss *SearchServiceV2) HybridSearch(ctx context.Context, req *models.SearchRequest) (*models.UnifiedSearchResponse, error) {
	if ss.embeddingService == nil {
		// Fallback to keyword search if embeddings unavailable
		return ss.KeywordSearch(ctx, req)
	}

	// Generate embedding for query
	embedding, err := ss.embeddingService.GenerateEmbedding(ctx, req.Query)
	if err != nil {
		// Fallback to keyword search on embedding error
		return ss.KeywordSearch(ctx, req)
	}

	collections := ss.config.SearchableCollections

	// Use provided alpha or default to 0.3
	alpha := req.Alpha
	if alpha == 0 {
		alpha = 0.3
	}

	// Build vector query string
	vectorQuery := buildVectorQueryString(embedding, alpha)

	// Build search parameters for each collection
	searches := make([]api.MultiSearchCollectionParameters, 0, len(collections))
	for _, collName := range collections {
		collConfig := ss.config.GetCollectionConfig(collName)
		params := ss.buildHybridSearchParams(collName, collConfig, req, vectorQuery)
		searches = append(searches, params)
	}

	// Execute MultiSearch
	searchParams := api.MultiSearchSearchesParameter{
		Searches: searches,
	}

	result, err := ss.client.MultiSearch.Perform(ctx, &api.MultiSearchParams{}, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar MultiSearch: %w", err)
	}

	// Transform results
	docs, totalCount := ss.transformMultiSearchResults(result, collections)

	// Apply thresholds if specified
	filtered := docs
	if req.ScoreThreshold != nil && req.ScoreThreshold.Hybrid != nil {
		filtered = ss.applyHybridThreshold(docs, *req.ScoreThreshold.Hybrid)
	}

	// Manual pagination
	paged := ss.paginateDocuments(filtered, req.Page, req.PerPage)

	return &models.UnifiedSearchResponse{
		Results:       paged,
		TotalCount:    totalCount,
		FilteredCount: len(filtered),
		Page:          req.Page,
		PerPage:       req.PerPage,
		SearchType:    models.SearchTypeHybrid,
		Collections:   collections,
	}, nil
}

// GetDocumentByID retrieves a document by ID with optional collection hint
func (ss *SearchServiceV2) GetDocumentByID(ctx context.Context, id string, collectionHint string) (*models.UnifiedDocument, error) {
	collections := ss.config.SearchableCollections

	// If hint provided and valid, try it first
	if collectionHint != "" {
		if collConfig := ss.config.GetCollectionConfig(collectionHint); collConfig != nil {
			doc, err := ss.tryGetFromCollection(ctx, id, collectionHint, collConfig.Type)
			if err == nil {
				return doc, nil
			}
		}
	}

	// Search all searchable collections
	for _, collName := range collections {
		collConfig := ss.config.GetCollectionConfig(collName)
		doc, err := ss.tryGetFromCollection(ctx, id, collName, collConfig.Type)
		if err == nil {
			return doc, nil
		}
	}

	return nil, fmt.Errorf("document not found in any collection")
}

// ============================================================================
// Helper Methods
// ============================================================================

func (ss *SearchServiceV2) buildKeywordSearchParams(collName string, collConfig *config.CollectionConfig, req *models.SearchRequest) api.MultiSearchCollectionParameters {
	queryStr := req.Query

	// Build query_by from collection config
	queryBy := fmt.Sprintf("%s,%s", collConfig.TitleField, collConfig.DescField)
	if collName == "prefrio_services_base" {
		queryBy += ",search_content"
	}

	params := api.MultiSearchCollectionParameters{
		Collection: &collName,
		Q:          &queryStr,
		QueryBy:    &queryBy,
		Page:       pointer.Int(1), // We paginate manually after combining results
		PerPage:    pointer.Int(250),
	}

	// Add filter if collection requires it
	if collConfig.FilterField != "" && !req.IncludeInactive {
		filterBy := fmt.Sprintf("%s:=%s", collConfig.FilterField, collConfig.FilterValue)
		params.FilterBy = &filterBy
	}

	return params
}

func (ss *SearchServiceV2) buildSemanticSearchParams(collName string, collConfig *config.CollectionConfig, req *models.SearchRequest, vectorQuery string) api.MultiSearchCollectionParameters {
	queryStr := "*"

	params := api.MultiSearchCollectionParameters{
		Collection:  &collName,
		Q:           &queryStr,
		VectorQuery: &vectorQuery,
		Page:        pointer.Int(1),
		PerPage:     pointer.Int(250),
	}

	// Add filter if collection requires it
	if collConfig.FilterField != "" && !req.IncludeInactive {
		filterBy := fmt.Sprintf("%s:=%s", collConfig.FilterField, collConfig.FilterValue)
		params.FilterBy = &filterBy
	}

	return params
}

func (ss *SearchServiceV2) buildHybridSearchParams(collName string, collConfig *config.CollectionConfig, req *models.SearchRequest, vectorQuery string) api.MultiSearchCollectionParameters {
	queryStr := req.Query

	// Build query_by from collection config
	queryBy := fmt.Sprintf("%s,%s", collConfig.TitleField, collConfig.DescField)
	if collName == "prefrio_services_base" {
		queryBy += ",search_content"
	}

	params := api.MultiSearchCollectionParameters{
		Collection:  &collName,
		Q:           &queryStr,
		QueryBy:     &queryBy,
		VectorQuery: &vectorQuery,
		Page:        pointer.Int(1),
		PerPage:     pointer.Int(250),
	}

	// Add filter if collection requires it
	if collConfig.FilterField != "" && !req.IncludeInactive {
		filterBy := fmt.Sprintf("%s:=%s", collConfig.FilterField, collConfig.FilterValue)
		params.FilterBy = &filterBy
	}

	return params
}

func (ss *SearchServiceV2) transformMultiSearchResults(result *api.MultiSearchResult, collections []string) ([]*models.UnifiedDocument, int) {
	var docs []*models.UnifiedDocument
	totalCount := 0

	for i, res := range result.Results {
		if res.Found != nil {
			totalCount += int(*res.Found)
		}
		if res.Hits == nil {
			continue
		}

		collName := collections[i]
		collConfig := ss.config.GetCollectionConfig(collName)

		for _, hit := range *res.Hits {
			if hit.Document == nil {
				continue
			}

			// Convert to map
			docBytes, _ := json.Marshal(*hit.Document)
			var tsDoc map[string]interface{}
			json.Unmarshal(docBytes, &tsDoc)

			// Extract ID
			id := getString(tsDoc, "id")

			// Create unified document with pure data passthrough
			doc := &models.UnifiedDocument{
				ID:         id,
				Collection: collName,
				Type:       collConfig.Type,
				Data:       tsDoc,
				ScoreInfo:  ss.extractScoreInfo(&hit),
			}

			docs = append(docs, doc)
		}
	}

	return docs, totalCount
}

func (ss *SearchServiceV2) extractScoreInfo(hit *api.SearchResultHit) *models.ScoreInfo {
	scoreInfo := &models.ScoreInfo{}

	// Extract text_match if present
	if hit.TextMatch != nil {
		textMatch := float64(*hit.TextMatch)
		// Normalize using log normalization: log(1 + score) / log(1 + max_score)
		// Assuming max_score ~= 1000 for text_match
		normalized := logNormalize(textMatch, 1000.0)
		scoreInfo.TextMatchNormalized = &normalized
	}

	// Extract vector_distance if present
	if hit.VectorDistance != nil {
		distance := float64(*hit.VectorDistance)
		// Convert distance to similarity: 1 / (1 + distance)
		similarity := 1.0 / (1.0 + distance)
		scoreInfo.VectorSimilarity = &similarity
	}

	// Try to extract hybrid score from the raw hit document if available
	// The API may store this differently, so we'll extract from JSON if needed

	return scoreInfo
}

func (ss *SearchServiceV2) tryGetFromCollection(ctx context.Context, id string, collName string, collType string) (*models.UnifiedDocument, error) {
	result, err := ss.client.Collection(collName).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, err
	}

	resultBytes, _ := json.Marshal(result)
	var tsDoc map[string]interface{}
	json.Unmarshal(resultBytes, &tsDoc)

	return &models.UnifiedDocument{
		ID:         id,
		Collection: collName,
		Type:       collType,
		Data:       tsDoc,
	}, nil
}

func (ss *SearchServiceV2) applyKeywordThreshold(docs []*models.UnifiedDocument, threshold float64) []*models.UnifiedDocument {
	filtered := make([]*models.UnifiedDocument, 0)
	for _, doc := range docs {
		if doc.ScoreInfo != nil && doc.ScoreInfo.TextMatchNormalized != nil {
			if *doc.ScoreInfo.TextMatchNormalized >= threshold {
				doc.ScoreInfo.PassedThreshold = true
				doc.ScoreInfo.ThresholdApplied = "keyword"
				doc.ScoreInfo.ThresholdValue = &threshold
				filtered = append(filtered, doc)
			}
		}
	}
	return filtered
}

func (ss *SearchServiceV2) applySemanticThreshold(docs []*models.UnifiedDocument, threshold float64) []*models.UnifiedDocument {
	filtered := make([]*models.UnifiedDocument, 0)
	for _, doc := range docs {
		if doc.ScoreInfo != nil && doc.ScoreInfo.VectorSimilarity != nil {
			if *doc.ScoreInfo.VectorSimilarity >= threshold {
				doc.ScoreInfo.PassedThreshold = true
				doc.ScoreInfo.ThresholdApplied = "semantic"
				doc.ScoreInfo.ThresholdValue = &threshold
				filtered = append(filtered, doc)
			}
		}
	}
	return filtered
}

func (ss *SearchServiceV2) applyHybridThreshold(docs []*models.UnifiedDocument, threshold float64) []*models.UnifiedDocument {
	filtered := make([]*models.UnifiedDocument, 0)
	for _, doc := range docs {
		if doc.ScoreInfo != nil && doc.ScoreInfo.HybridScore != nil {
			if *doc.ScoreInfo.HybridScore >= threshold {
				doc.ScoreInfo.PassedThreshold = true
				doc.ScoreInfo.ThresholdApplied = "hybrid"
				doc.ScoreInfo.ThresholdValue = &threshold
				filtered = append(filtered, doc)
			}
		}
	}
	return filtered
}

func (ss *SearchServiceV2) paginateDocuments(docs []*models.UnifiedDocument, page, perPage int) []*models.UnifiedDocument {
	startIdx := (page - 1) * perPage
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(docs) {
		return []*models.UnifiedDocument{}
	}

	endIdx := startIdx + perPage
	if endIdx > len(docs) {
		endIdx = len(docs)
	}

	return docs[startIdx:endIdx]
}

// buildVectorQueryString builds the vector query string for Typesense
func buildVectorQueryString(embedding []float32, alpha float64) string {
	vectorStr := "["
	for i, val := range embedding {
		if i > 0 {
			vectorStr += ", "
		}
		vectorStr += fmt.Sprintf("%.6f", val)
	}
	vectorStr += "]"

	return fmt.Sprintf("embedding:(%s, alpha:%.1f)", vectorStr, alpha)
}

// logNormalize applies log normalization to a score
func logNormalize(score, maxScore float64) float64 {
	if score <= 0 {
		return 0
	}
	return logBase(1+score, 10) / logBase(1+maxScore, 10)
}

func logBase(x, base float64) float64 {
	return logNatural(x) / logNatural(base)
}

func logNatural(x float64) float64 {
	if x <= 0 {
		return 0
	}
	return math.Log(x)
}
