package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/genai"
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
	chatModel string,
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
		chatModel:        chatModel,
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
	ctx, span := otel.Tracer("search").Start(ctx, "KeywordSearch")
	defer span.End()

	span.SetAttributes(
		attribute.String("search.query", req.Query),
		attribute.Int("search.page", req.Page),
		attribute.Int("search.per_page", req.PerPage),
	)

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

	// Aplicar filtros (status, exclusive_for_agents)
	if filterBy := buildFilterBy(req); filterBy != "" {
		searchParams.FilterBy = stringPtr(filterBy)
	}

	// Executar busca
	_, typesenseSpan := otel.Tracer("search").Start(ctx, "Typesense.KeywordSearch")
	result, err := ss.client.Collection(CollectionName).Documents().Search(ctx, searchParams)
	typesenseSpan.End()

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Typesense search failed")
		return nil, fmt.Errorf("erro ao executar busca keyword: %w", err)
	}

	// Transformar resultados
	docs, err := ss.transformResults(result)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Transform results failed")
		return nil, err
	}

	span.SetAttributes(attribute.Int("search.results.raw_count", len(docs)))

	// Total original do Typesense
	totalCount := 0
	if result.Found != nil {
		totalCount = *result.Found
	}

	// Aplicar filtro de score threshold
	_, filterSpan := otel.Tracer("search").Start(ctx, "ApplyScoreThreshold")
	filteredDocs, filterMeta := ss.applyScoreThreshold(docs, req, models.SearchTypeKeyword)
	filterSpan.End()

	span.SetAttributes(attribute.Int("search.results.filtered_count", len(filteredDocs)))

	response := &models.SearchResponse{
		Results:       filteredDocs,
		TotalCount:    totalCount,
		FilteredCount: len(filteredDocs),
		Page:          req.Page,
		PerPage:       req.PerPage,
		SearchType:    models.SearchTypeKeyword,
	}

	// Adicionar metadata de filtragem se aplicável
	if filterMeta != nil {
		response.Metadata = filterMeta
	}

	return response, nil
}

// ============================================================================
// SEMANTIC SEARCH - Busca vetorial pura
// ============================================================================

func (ss *SearchService) SemanticSearch(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	ctx, span := otel.Tracer("search").Start(ctx, "SemanticSearch")
	defer span.End()

	span.SetAttributes(
		attribute.String("search.query", req.Query),
		attribute.Int("search.page", req.Page),
		attribute.Int("search.per_page", req.PerPage),
	)

	if ss.embeddingService == nil {
		span.SetStatus(codes.Error, "Embedding service not configured")
		return nil, fmt.Errorf("busca semântica requer serviço de embeddings configurado")
	}

	// Gerar embedding da query com timeout
	ctxEmbed, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, embeddingSpan := otel.Tracer("search").Start(ctx, "GenerateEmbedding")
	embedding, err := ss.embeddingService.GenerateEmbedding(ctxEmbed, req.Query)
	embeddingSpan.End()

	if err != nil {
		span.RecordError(err)
		if errors.Is(err, context.Canceled) || errors.Is(ctxEmbed.Err(), context.Canceled) {
			span.SetStatus(codes.Error, "Embedding generation canceled")
			log.Printf("Semantic search canceled for query: %s", req.Query)
			return nil, ErrSearchCanceled
		}
		span.SetStatus(codes.Error, "Embedding generation failed")
		return nil, fmt.Errorf("erro ao gerar embedding: %v", err)
	}

	span.SetAttributes(attribute.Int("search.embedding.dimensions", len(embedding)))

	// Busca vetorial pura (alpha = 1.0 = 100% vector)
	return ss.executeVectorSearch(ctx, req, embedding, 1.0)
}

// ============================================================================
// HYBRID SEARCH - Combinação otimizada de texto + vetor
// ============================================================================

