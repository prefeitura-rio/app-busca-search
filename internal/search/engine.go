package search

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search/adapter"
	"github.com/prefeitura-rio/app-busca-search/internal/search/query"
	"github.com/prefeitura-rio/app-busca-search/internal/search/ranking"
	"github.com/prefeitura-rio/app-busca-search/internal/search/synonyms"
)

// Engine é o motor de busca v3
type Engine struct {
	typesense         *adapter.TypesenseAdapter
	gemini            *adapter.GeminiAdapter
	parser            *query.Parser
	expander          *query.Expander
	analyzer          *query.Analyzer
	synonymService    *synonyms.Service
	reranker          *ranking.Reranker
	popularity        ranking.PopularityProvider
	cache             *SearchCache
	collectionConfigs map[string]*v3.CollectionConfig
	defaultCollection string
}

// NewEngine cria um novo motor de busca
func NewEngine(
	typesense *adapter.TypesenseAdapter,
	gemini *adapter.GeminiAdapter,
	synonymService *synonyms.Service,
	popularity ranking.PopularityProvider,
	collectionConfigs map[string]*v3.CollectionConfig,
	defaultCollection string,
) *Engine {
	var analyzerInstance *query.Analyzer
	var rerankerInstance *ranking.Reranker
	
	if gemini != nil && gemini.IsAvailable() {
		analyzerInstance = query.NewAnalyzer(gemini.GetClient(), gemini.GetChatModel())
		rerankerInstance = ranking.NewReranker(gemini.GetClient(), gemini.GetChatModel())
	}

	return &Engine{
		typesense:         typesense,
		gemini:            gemini,
		parser:            query.NewParser(),
		expander:          query.NewExpander(synonymService, 5),
		analyzer:          analyzerInstance,
		synonymService:    synonymService,
		reranker:          rerankerInstance,
		popularity:        popularity,
		cache:             NewSearchCache(2*time.Minute, 500),
		collectionConfigs: collectionConfigs,
		defaultCollection: defaultCollection,
	}
}

