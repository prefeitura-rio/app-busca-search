package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
	"google.golang.org/genai"
)

const (
	CollectionName = "prefrio_services_base"
)

var (
	ErrSearchCanceled = errors.New("busca cancelada")
)

// SearchService fornece busca unificada de alta qualidade
type SearchService struct {
	client           *typesense.Client
	embeddingService EmbeddingProvider
	geminiClient     *genai.Client
	cache            Cache
	chatModel        string
	// Configurações para HTTP direto
	typesenseURL string
	typesenseKey string
	httpClient   *http.Client
}

// NewSearchService cria um novo serviço de busca
func NewSearchService(
	client *typesense.Client,
	geminiClient *genai.Client,
	embeddingModel string,
	cache Cache,
	typesenseURL string,
	typesenseKey string,
) *SearchService {
	var embeddingService EmbeddingProvider
	if geminiClient != nil {
		embeddingService = NewGeminiEmbeddingProvider(geminiClient, embeddingModel, cache)
	}

	return &SearchService{
		client:           client,
		embeddingService: embeddingService,
		geminiClient:     geminiClient,
		cache:            cache,
		chatModel:        "gemini-2.5-flash",
		typesenseURL:     typesenseURL,
		typesenseKey:     typesenseKey,
		httpClient:       &http.Client{Timeout: 60 * time.Second},
	}
}

// Search executa busca baseada no tipo especificado
func (ss *SearchService) Search(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	// Validações
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PerPage < 1 || req.PerPage > 100 {
		req.PerPage = 10
	}

	// Executa busca baseada no tipo
	switch req.Type {
	case models.SearchTypeKeyword:
		return ss.KeywordSearch(ctx, req)
	case models.SearchTypeSemantic:
		return ss.SemanticSearch(ctx, req)
	case models.SearchTypeHybrid:
		return ss.HybridSearch(ctx, req)
	case models.SearchTypeAI:
		return ss.AIAgentSearch(ctx, req)
	default:
		return nil, fmt.Errorf("tipo de busca inválido: %s", req.Type)
	}
}

// ============================================================================
// KEYWORD SEARCH - Busca textual BM25 otimizada
// ============================================================================

func (ss *SearchService) KeywordSearch(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	prioritizeExact := true
	prioritizePos := true

	searchParams := &api.SearchCollectionParams{
		Q: &req.Query,
		// Campos ordenados por relevância
		QueryBy: stringPtr("nome_servico,resumo,descricao_completa,documentos_necessarios,instrucoes_solicitante"),
		// Pesos: nome do serviço é mais importante
		QueryByWeights:          stringPtr("4,3,2,1,1"),
		PerPage:                 intPtr(req.PerPage),
		Page:                    intPtr(req.Page),
		PrioritizeExactMatch:    &prioritizeExact,
		PrioritizeTokenPosition: &prioritizePos,
		DropTokensThreshold:     intPtr(1),
		SortBy:                  stringPtr("_text_match:desc"),
		ExhaustiveSearch:        boolPtr(true),
	}

	// Filtro de status (apenas publicados, a menos que include_inactive)
	if !req.IncludeInactive {
		searchParams.FilterBy = stringPtr("status:=1")
	}

	// Executar busca
	result, err := ss.client.Collection(CollectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar busca keyword: %w", err)
	}

	// Transformar resultados
	docs, err := ss.transformResults(result)
	if err != nil {
		return nil, err
	}

	totalCount := 0
	if result.Found != nil {
		totalCount = *result.Found
	}

	return &models.SearchResponse{
		Results:    docs,
		TotalCount: totalCount,
		Page:       req.Page,
		PerPage:    req.PerPage,
		SearchType: models.SearchTypeKeyword,
	}, nil
}

// ============================================================================
// SEMANTIC SEARCH - Busca vetorial pura
// ============================================================================

func (ss *SearchService) SemanticSearch(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	if ss.embeddingService == nil {
		return nil, fmt.Errorf("busca semântica requer serviço de embeddings configurado")
	}

	// Gerar embedding da query com timeout
	ctxEmbed, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	embedding, err := ss.embeddingService.GenerateEmbedding(ctxEmbed, req.Query)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctxEmbed.Err(), context.Canceled) {
			log.Printf("Semantic search canceled for query: %s", req.Query)
			return nil, ErrSearchCanceled
		}
		return nil, fmt.Errorf("erro ao gerar embedding: %v", err)
	}

	// Busca vetorial pura (alpha = 1.0 = 100% vector)
	return ss.executeVectorSearch(ctx, req, embedding, 1.0)
}

// ============================================================================
// HYBRID SEARCH - Combinação otimizada de texto + vetor
// ============================================================================