func (ss *SearchService) HybridSearch(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	ctx, span := otel.Tracer("search").Start(ctx, "HybridSearch")
	defer span.End()

	span.SetAttributes(
		attribute.String("search.query", req.Query),
		attribute.Int("search.page", req.Page),
		attribute.Int("search.per_page", req.PerPage),
	)

	// Tentar gerar embedding com fallback gracioso para keyword
	var embedding []float32
	var err error

	if ss.embeddingService != nil {
		ctxEmbed, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		_, embeddingSpan := otel.Tracer("search").Start(ctx, "GenerateEmbedding")
		embedding, err = ss.embeddingService.GenerateEmbedding(ctxEmbed, req.Query)
		embeddingSpan.End()

		if err != nil {
			span.AddEvent("Fallback to KeywordSearch due to embedding failure")
			log.Printf("Hybrid search fallback to keyword: %v", err)
			return ss.KeywordSearch(ctx, req)
		}

		span.SetAttributes(attribute.Int("search.embedding.dimensions", len(embedding)))
	} else {
		// Sem embeddings, fallback para keyword
		span.AddEvent("Fallback to KeywordSearch - no embedding service")
		return ss.KeywordSearch(ctx, req)
	}

	// Alpha configurável (default 0.3 = 70% texto + 30% vetor)
	alpha := 0.3
	if req.Alpha > 0 && req.Alpha <= 1.0 {
		alpha = req.Alpha
	}

	span.SetAttributes(attribute.Float64("search.alpha", alpha))

	return ss.executeVectorSearch(ctx, req, embedding, alpha)
}

// executeVectorSearch executa busca com vector query usando HTTP POST direto
func (ss *SearchService) executeVectorSearch(
	ctx context.Context,
	req *models.SearchRequest,
	embedding []float32,
	alpha float64,
) (*models.SearchResponse, error) {
	ctx, span := otel.Tracer("search").Start(ctx, "ExecuteVectorSearch")
	defer span.End()

	span.SetAttributes(
		attribute.Int("search.embedding.size", len(embedding)),
		attribute.Float64("search.alpha", alpha),
	)

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

	// Aplicar filtros (status, exclusive_for_agents)
	if filterBy := buildFilterBy(req); filterBy != "" {
		search["filter_by"] = filterBy
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
	_, httpSpan := otel.Tracer("search").Start(ctx, "HTTP.POST.MultiSearch")
	httpSpan.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("http.url", url),
	)
	resp, err := ss.httpClient.Do(httpReq)
	httpSpan.End()

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "HTTP request failed")
		return nil, fmt.Errorf("erro ao executar busca vetorial: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	// Ler resposta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read response body")
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	// Verificar status
	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
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
			Results:       []*models.ServiceDocument{},
			TotalCount:    0,
			FilteredCount: 0,
			Page:          req.Page,
			PerPage:       req.PerPage,
			SearchType:    models.SearchTypeSemantic,
		}, nil
	}

	result := &multiResult.Results[0]

	// Total original do Typesense
	totalCount := 0
	if result.Found != nil {
		totalCount = *result.Found
	}

	// Transformar resultados
	docs, err := ss.transformResults(result)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Transform results failed")
		return nil, err
	}

	span.SetAttributes(attribute.Int("search.results.raw_count", len(docs)))

	// Determinar tipo de busca
	searchType := models.SearchTypeSemantic
	if alpha < 1.0 {
		searchType = models.SearchTypeHybrid
	}

	// Aplicar filtro de score threshold
	_, filterSpan := otel.Tracer("search").Start(ctx, "ApplyScoreThreshold")
	filteredDocs, filterMeta := ss.applyScoreThreshold(docs, req, searchType)
	filterSpan.End()

	span.SetAttributes(attribute.Int("search.results.filtered_count", len(filteredDocs)))

	response := &models.SearchResponse{
		Results:       filteredDocs,
		TotalCount:    totalCount,
		FilteredCount: len(filteredDocs),
		Page:          req.Page,
		PerPage:       req.PerPage,
		SearchType:    searchType,
	}

	// Adicionar metadata de filtragem se aplicável
	if filterMeta != nil {
		response.Metadata = filterMeta
	}

	return response, nil
}

