package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search/ranking"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
)

// TypesenseAdapter encapsula operações com Typesense
type TypesenseAdapter struct {
	client     *typesense.Client
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// NewTypesenseAdapter cria um novo adapter para Typesense
func NewTypesenseAdapter(client *typesense.Client, baseURL, apiKey string) *TypesenseAdapter {
	return &TypesenseAdapter{
		client:     client,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    baseURL,
		apiKey:     apiKey,
	}
}

// SearchResult representa resultado de uma busca
type SearchResult struct {
	Hits       []ranking.Hit
	TotalFound int
}

// KeywordSearch executa busca textual
func (t *TypesenseAdapter) KeywordSearch(
	ctx context.Context,
	collection string,
	query string,
	config *v3.SearchConfig,
	page, perPage int,
	filters map[string]interface{},
) (*SearchResult, error) {
	prioritizeExact := true
	prioritizePos := true
	numTypos := fmt.Sprintf("%d", config.NumTypos)

	queryBy := strings.Join(config.QueryBy, ",")
	weights := make([]string, len(config.QueryWeights))
	for i, w := range config.QueryWeights {
		weights[i] = fmt.Sprintf("%d", w)
	}
	queryByWeights := strings.Join(weights, ",")

	searchParams := &api.SearchCollectionParams{
		Q:                       &query,
		QueryBy:                 &queryBy,
		QueryByWeights:          &queryByWeights,
		PerPage:                 &perPage,
		Page:                    &page,
		PrioritizeExactMatch:    &prioritizeExact,
		PrioritizeTokenPosition: &prioritizePos,
		NumTypos:                &numTypos,
		SortBy:                  strPtr("_text_match:desc"),
		ExhaustiveSearch:        boolPtr(true),
	}

	// Aplica filtros
	filterBy := t.buildFilterBy(filters)
	if filterBy != "" {
		searchParams.FilterBy = &filterBy
	}

	result, err := t.client.Collection(collection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro na busca keyword: %w", err)
	}

	return t.transformResult(result), nil
}

// HybridSearch executa busca híbrida (texto + vetor)
func (t *TypesenseAdapter) HybridSearch(
	ctx context.Context,
	collection string,
	query string,
	embedding []float32,
	config *v3.SearchConfig,
	page, perPage int,
	filters map[string]interface{},
) (*SearchResult, error) {
	// Formata embedding
	embeddingStr := make([]string, len(embedding))
	for i, v := range embedding {
		embeddingStr[i] = fmt.Sprintf("%.6f", v)
	}
	vectorQuery := fmt.Sprintf("%s:([%s], alpha:%.2f)", config.EmbeddingField, strings.Join(embeddingStr, ","), config.Alpha)

	// Query textual
	queryBy := strings.Join(config.QueryBy, ",")
	weights := make([]string, len(config.QueryWeights))
	for i, w := range config.QueryWeights {
		weights[i] = fmt.Sprintf("%d", w)
	}
	queryByWeights := strings.Join(weights, ",")

	// Monta body para multi_search
	searchBody := map[string]interface{}{
		"collection":       collection,
		"q":                query,
		"query_by":         queryBy,
		"query_by_weights": queryByWeights,
		"vector_query":     vectorQuery,
		"per_page":         perPage,
		"page":             page,
	}

	// Aplica filtros
	filterBy := t.buildFilterBy(filters)
	if filterBy != "" {
		searchBody["filter_by"] = filterBy
	}

	multiSearchBody := map[string]interface{}{
		"searches": []interface{}{searchBody},
	}

	return t.executeMultiSearch(ctx, multiSearchBody)
}

// SemanticSearch executa busca puramente vetorial
func (t *TypesenseAdapter) SemanticSearch(
	ctx context.Context,
	collection string,
	embedding []float32,
	config *v3.SearchConfig,
	page, perPage int,
	filters map[string]interface{},
) (*SearchResult, error) {
	// Formata embedding
	embeddingStr := make([]string, len(embedding))
	for i, v := range embedding {
		embeddingStr[i] = fmt.Sprintf("%.6f", v)
	}
	vectorQuery := fmt.Sprintf("%s:([%s], alpha:1.0)", config.EmbeddingField, strings.Join(embeddingStr, ","))

	// Monta body para multi_search
	searchBody := map[string]interface{}{
		"collection":   collection,
		"q":            "*",
		"vector_query": vectorQuery,
		"per_page":     perPage,
		"page":         page,
	}

	// Aplica filtros
	filterBy := t.buildFilterBy(filters)
	if filterBy != "" {
		searchBody["filter_by"] = filterBy
	}

	multiSearchBody := map[string]interface{}{
		"searches": []interface{}{searchBody},
	}

	return t.executeMultiSearch(ctx, multiSearchBody)
}

// MultiCollectionSearch executa busca em múltiplas collections
func (t *TypesenseAdapter) MultiCollectionSearch(
	ctx context.Context,
	collections []string,
	query string,
	embedding []float32,
	config *v3.SearchConfig,
	searchType v3.SearchType,
	page, perPage int,
	filters map[string]interface{},
) (map[string]*SearchResult, error) {
	searches := make([]interface{}, 0, len(collections))

	queryBy := strings.Join(config.QueryBy, ",")
	weights := make([]string, len(config.QueryWeights))
	for i, w := range config.QueryWeights {
		weights[i] = fmt.Sprintf("%d", w)
	}
	queryByWeights := strings.Join(weights, ",")

	// Formata embedding se disponível
	var vectorQuery string
	if len(embedding) > 0 && (searchType == v3.SearchTypeSemantic || searchType == v3.SearchTypeHybrid || searchType == v3.SearchTypeAI) {
		embeddingStr := make([]string, len(embedding))
		for i, v := range embedding {
			embeddingStr[i] = fmt.Sprintf("%.6f", v)
		}
		alpha := config.Alpha
		if searchType == v3.SearchTypeSemantic {
			alpha = 1.0
		}
		vectorQuery = fmt.Sprintf("%s:([%s], alpha:%.2f)", config.EmbeddingField, strings.Join(embeddingStr, ","), alpha)
	}

	filterBy := t.buildFilterBy(filters)

	for _, coll := range collections {
		search := map[string]interface{}{
			"collection": coll,
			"per_page":   perPage,
			"page":       page,
		}

		if searchType == v3.SearchTypeSemantic {
			search["q"] = "*"
		} else {
			search["q"] = query
			search["query_by"] = queryBy
			search["query_by_weights"] = queryByWeights
		}

		if vectorQuery != "" {
			search["vector_query"] = vectorQuery
		}

		if filterBy != "" {
			search["filter_by"] = filterBy
		}

		searches = append(searches, search)
	}

	multiSearchBody := map[string]interface{}{
		"searches": searches,
	}

	// Executa multi_search
	jsonBody, err := json.Marshal(multiSearchBody)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar body: %w", err)
	}

	url := fmt.Sprintf("%s/multi_search", t.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TYPESENSE-API-KEY", t.apiKey)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("busca falhou (status %d): %s", resp.StatusCode, string(body))
	}

	var multiResult struct {
		Results []api.SearchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &multiResult); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	// Mapeia resultados por collection
	results := make(map[string]*SearchResult)
	for i, coll := range collections {
		if i < len(multiResult.Results) {
			results[coll] = t.transformResult(&multiResult.Results[i])
		}
	}

	return results, nil
}