func (ss *SearchService) HybridSearch(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	// Tentar gerar embedding com fallback gracioso para keyword
	var embedding []float32
	var err error

	if ss.embeddingService != nil {
		ctxEmbed, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		embedding, err = ss.embeddingService.GenerateEmbedding(ctxEmbed, req.Query)
		if err != nil {
			log.Printf("Hybrid search fallback to keyword: %v", err)
			return ss.KeywordSearch(ctx, req)
		}
	} else {
		// Sem embeddings, fallback para keyword
		return ss.KeywordSearch(ctx, req)
	}

	// Alpha configurável (default 0.3 = 70% texto + 30% vetor)
	alpha := 0.3
	if req.Alpha > 0 && req.Alpha <= 1.0 {
		alpha = req.Alpha
	}

	return ss.executeVectorSearch(ctx, req, embedding, alpha)
}

// executeVectorSearch executa busca com vector query usando HTTP POST direto
func (ss *SearchService) executeVectorSearch(
	ctx context.Context,
	req *models.SearchRequest,
	embedding []float32,
	alpha float64,
) (*models.SearchResponse, error) {
	// Formatar embedding como array de floats
	embeddingStr := make([]string, len(embedding))
	for i, v := range embedding {
		embeddingStr[i] = fmt.Sprintf("%.6f", v)
	}
	vectorQuery := fmt.Sprintf("embedding:([%s], alpha:%.2f)", strings.Join(embeddingStr, ","), alpha)

	// Montar o body da requisição POST para multi_search
	search := map[string]interface{}{
		"collection":   CollectionName,
		"q":            "*",
		"vector_query": vectorQuery,
		"per_page":     req.PerPage,
		"page":         req.Page,
	}

	// Filtro de status
	if !req.IncludeInactive {
		search["filter_by"] = "status:=1"
	}

	// Se alpha < 1.0, incluir busca textual híbrida
	if alpha < 1.0 {
		search["q"] = req.Query
		search["query_by"] = "nome_servico,resumo,descricao_completa"
		search["query_by_weights"] = "4,3,2"
	}

	// Montar multi_search body
	multiSearchBody := map[string]interface{}{
		"searches": []interface{}{search},
	}

	// Converter para JSON
	jsonBody, err := json.Marshal(multiSearchBody)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar body: %w", err)
	}

	// Montar URL do endpoint multi_search
	url := fmt.Sprintf("%s/multi_search", ss.typesenseURL)

	// Criar requisição POST
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	// Headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-TYPESENSE-API-KEY", ss.typesenseKey)

	// Executar requisição
	resp, err := ss.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar busca vetorial: %w", err)
	}
	defer resp.Body.Close()

	// Ler resposta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	// Verificar status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("busca vetorial falhou (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse resposta do multi_search
	var multiResult struct {
		Results []api.SearchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &multiResult); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	// Extrair primeiro resultado (nossa única busca)
	if len(multiResult.Results) == 0 {
		return &models.SearchResponse{
			Results:    []*models.ServiceDocument{},
			TotalCount: 0,
			Page:       req.Page,
			PerPage:    req.PerPage,
			SearchType: models.SearchTypeSemantic,
		}, nil
	}

	result := &multiResult.Results[0]

	// Transformar resultados
	docs, err := ss.transformResults(result)
	if err != nil {
		return nil, err
	}

	totalCount := 0
	if result.Found != nil {
		totalCount = *result.Found
	}

	searchType := models.SearchTypeSemantic
	if alpha < 1.0 {
		searchType = models.SearchTypeHybrid
	}

	return &models.SearchResponse{
		Results:    docs,
		TotalCount: totalCount,
		Page:       req.Page,
		PerPage:    req.PerPage,
		SearchType: searchType,
	}, nil
}

// ============================================================================
// AI AGENT SEARCH - Busca inteligente com LLM
// ============================================================================

func (ss *SearchService) AIAgentSearch(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	if ss.geminiClient == nil {
		// Fallback para hybrid
		log.Printf("AI search unavailable, falling back to hybrid")
		return ss.HybridSearch(ctx, req)
	}

	startTime := time.Now()
	metrics := &models.AISearchMetrics{}

	// 1. Análise da query com LLM (1 chamada Gemini)
	analysis, err := ss.analyzeQuery(ctx, req.Query)
	if err != nil {
		log.Printf("AI analysis failed, fallback to hybrid: %v", err)
		return ss.HybridSearch(ctx, req)
	}
	metrics.GeminiCalls++

	// 2. Executar busca baseada na estratégia sugerida pelo LLM
	var results *models.SearchResponse

	switch analysis.SearchStrategy {
	case "semantic":
		results, err = ss.SemanticSearch(ctx, req)
		if err == nil {
			metrics.GeminiCalls++ // embedding
		}
	case "keyword":
		results, err = ss.KeywordSearch(ctx, req)
	default: // hybrid
		results, err = ss.HybridSearch(ctx, req)
		if err == nil {
			metrics.GeminiCalls++ // embedding
		}
	}

	if err != nil {
		return nil, err
	}

	// 3. Re-ranking condicional (apenas se confiança baixa E muitos resultados)
	if analysis.Confidence < 0.7 && len(results.Results) >= 10 {
		reranked, rerankErr := ss.rerankResults(ctx, req.Query, analysis.Intent, results.Results)
		if rerankErr == nil {
			results.Results = reranked
			metrics.RerankExecuted = true
			metrics.GeminiCalls++
		}
	}

	// Adicionar metadata
	metrics.TotalTime = float64(time.Since(startTime).Milliseconds())
	results.Metadata = map[string]interface{}{
		"analysis": analysis,
		"metrics":  metrics,
	}
	results.SearchType = models.SearchTypeAI

	return results, nil
}