// ============================================================================
// AI AGENT SEARCH - Busca inteligente com LLM
// ============================================================================

func (ss *SearchService) AIAgentSearch(ctx context.Context, req *models.SearchRequest) (*models.SearchResponse, error) {
	ctx, span := otel.Tracer("search").Start(ctx, "AIAgentSearch")
	defer span.End()

	span.SetAttributes(
		attribute.String("search.query", req.Query),
		attribute.Int("search.page", req.Page),
		attribute.Int("search.per_page", req.PerPage),
	)

	if ss.geminiClient == nil {
		// Fallback para hybrid
		span.AddEvent("Fallback to HybridSearch - no Gemini client")
		log.Printf("AI search unavailable, falling back to hybrid")
		return ss.HybridSearch(ctx, req)
	}

	startTime := time.Now()
	metrics := &models.AISearchMetrics{}

	// 1. Análise da query com LLM (1 chamada Gemini)
	_, analysisSpan := otel.Tracer("search").Start(ctx, "Gemini.AnalyzeQuery")
	analysis, err := ss.analyzeQuery(ctx, req.Query)
	analysisSpan.End()

	if err != nil {
		span.AddEvent("Fallback to HybridSearch - analysis failed")
		span.RecordError(err)
		log.Printf("AI analysis failed, fallback to hybrid: %v", err)
		return ss.HybridSearch(ctx, req)
	}
	metrics.GeminiCalls++

	span.SetAttributes(
		attribute.String("ai.intent", analysis.Intent),
		attribute.String("ai.search_strategy", analysis.SearchStrategy),
		attribute.Float64("ai.confidence", analysis.Confidence),
	)

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
		_, rerankSpan := otel.Tracer("search").Start(ctx, "Gemini.RerankResults")
		reranked, rerankErr := ss.rerankResults(ctx, req.Query, analysis.Intent, results.Results)
		rerankSpan.End()

		if rerankErr == nil {
			results.Results = reranked
			metrics.RerankExecuted = true
			metrics.GeminiCalls++
			span.AddEvent("Results reranked by Gemini")
		} else {
			span.AddEvent("Reranking failed, using original order")
		}
	}

	// 4. AI Scoring com LLM (se generate_scores=true)
	if req.GenerateScores && len(results.Results) > 0 {
		_, scoringSpan := otel.Tracer("search").Start(ctx, "Gemini.GenerateAIScores")
		topN := 20 // Configurável (máximo 20 por limitação do batch)
		if len(results.Results) < topN {
			topN = len(results.Results)
		}

		err := ss.generateAIScores(ctx, req.Query, results.Results, topN)
		scoringSpan.End()

		if err == nil {
			// OTIMIZAÇÃO: Apenas 1 chamada Gemini (batch) ao invés de topN chamadas
			metrics.GeminiCalls += 1
			span.AddEvent(fmt.Sprintf("Generated AI scores for top %d results in 1 batch call", topN))

			// Aplicar threshold_ai se especificado
			if req.ScoreThreshold != nil && req.ScoreThreshold.AI != nil {
				originalCount := len(results.Results)
				filtered := make([]*models.ServiceDocument, 0)

				for _, doc := range results.Results {
					aiScore := getAIFinalScore(doc)
					if aiScore >= *req.ScoreThreshold.AI {
						filtered = append(filtered, doc)
					}
				}

				results.Results = filtered
				results.FilteredCount = len(filtered)
				span.AddEvent(fmt.Sprintf("Applied AI threshold %.2f: %d -> %d results",
					*req.ScoreThreshold.AI, originalCount, len(filtered)))
			}

			// Adicionar AI scores ao ScoreInfo de cada documento
			for _, doc := range results.Results {
				if _, ok := doc.Metadata["ai_score"]; ok {
					// Obter ou criar ScoreInfo
					var scoreInfo *models.ScoreInfo
					if scoreInfoRaw, exists := doc.Metadata["score_info"]; exists {
						scoreInfo, _ = scoreInfoRaw.(*models.ScoreInfo)
					}
					if scoreInfo == nil {
						scoreInfo = &models.ScoreInfo{}
					}

					// Manter ScoreInfo (AI scores ficam no ai_score separado)
					doc.Metadata["score_info"] = scoreInfo
				}
			}
		} else {
			span.AddEvent("AI scoring failed, continuing without scores")
			log.Printf("Failed to generate AI scores: %v", err)
		}
	}

	// Adicionar metadata
	metrics.TotalTime = float64(time.Since(startTime).Milliseconds())
	span.SetAttributes(
		attribute.Int("ai.gemini_calls", metrics.GeminiCalls),
		attribute.Bool("ai.rerank_executed", metrics.RerankExecuted),
		attribute.Float64("ai.total_time_ms", metrics.TotalTime),
	)

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