// Search executa a busca baseada na requisição
func (e *Engine) Search(ctx context.Context, req *v3.SearchRequest) (*v3.SearchResponse, error) {
	startTime := time.Now()
	timing := &v3.TimingMeta{}

	// Valida request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Verifica cache (exceto para busca AI que tem análise dinâmica)
	var cacheKey string
	if req.Type != v3.SearchTypeAI && e.cache != nil {
		cacheKey = e.cache.GenerateKey(req)
		if cached := e.cache.Get(cacheKey); cached != nil {
			return cached, nil
		}
	}

	// Configura busca baseada no modo
	config := v3.ConfigForMode(req.Mode)
	config.ApplyRequest(req)

	// Determina collections
	collections := e.resolveCollections(req.ParsedCollections)
	if len(collections) == 0 {
		return nil, ErrNoCollections
	}

	// 1. Parse da query
	parseStart := time.Now()
	parsed := e.parser.Parse(req.Query)
	timing.ParsingMs = float64(time.Since(parseStart).Microseconds()) / 1000

	// 2. Expande query (se habilitado)
	var expanded *query.ExpandedQuery
	if config.EnableExpansion {
		expanded = e.expander.ExpandSimple(parsed)
	} else {
		// Usa tokens (sem stopwords) para QueryString, não Normalized
		queryStr := strings.Join(parsed.Tokens, " ")
		if queryStr == "" {
			queryStr = "*"
		}
		expanded = &query.ExpandedQuery{
			Original:      parsed.Original,
			Normalized:    parsed.Normalized,
			Tokens:        parsed.Tokens,
			ExpandedTerms: parsed.Tokens,
			QueryString:   queryStr,
		}
	}

	// 3. Gera embedding e analisa query (em paralelo para AI search)
	var embedding []float32
	var embeddingErr error
	var aiAnalysis *v3.AIAnalysis

	needsEmbedding := req.Type == v3.SearchTypeSemantic || req.Type == v3.SearchTypeHybrid || req.Type == v3.SearchTypeAI
	needsAnalysis := req.Type == v3.SearchTypeAI && e.analyzer != nil

	// Verifica disponibilidade do Gemini
	if needsEmbedding && (e.gemini == nil || !e.gemini.IsAvailable()) {
		if req.Type == v3.SearchTypeSemantic {
			return nil, ErrEmbeddingService
		}
		// Fallback para keyword em hybrid/AI
		req.Type = v3.SearchTypeKeyword
		needsEmbedding = false
		needsAnalysis = false
	}

	// Executa embedding e análise em paralelo (otimiza AI search)
	if needsEmbedding || needsAnalysis {
		var wg sync.WaitGroup
		embedStart := time.Now()

		if needsEmbedding {
			wg.Add(1)
			go func() {
				defer wg.Done()
				embedding, embeddingErr = e.gemini.GenerateEmbedding(ctx, expanded.QueryString)
			}()
		}

		if needsAnalysis {
			wg.Add(1)
			go func() {
				defer wg.Done()
				analysis, _ := e.analyzer.Analyze(ctx, req.Query)
				if analysis != nil {
					aiAnalysis = &v3.AIAnalysis{
						Intent:         analysis.Intent,
						Keywords:       analysis.Keywords,
						Categories:     analysis.Categories,
						RefinedQueries: analysis.RefinedQueries,
						SearchStrategy: analysis.SearchStrategy,
						Confidence:     analysis.Confidence,
					}
				}
			}()
		}

		wg.Wait()
		timing.EmbeddingMs = float64(time.Since(embedStart).Microseconds()) / 1000

		// Trata erro de embedding
		if embeddingErr != nil {
			if req.Type == v3.SearchTypeSemantic {
				return nil, fmt.Errorf("%w: %v", ErrEmbeddingService, embeddingErr)
			}
			// Fallback para keyword
			req.Type = v3.SearchTypeKeyword
		}

		// Usa categorias inferidas como filtro se não especificada
		if aiAnalysis != nil && req.Category == "" && len(aiAnalysis.Categories) > 0 {
			if aiAnalysis.Confidence >= 0.7 {
				req.Category = aiAnalysis.Categories[0]
			}
		}
	}

	// 4. Executa busca
	searchStart := time.Now()
	
	// Prepara filtros
	filters := make(map[string]interface{})
	if req.Status != nil {
		filters["status"] = *req.Status
	} else {
		filters["status"] = 1 // Default: apenas publicados
	}
	if req.Category != "" {
		filters["tema_geral"] = req.Category
	}
	if req.SubCategory != "" {
		filters["subtema"] = req.SubCategory
	}
	if req.OrgaoGestor != "" {
		filters["orgao_gestor"] = req.OrgaoGestor
	}
	if req.TempoMax != "" {
		filters["prazo"] = req.TempoMax
	}
	if req.IsFree != nil {
		filters["gratuito"] = *req.IsFree
	}
	if req.HasDigital != nil {
		filters["canal_digital"] = *req.HasDigital
	}

	// Busca em todas as collections
	allDocs := make([]v3.Document, 0)
	totalFound := 0

	results, err := e.typesense.MultiCollectionSearch(
		ctx,
		collections,
		expanded.QueryString,
		embedding,
		config,
		req.Type,
		req.Page,
		req.PerPage,
		filters,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTypesenseFailed, err)
	}

	timing.SearchMs = float64(time.Since(searchStart).Microseconds()) / 1000

	// 5. Processa resultados de cada collection
	rankStart := time.Now()
	
	for collName, result := range results {
		if result == nil {
			continue
		}
		
		totalFound += result.TotalFound
		collConfig := e.collectionConfigs[collName]

		// Prepara normalização para scores vetoriais
		scorer := ranking.NewScorerWithPopularity(config, e.popularity)
		scorer.PrepareNormalization(result.Hits)

		// Transforma hits em documentos (usa query para boost de título)
		for _, hit := range result.Hits {
			score := scorer.CalculateWithQuery(hit, req.Type, expanded.QueryString)
			
			doc := e.transformDocument(hit, collName, collConfig, score)
			allDocs = append(allDocs, doc)
		}
	}

	// 6. Ordena por score
	scorer := ranking.NewScorerWithPopularity(config, e.popularity)
	scorer.RankDocuments(allDocs)

	// 7. Aplica threshold (usa o score apropriado por tipo de busca)
	if config.MinScore > 0 {
		allDocs = scorer.FilterByThreshold(allDocs, config.MinScore, req.Type)
	}

	// 8. Re-rank com LLM (apenas para AI search, topN=5 para melhor performance)
	if req.Type == v3.SearchTypeAI && e.reranker != nil && len(allDocs) > 0 {
		reranked, _ := e.reranker.Rerank(ctx, req.Query, allDocs, 5)
		if reranked != nil {
			allDocs = reranked
		}
	}

	timing.RankingMs = float64(time.Since(rankStart).Microseconds()) / 1000
	timing.TotalMs = float64(time.Since(startTime).Microseconds()) / 1000

	// Monta resposta
	response := &v3.SearchResponse{
		Results:    allDocs,
		Pagination: v3.NewPagination(req.Page, req.PerPage, totalFound),
		Query: v3.QueryMeta{
			Original:   req.Query,
			Normalized: expanded.Normalized,
			Expanded:   expanded.ExpandedTerms,
		},
		Timing:     *timing,
		AIAnalysis: aiAnalysis,
	}

	// Armazena no cache (exceto busca AI)
	if cacheKey != "" && e.cache != nil {
		e.cache.Set(cacheKey, response)
	}

	return response, nil
}

// resolveCollections determina quais collections buscar
func (e *Engine) resolveCollections(requested []string) []string {
	if len(requested) > 0 {
		// Valida collections solicitadas
		valid := make([]string, 0)
		for _, c := range requested {
			if _, ok := e.collectionConfigs[c]; ok {
				valid = append(valid, c)
			}
		}
		if len(valid) > 0 {
			return valid
		}
	}

	// Retorna todas as collections configuradas
	collections := make([]string, 0, len(e.collectionConfigs))
	for name := range e.collectionConfigs {
		collections = append(collections, name)
	}

	// Se não há nenhuma, usa default
	if len(collections) == 0 && e.defaultCollection != "" {
		return []string{e.defaultCollection}
	}

	return collections
}