// analyzeQuery analisa a query com LLM usando structured outputs
func (ss *SearchService) analyzeQuery(ctx context.Context, query string) (*models.QueryAnalysis, error) {
	// Verificar cache
	cacheKey := "analysis:" + query
	if cached := ss.cache.Get(cacheKey); cached != nil {
		return cached.(*models.QueryAnalysis), nil
	}

	// Timeout de 60s para análise
	ctxAnalysis, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Prompt otimizado para análise estruturada
	prompt := fmt.Sprintf(`Analise esta query de busca de serviços públicos e retorne JSON:

Query: "%s"

Retorne JSON com:
{
  "intent": "buscar_servico|listar_categoria|esclarecer_duvida",
  "keywords": ["palavra1", "palavra2"],
  "categories": ["Educação", "Saúde"],
  "refined_queries": ["variação 1", "variação 2"],
  "search_strategy": "keyword|semantic|hybrid",
  "confidence": 0.85,
  "portal_tags": ["carioca-digital"]
}

Regras:
- intent: o que o usuário quer fazer
- keywords: termos-chave principais
- categories: categorias inferidas (Educação, Saúde, Transporte, etc)
- refined_queries: max 2 reformulações da query
- search_strategy: keyword para buscas literais, semantic para conceituais, hybrid para misto
- confidence: 0-1 (quão claro é o intent)
- portal_tags: ["carioca-digital"] se relacionado

Retorne APENAS o JSON, sem explicações.`, query)

	content := genai.NewContentFromText(prompt, genai.RoleUser)

	resp, err := ss.geminiClient.Models.GenerateContent(ctxAnalysis, ss.chatModel, []*genai.Content{content}, nil)

	if err != nil {
		return nil, fmt.Errorf("erro ao chamar Gemini: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("resposta vazia do Gemini")
	}

	// Parse JSON response
	// Extract text from Part - fmt.Sprintf includes struct representation, so we need to extract JSON
	part := resp.Candidates[0].Content.Parts[0]
	fullStr := fmt.Sprintf("%v", part)

	// The response may be wrapped in markdown code blocks and prefixed with struct data
	// Look for ```json marker first
	var jsonStr string
	if idx := strings.Index(fullStr, "```json"); idx != -1 {
		// Found markdown code block - extract JSON after ```json
		jsonStart := idx + len("```json")
		jsonStr = fullStr[jsonStart:]
		// Remove closing ```
		if endIdx := strings.Index(jsonStr, "```"); endIdx != -1 {
			jsonStr = jsonStr[:endIdx]
		}
	} else {
		// No markdown - look for JSON object starting with { followed by newline (not struct representation)
		// The pattern for JSON is "{\n" while struct is "&{" or "{ <content> }"
		if idx := strings.Index(fullStr, "{\n"); idx != -1 {
			jsonStr = fullStr[idx:]
		} else {
			log.Printf("No JSON found in Gemini response: %s", fullStr)
			return nil, fmt.Errorf("resposta do Gemini não contém JSON")
		}
	}

	jsonStr = strings.TrimSpace(jsonStr)

	var analysis models.QueryAnalysis
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		log.Printf("Failed to parse Gemini JSON response: %s", jsonStr)
		return nil, fmt.Errorf("erro ao parsear JSON do Gemini: %w", err)
	}

	// Cache por 5 minutos
	ss.cache.Set(cacheKey, &analysis, 5*time.Minute)

	return &analysis, nil
}