// generateAIScores gera scores detalhados via LLM em UMA ÚNICA CHAMADA com structured output
// OTIMIZAÇÃO: Ao invés de N chamadas (1 por doc), faz 1 chamada batch para todos os docs
func (ss *SearchService) generateAIScores(
	ctx context.Context,
	query string,
	docs []*models.ServiceDocument,
	topN int,
) error {
	if len(docs) == 0 {
		return nil
	}

	// Limitar ao top-N (máximo 20 para evitar exceder contexto)
	const MAX_BATCH_SIZE = 20
	limit := topN
	if limit > MAX_BATCH_SIZE {
		limit = MAX_BATCH_SIZE
	}
	if len(docs) < limit {
		limit = len(docs)
	}
	scoresToGenerate := docs[:limit]

	// Timeout de 30s para batch (mais generoso que 10s individual)
	ctxScore, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Construir lista de serviços para o prompt
	servicesList := make([]string, len(scoresToGenerate))
	for i, doc := range scoresToGenerate {
		servicesList[i] = fmt.Sprintf("%d. [ID:%s] %s - %s",
			i+1, doc.ID, doc.Title, doc.Description)
	}

	// Prompt estruturado para scoring em batch com CATEGORIAS (não números)
	prompt := fmt.Sprintf(`Analise a relevância de cada um dos serviços abaixo para a busca do usuário.

Query do usuário: "%s"

Serviços a avaliar:
%s

Retorne um JSON com array de avaliações, uma para cada serviço (na mesma ordem):
{
  "scores": [
    {
      "service_id": "id-do-servico",
      "relevance_category": "muito_relevante",
      "confidence_level": "alta",
      "exact_match": false,
      "reasoning": "Breve explicação..."
    }
  ]
}

Campos a avaliar:
- service_id: ID do serviço (copiar exatamente do [ID:...])
- relevance_category: Use EXATAMENTE uma dessas opções:
  * "irrelevante" - Serviço não tem relação com a query
  * "pouco_relevante" - Serviço tem relação tangencial/indireta
  * "relevante" - Serviço está relacionado à query
  * "muito_relevante" - Serviço está fortemente relacionado
  * "match_exato" - É exatamente o que o usuário busca

- confidence_level: Use EXATAMENTE uma dessas opções:
  * "baixa" - Não tenho certeza da avaliação
  * "media" - Razoavelmente certo
  * "alta" - Muito certo da avaliação
  * "muito_alta" - Absolutamente certo

- exact_match: true APENAS se é match_exato, false caso contrário

- reasoning: explicação concisa (max 50 palavras) justificando a categoria

IMPORTANTE:
- Retornar avaliações para TODOS os %d serviços listados
- Manter a mesma ordem da lista
- Use APENAS as categorias listadas acima (copie exatamente como escrito)
- Retornar APENAS o JSON, sem texto adicional`,
		query,
		strings.Join(servicesList, "\n"),
		len(scoresToGenerate))

	content := genai.NewContentFromText(prompt, genai.RoleUser)

	resp, err := ss.geminiClient.Models.GenerateContent(ctxScore, ss.chatModel, []*genai.Content{content}, nil)
	if err != nil {
		return fmt.Errorf("erro ao chamar Gemini para batch scoring: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return fmt.Errorf("resposta vazia do Gemini para batch scoring")
	}

	// Parse JSON response
	part := resp.Candidates[0].Content.Parts[0]
	fullStr := fmt.Sprintf("%v", part)

	// Extrair JSON da resposta
	var jsonStr string
	if idx := strings.Index(fullStr, "```json"); idx != -1 {
		jsonStart := idx + len("```json")
		jsonStr = fullStr[jsonStart:]
		if endIdx := strings.Index(jsonStr, "```"); endIdx != -1 {
			jsonStr = jsonStr[:endIdx]
		}
	} else if idx := strings.Index(fullStr, "{\n"); idx != -1 {
		jsonStr = fullStr[idx:]
	} else if idx := strings.Index(fullStr, "{\"scores\""); idx != -1 {
		// Tentar encontrar início do JSON object
		jsonStr = fullStr[idx:]
	} else {
		log.Printf("No JSON found in batch scoring response: %s", fullStr)
		return fmt.Errorf("resposta do Gemini não contém JSON")
	}

	jsonStr = strings.TrimSpace(jsonStr)

	// Struct para batch response
	var batchResult struct {
		Scores []models.AIScore `json:"scores"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &batchResult); err != nil {
		log.Printf("Failed to parse batch scoring JSON: %s", jsonStr)
		return fmt.Errorf("erro ao parsear JSON do batch scoring: %w", err)
	}

	// Validar que recebemos scores para todos os documentos
	if len(batchResult.Scores) != len(scoresToGenerate) {
		log.Printf("Warning: Expected %d scores, got %d", len(scoresToGenerate), len(batchResult.Scores))
	}

	// Mapear scores aos documentos por service_id
	scoreMap := make(map[string]*models.AIScore)
	for i := range batchResult.Scores {
		score := &batchResult.Scores[i]

		// Mapear categorias para scores numéricos
		score.Relevance = mapRelevanceCategoryToScore(score.RelevanceCategory)
		score.Confidence = mapConfidenceLevelToScore(score.ConfidenceLevel)

		// Calcular final_score baseado nos scores mapeados
		// Fórmula: (relevance × confidence) com boost de 15% para exact_match
		score.FinalScore = score.Relevance * score.Confidence
		if score.ExactMatch {
			// Boost de 15% para match exato (mantendo máximo de 1.0)
			score.FinalScore = math.Min(1.0, score.FinalScore*1.15)
		}

		scoreMap[score.ServiceID] = score
	}

	// Adicionar scores ao metadata dos documentos
	for _, doc := range scoresToGenerate {
		if score, exists := scoreMap[doc.ID]; exists {
			if doc.Metadata == nil {
				doc.Metadata = make(map[string]interface{})
			}
			doc.Metadata["ai_score"] = score
		} else {
			log.Printf("Warning: No score received for document %s", doc.ID)
		}
	}

	// Re-ordenar por final_score
	for i := 0; i < len(docs); i++ {
		for j := i + 1; j < len(docs); j++ {
			scoreI := getAIFinalScore(docs[i])
			scoreJ := getAIFinalScore(docs[j])
			if scoreJ > scoreI {
				docs[i], docs[j] = docs[j], docs[i]
			}
		}
	}

	return nil
}

// mapRelevanceCategoryToScore mapeia categoria de relevância para score numérico
func mapRelevanceCategoryToScore(category string) float64 {
	// Normalizar string (lowercase, trim)
	category = strings.ToLower(strings.TrimSpace(category))

	switch category {
	case models.RelevanceIrrelevant:
		return 0.0
	case models.RelevanceLow:
		return 0.3
	case models.RelevanceModerate:
		return 0.6
	case models.RelevanceHigh:
		return 0.85
	case models.RelevanceExact:
		return 1.0
	default:
		// Fallback: tentar inferir pelo texto
		log.Printf("Warning: Unknown relevance category '%s', defaulting to 0.5", category)
		return 0.5
	}
}

// mapConfidenceLevelToScore mapeia nível de confiança para score numérico
func mapConfidenceLevelToScore(level string) float64 {
	// Normalizar string (lowercase, trim)
	level = strings.ToLower(strings.TrimSpace(level))

	switch level {
	case models.ConfidenceLow:
		return 0.5
	case models.ConfidenceMedium:
		return 0.7
	case models.ConfidenceHigh:
		return 0.9
	case models.ConfidenceVeryHigh:
		return 1.0
	default:
		// Fallback
		log.Printf("Warning: Unknown confidence level '%s', defaulting to 0.7", level)
		return 0.7
	}
}

// getAIFinalScore extrai o final_score do ai_score de um documento
func getAIFinalScore(doc *models.ServiceDocument) float64 {
	if doc.Metadata == nil {
		return 0.0
	}

	aiScoreRaw, ok := doc.Metadata["ai_score"]
	if !ok {
		return 0.0
	}

	aiScore, ok := aiScoreRaw.(*models.AIScore)
	if !ok {
		return 0.0
	}

	return aiScore.FinalScore
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

		// Adicionar scores ao metadata para filtragem posterior
		if hit.TextMatch != nil {
			doc.Metadata["text_match"] = *hit.TextMatch
		}
		if hit.VectorDistance != nil {
			doc.Metadata["vector_distance"] = *hit.VectorDistance
		}

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
		"search_content": true, // não retornar search_content bagunçado
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

// Pointer helpers
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }
func boolPtr(b bool) *bool       { return &b }

// normalizeTextMatch normaliza text_match usando log normalization
func normalizeTextMatch(score float64) float64 {
	const MAX_OBSERVED = 100000.0
	if score <= 0 {
		return 0.0
	}
	// Log normalization para melhor distribuição
	normalized := math.Log1p(score) / math.Log1p(MAX_OBSERVED)
	return math.Min(1.0, normalized)
}

// calculateRecencyFactor calcula o fator de recência baseado em last_update
// Docs atualizados nos últimos 30 dias: fator = 1.0
// Docs mais antigos: decaimento exponencial até 0.5 em ~1 ano
func calculateRecencyFactor(lastUpdateTimestamp int64) float64 {
	if lastUpdateTimestamp <= 0 {
		return 0.5 // Docs sem data recebem fator mínimo
	}

	now := time.Now().Unix()
	daysSinceUpdate := float64(now-lastUpdateTimestamp) / 86400.0 // segundos para dias

	const gracePeriodDays = 30.0 // período sem penalidade
	const lambda = 0.00207       // decay rate: ~0.5 em 365 dias após período de graça

	if daysSinceUpdate <= gracePeriodDays {
		return 1.0
	}

	daysAfterGrace := daysSinceUpdate - gracePeriodDays
	factor := math.Exp(-lambda * daysAfterGrace)

	return math.Max(0.5, factor) // mínimo de 0.5
}

// buildFilterBy constrói a expressão de filtro baseada no SearchRequest
func buildFilterBy(req *models.SearchRequest) string {
	var filters []string

	// Filtro de status (apenas publicados, a menos que include_inactive)
	if !req.IncludeInactive {
		filters = append(filters, "status:=1")
	}

	// Filtro exclude_agent_exclusive
	// Se true, exclui serviços exclusivos para agentes (mostra apenas para humanos)
	if req.ExcludeAgentExclusive != nil && *req.ExcludeAgentExclusive {
		filters = append(filters, "agents.exclusive_for_agents:=false")
	}

	if len(filters) == 0 {
		return ""
	}

	return strings.Join(filters, " && ")
}

// applyScoreThreshold filtra resultados baseado nos thresholds configurados
func (ss *SearchService) applyScoreThreshold(
	docs []*models.ServiceDocument,
	req *models.SearchRequest,
	searchType models.SearchType,
) ([]*models.ServiceDocument, map[string]interface{}) {
	// Se não há documentos, retornar vazio
	if len(docs) == 0 {
		return docs, nil
	}

	// Determinar qual threshold usar baseado no tipo de busca
	var threshold *float64
	var thresholdType string
	switch searchType {
	case models.SearchTypeKeyword:
		if req.ScoreThreshold != nil {
			threshold = req.ScoreThreshold.Keyword
		}
		thresholdType = "keyword"
	case models.SearchTypeSemantic:
		if req.ScoreThreshold != nil {
			threshold = req.ScoreThreshold.Semantic
		}
		thresholdType = "semantic"
	case models.SearchTypeHybrid:
		if req.ScoreThreshold != nil {
			threshold = req.ScoreThreshold.Hybrid
		}
		thresholdType = "hybrid"
	default:
		// AI search: não aplicar threshold (já foi aplicado na busca delegada)
		thresholdType = "none"
	}

	// Calcular alpha para hybrid
	alpha := 0.3
	if searchType == models.SearchTypeHybrid && req.Alpha > 0 && req.Alpha <= 1.0 {
		alpha = req.Alpha
	}

	// Para semantic e hybrid: calcular min/max de vector_distance para normalização
	var minVectorDist, maxVectorDist, maxSimilarity float64
	if searchType == models.SearchTypeSemantic || searchType == models.SearchTypeHybrid {
		minVectorDist = math.MaxFloat64
		maxVectorDist = -math.MaxFloat64

		for _, doc := range docs {
			var vd float64
			if vdFloat32, ok := doc.Metadata["vector_distance"].(float32); ok {
				vd = float64(vdFloat32)
			} else if vdFloat64, ok := doc.Metadata["vector_distance"].(float64); ok {
				vd = vdFloat64
			}

			if vd < minVectorDist {
				minVectorDist = vd
			}
			if vd > maxVectorDist {
				maxVectorDist = vd
			}
		}

		// Similarity absoluta do melhor resultado (menor distance)
		maxSimilarity = 1.0 - (minVectorDist / 2.0)
	}

	// Processar cada documento, calcular scores e aplicar threshold
	originalCount := len(docs)
	filtered := make([]*models.ServiceDocument, 0, len(docs))

	for _, doc := range docs {
		var normalizedScore float64
		passes := true // Por padrão, passa (se não houver threshold)
		scoreInfo := &models.ScoreInfo{
			ThresholdApplied: thresholdType,
			PassedThreshold:  true,
		}

		if threshold != nil {
			scoreInfo.ThresholdValue = threshold
		}

		switch searchType {
		case models.SearchTypeKeyword:
			// Para keyword: normalizar text_match usando log normalization
			// text_match do Typesense são valores absolutos unbounded (podem ser bilhões/trilhões)
			var tm float64
			if tmInt, ok := doc.Metadata["text_match"].(int64); ok {
				tm = float64(tmInt)
			} else if tmFloat, ok := doc.Metadata["text_match"].(float64); ok {
				tm = tmFloat
			}

			// Log normalization para melhor distribuição
			normalizedScore = normalizeTextMatch(tm)
			scoreInfo.TextMatchNormalized = &normalizedScore

			if threshold != nil {
				passes = normalizedScore >= *threshold
			}

		case models.SearchTypeSemantic:
			// Para semantic: normalização onde pior = 0, melhor = similarity absoluta
			var vd float64
			if vdFloat32, ok := doc.Metadata["vector_distance"].(float32); ok {
				vd = float64(vdFloat32)
			} else if vdFloat64, ok := doc.Metadata["vector_distance"].(float64); ok {
				vd = vdFloat64
			}

			// Normalização: pior resultado = 0, melhor resultado = maxSimilarity (valor absoluto)
			var similarity float64
			if maxVectorDist > minVectorDist {
				proportion := 1.0 - ((vd - minVectorDist) / (maxVectorDist - minVectorDist))
				similarity = proportion * maxSimilarity
			} else {
				// Todos os resultados têm a mesma distance (edge case)
				similarity = maxSimilarity
			}
			similarity = math.Max(0.0, math.Min(maxSimilarity, similarity))
			scoreInfo.VectorSimilarity = &similarity
			normalizedScore = similarity

			if threshold != nil {
				passes = normalizedScore >= *threshold
			}

		case models.SearchTypeHybrid:
			// Para hybrid: combinar text_match normalizado com vector similarity (min-max)
			var textScore, vectorScore float64

			// Extrair e normalizar text_match (log normalization)
			var tm float64
			if tmInt, ok := doc.Metadata["text_match"].(int64); ok {
				tm = float64(tmInt)
			} else if tmFloat, ok := doc.Metadata["text_match"].(float64); ok {
				tm = tmFloat
			}

			textScore = normalizeTextMatch(tm)
			scoreInfo.TextMatchNormalized = &textScore

			// Extrair e normalizar vector_distance: pior = 0, melhor = maxSimilarity
			var vd float64
			if vdFloat32, ok := doc.Metadata["vector_distance"].(float32); ok {
				vd = float64(vdFloat32)
			} else if vdFloat64, ok := doc.Metadata["vector_distance"].(float64); ok {
				vd = vdFloat64
			}

			// Normalização: pior resultado = 0, melhor resultado = maxSimilarity (valor absoluto)
			if maxVectorDist > minVectorDist {
				proportion := 1.0 - ((vd - minVectorDist) / (maxVectorDist - minVectorDist))
				vectorScore = proportion * maxSimilarity
			} else {
				// Todos os resultados têm a mesma distance (edge case)
				vectorScore = maxSimilarity
			}
			vectorScore = math.Max(0.0, math.Min(maxSimilarity, vectorScore))
			scoreInfo.VectorSimilarity = &vectorScore

			// Calcular score híbrido: alpha*text + (1-alpha)*vector (fórmula corrigida)
			hybridScore := alpha*textScore + (1.0-alpha)*vectorScore
			scoreInfo.HybridScore = &hybridScore
			normalizedScore = hybridScore

			if threshold != nil {
				passes = normalizedScore >= *threshold
			}
		}

		scoreInfo.PassedThreshold = passes

		// Aplicar recency boost se habilitado
		finalScore := normalizedScore
		if req.RecencyBoost {
			recencyFactor := calculateRecencyFactor(doc.UpdatedAt)
			scoreInfo.RecencyFactor = &recencyFactor
			finalScore = normalizedScore * recencyFactor
			scoreInfo.FinalScore = &finalScore
		}

		// Adicionar ScoreInfo ao metadata do documento
		if doc.Metadata == nil {
			doc.Metadata = make(map[string]interface{})
		}
		doc.Metadata["score_info"] = scoreInfo

		// Limpar metadata poluída (remover campos internos)
		delete(doc.Metadata, "text_match")
		delete(doc.Metadata, "vector_distance")

		// Aplicar filtro se threshold está configurado
		if threshold == nil || passes {
			filtered = append(filtered, doc)
		}
	}

	// Se recency boost está habilitado, reordenar por final_score
	if req.RecencyBoost && len(filtered) > 1 {
		sort.Slice(filtered, func(i, j int) bool {
			scoreI := getFinalScoreFromMetadata(filtered[i])
			scoreJ := getFinalScoreFromMetadata(filtered[j])
			return scoreI > scoreJ
		})
	}

	// Metadata sobre a filtragem (só incluir se threshold foi aplicado)
	var filterMeta map[string]interface{}
	if threshold != nil {
		filterMeta = map[string]interface{}{
			"score_threshold_applied": true,
			"original_count":          originalCount,
			"filtered_count":          len(filtered),
			"threshold_value":         *threshold,
			"search_type":             string(searchType),
		}
	}

	if req.RecencyBoost {
		if filterMeta == nil {
			filterMeta = make(map[string]interface{})
		}
		filterMeta["recency_boost_applied"] = true
	}

	return filtered, filterMeta
}

// getFinalScoreFromMetadata extrai o final_score do metadata do documento
func getFinalScoreFromMetadata(doc *models.ServiceDocument) float64 {
	if doc.Metadata == nil {
		return 0
	}
	if scoreInfo, ok := doc.Metadata["score_info"].(*models.ScoreInfo); ok && scoreInfo.FinalScore != nil {
		return *scoreInfo.FinalScore
	}
	return 0
}