// transformDocument transforma um hit em documento v3
func (e *Engine) transformDocument(
	hit ranking.Hit,
	collection string,
	config *v3.CollectionConfig,
	score *ranking.ScoreResult,
) v3.Document {
	doc := v3.Document{
		ID:         getString(hit.Document, "id"),
		Collection: collection,
		Type:       "service",
		Data:       hit.Document,
		Score: v3.ScoreInfo{
			Final:      score.FinalScore,
			Text:       score.TextScore,
			Vector:     score.VectorScore,
			Hybrid:     score.HybridScore,
			Recency:    score.RecencyScore,
			Popularity: score.PopularityScore,
		},
	}

	if config != nil {
		doc.Type = config.Type
		doc.Title = getString(hit.Document, config.TitleField)
		doc.Description = getString(hit.Document, config.DescField)
		if config.CategoryField != "" {
			doc.Category = getString(hit.Document, config.CategoryField)
		}
		if config.SlugField != "" {
			doc.Slug = getString(hit.Document, config.SlugField)
		}
	} else {
		// Fallback para campos padrão
		doc.Title = getString(hit.Document, "nome_servico")
		if doc.Title == "" {
			doc.Title = getString(hit.Document, "title")
		}
		doc.Description = getString(hit.Document, "resumo")
		if doc.Description == "" {
			doc.Description = getString(hit.Document, "description")
		}
		doc.Category = getString(hit.Document, "tema_geral")
		doc.Slug = getString(hit.Document, "slug")
	}

	// Remove campos sensíveis do Data
	delete(doc.Data, "embedding")
	delete(doc.Data, "search_content")

	return doc
}

// GetDocument busca um documento por ID, tentando nas collections configuradas
func (e *Engine) GetDocument(ctx context.Context, id string, collectionHint string) (*v3.Document, error) {
	// Tenta na collection hint primeiro
	if collectionHint != "" {
		if doc, err := e.tryGetDocument(ctx, collectionHint, id); err == nil {
			return doc, nil
		}
	}

	// Tenta na collection default
	if e.defaultCollection != "" && e.defaultCollection != collectionHint {
		if doc, err := e.tryGetDocument(ctx, e.defaultCollection, id); err == nil {
			return doc, nil
		}
	}

	// Tenta em todas as collections configuradas
	for collName := range e.collectionConfigs {
		if collName == collectionHint || collName == e.defaultCollection {
			continue
		}
		if doc, err := e.tryGetDocument(ctx, collName, id); err == nil {
			return doc, nil
		}
	}

	return nil, fmt.Errorf("documento nao encontrado em nenhuma collection")
}

// tryGetDocument tenta buscar documento em uma collection especifica
func (e *Engine) tryGetDocument(ctx context.Context, collection, id string) (*v3.Document, error) {
	raw, err := e.typesense.GetDocument(ctx, collection, id)
	if err != nil {
		return nil, err
	}

	collConfig := e.collectionConfigs[collection]
	return e.transformRawDocument(raw, collection, collConfig), nil
}

// transformRawDocument converte um documento raw para v3.Document
func (e *Engine) transformRawDocument(raw map[string]interface{}, collection string, config *v3.CollectionConfig) *v3.Document {
	doc := &v3.Document{
		ID:         getString(raw, "id"),
		Collection: collection,
		Type:       "service",
		Data:       raw,
		Score:      v3.ScoreInfo{}, // Documento direto, sem score
	}

	if config != nil {
		doc.Type = config.Type
		doc.Title = getString(raw, config.TitleField)
		doc.Description = getString(raw, config.DescField)
		if config.CategoryField != "" {
			doc.Category = getString(raw, config.CategoryField)
		}
		if config.SlugField != "" {
			doc.Slug = getString(raw, config.SlugField)
		}
	} else {
		// Fallback para campos padrao
		doc.Title = getString(raw, "nome_servico")
		if doc.Title == "" {
			doc.Title = getString(raw, "title")
		}
		doc.Description = getString(raw, "resumo")
		if doc.Description == "" {
			doc.Description = getString(raw, "description")
		}
		doc.Category = getString(raw, "tema_geral")
		doc.Slug = getString(raw, "slug")
	}

	// Remove campos sensiveis do Data
	delete(doc.Data, "embedding")
	delete(doc.Data, "search_content")

	return doc
}

// LoadSynonyms carrega sinonimos padrao
func (e *Engine) LoadSynonyms(ctx context.Context) error {
	if e.synonymService == nil {
		return nil
	}
	return e.synonymService.LoadDefaults(ctx)
}

// getString extrai string de um mapa
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ParseCollections converte string comma-separated em slice
func ParseCollections(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