func (t *TypesenseAdapter) executeMultiSearch(ctx context.Context, body map[string]interface{}) (*SearchResult, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar body: %w", err)
	}

	url := fmt.Sprintf("%s/multi_search", t.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TYPESENSE-API-KEY", t.apiKey)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("busca falhou (status %d): %s", resp.StatusCode, string(respBody))
	}

	var multiResult struct {
		Results []api.SearchResult `json:"results"`
	}
	if err := json.Unmarshal(respBody, &multiResult); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	if len(multiResult.Results) == 0 {
		return &SearchResult{Hits: []ranking.Hit{}, TotalFound: 0}, nil
	}

	return t.transformResult(&multiResult.Results[0]), nil
}

func (t *TypesenseAdapter) transformResult(result *api.SearchResult) *SearchResult {
	searchResult := &SearchResult{
		Hits: make([]ranking.Hit, 0),
	}

	if result.Found != nil {
		searchResult.TotalFound = *result.Found
	}

	if result.Hits == nil {
		return searchResult
	}

	for _, hit := range *result.Hits {
		if hit.Document == nil {
			continue
		}

		h := ranking.Hit{
			Document:       *hit.Document,
			TextMatch:      hit.TextMatch,
			VectorDistance: hit.VectorDistance,
		}
		searchResult.Hits = append(searchResult.Hits, h)
	}

	return searchResult
}

func (t *TypesenseAdapter) buildFilterBy(filters map[string]interface{}) string {
	if len(filters) == 0 {
		return ""
	}

	parts := make([]string, 0, len(filters))
	for key, value := range filters {
		switch v := value.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("%s:=%s", key, v))
		case int, int32, int64:
			parts = append(parts, fmt.Sprintf("%s:=%d", key, v))
		case bool:
			parts = append(parts, fmt.Sprintf("%s:=%t", key, v))
		}
	}

	return strings.Join(parts, " && ")
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