// rerankResults re-ordena resultados usando LLM
func (ss *SearchService) rerankResults(ctx context.Context, query string, intent string, results []*models.ServiceDocument) ([]*models.ServiceDocument, error) {
	if len(results) == 0 {
		return results, nil
	}

	// Limitar a 10 melhores resultados para re-ranking
	topResults := results
	if len(results) > 10 {
		topResults = results[:10]
	}

	// Preparar lista de serviços para o LLM
	services := make([]string, len(topResults))
	for i, doc := range topResults {
		services[i] = fmt.Sprintf("%d. [ID:%s] %s - %s", i+1, doc.ID, doc.Title, doc.Description)
	}

	prompt := fmt.Sprintf(`Reordene estes serviços por relevância para a query.

Query: "%s"
Intent: %s

Serviços:
%s

Retorne JSON com array de IDs na ordem de relevância:
{"ranked_ids": ["id1", "id2", "id3", ...]}

Retorne APENAS o JSON.`, query, intent, strings.Join(services, "\n"))

	content := genai.NewContentFromText(prompt, genai.RoleUser)

	resp, err := ss.geminiClient.Models.GenerateContent(ctx, ss.chatModel, []*genai.Content{content}, nil)

	if err != nil {
		return results, err // Retorna original em caso de erro
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return results, nil
	}

	// Parse response
	part := resp.Candidates[0].Content.Parts[0]
	fullStr := fmt.Sprintf("%v", part)

	// Look for ```json marker first
	var jsonStr string
	if idx := strings.Index(fullStr, "```json"); idx != -1 {
		// Found markdown code block - extract JSON after ```json
		jsonStart := idx + len("```json")
		jsonStr = fullStr[jsonStart:]
		// Remove closing ```
		if endIdx := strings.Index(jsonStr, "```"); endIdx != -1 {
			jsonStr = jsonStr[:endIdx]
		}
	} else {
		// No markdown - look for JSON object starting with { followed by newline
		if idx := strings.Index(fullStr, "{\n"); idx != -1 {
			jsonStr = fullStr[idx:]
		} else {
			log.Printf("No JSON found in rerank response: %s", fullStr)
			return results, nil
		}
	}

	jsonStr = strings.TrimSpace(jsonStr)

	var rankResult struct {
		RankedIDs []string `json:"ranked_ids"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &rankResult); err != nil {
		log.Printf("Failed to parse rerank JSON: %s", jsonStr)
		return results, nil
	}

	// Reordenar baseado nos IDs
	reranked := make([]*models.ServiceDocument, 0, len(topResults))
	idMap := make(map[string]*models.ServiceDocument)
	for _, doc := range topResults {
		idMap[doc.ID] = doc
	}

	for _, id := range rankResult.RankedIDs {
		if doc, exists := idMap[id]; exists {
			reranked = append(reranked, doc)
			delete(idMap, id)
		}
	}

	// Adicionar documentos não ranqueados no final
	for _, doc := range topResults {
		if _, exists := idMap[doc.ID]; exists {
			reranked = append(reranked, doc)
		}
	}

	// Se tínhamos mais de 10, adicionar o resto
	if len(results) > 10 {
		reranked = append(reranked, results[10:]...)
	}

	return reranked, nil
}

// ============================================================================
// HELPERS
// ============================================================================

// transformResults transforma resultados Typesense em ServiceDocument
func (ss *SearchService) transformResults(result *api.SearchResult) ([]*models.ServiceDocument, error) {
	docs := make([]*models.ServiceDocument, 0)

	if result.Hits == nil {
		return docs, nil
	}

	for _, hit := range *result.Hits {
		if hit.Document == nil {
			continue
		}

		doc := ss.transformDocument(*hit.Document)
		docs = append(docs, doc)
	}

	return docs, nil
}

// transformDocument transforma um documento Typesense em ServiceDocument
func (ss *SearchService) transformDocument(tsDoc map[string]interface{}) *models.ServiceDocument {
	// Extrair campos principais
	id := getString(tsDoc, "id")
	title := getString(tsDoc, "nome_servico")
	description := getString(tsDoc, "resumo")
	category := getString(tsDoc, "tema_geral")
	status := getInt32(tsDoc, "status")
	createdAt := getInt64(tsDoc, "created_at")
	updatedAt := getInt64(tsDoc, "last_update")

	// Todos os outros campos vão para metadata
	metadata := make(map[string]interface{})
	excludeFields := map[string]bool{
		"id": true, "nome_servico": true, "resumo": true,
		"tema_geral": true, "status": true, "created_at": true,
		"last_update": true, "embedding": true, // não retornar embedding
		"search_content": true, // não retornar search_content bagunçado
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
		Status:      status,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Metadata:    metadata,
	}
}

// Helpers para extrair valores
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt32(m map[string]interface{}, key string) int32 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return int32(val)
		case int32:
			return val
		case int64:
			return int32(val)
		case float64:
			return int32(val)
		}
	}
	return 0
}

func getInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return int64(val)
		case int32:
			return int64(val)
		case int64:
			return val
		case float64:
			return int64(val)
		}
	}
	return 0
}

// Pointer helpers
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }
func boolPtr(b bool) *bool       { return &b }
