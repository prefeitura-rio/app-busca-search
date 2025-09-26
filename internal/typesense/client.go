package typesense

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/prefeitura-rio/app-busca-search/internal/constants"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	"github.com/prefeitura-rio/app-busca-search/internal/utils"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
	"google.golang.org/genai"
)

type Client struct {
	client *typesense.Client
	geminiClient *genai.Client
	embeddingModel string
	relevanciaService *services.RelevanciaService
	filterService *services.FilterService
}

func NewClient(cfg *config.Config) *Client {
	typesenseClient := typesense.NewClient(
		typesense.WithServer(fmt.Sprintf("%s://%s:%s", cfg.TypesenseProtocol, cfg.TypesenseHost, cfg.TypesensePort)),
		typesense.WithAPIKey(cfg.TypesenseAPIKey),
	)

	ctx := context.Background()
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.GeminiAPIKey,
	})
	
	if err != nil {
		fmt.Printf("Erro ao inicializar cliente Gemini: %v\n", err)
		geminiClient = nil
	}

	// Inicializa o serviço de relevância
	relevanciaConfig := &models.RelevanciaConfig{
		CaminhoArquivo1746:          cfg.RelevanciaArquivo1746,
		CaminhoArquivoCariocaDigital: cfg.RelevanciaArquivoCariocaDigital,
		IntervaloAtualizacao:         cfg.RelevanciaIntervaloAtualizacao,
	}
	
	relevanciaService := services.NewRelevanciaService(relevanciaConfig)

	// Inicializa o serviço de filtro
	filterService := services.NewFilterService(cfg.FilterCSVPath)

	return &Client{
		client: typesenseClient,
		geminiClient: geminiClient,
		embeddingModel: cfg.GeminiEmbeddingModel,
		relevanciaService: relevanciaService,
		filterService: filterService,
	}
}

func (c *Client) GerarEmbedding(ctx context.Context, texto string) ([]float32, error) {
	if c.geminiClient == nil {
		return nil, fmt.Errorf("cliente Gemini não inicializado")
	}
	
	content := genai.NewContentFromText(texto, genai.RoleUser)
	
	config := &genai.EmbedContentConfig{}
	
	resp, err := c.geminiClient.Models.EmbedContent(ctx, c.embeddingModel, []*genai.Content{content}, config)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar embedding: %v", err)
	}
	
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("nenhum embedding foi gerado")
	}

	return resp.Embeddings[0].Values, nil
}

func (c *Client) BuscaMultiColecaoComTexto(ctx context.Context, colecoes []string, query string, pagina int, porPagina int) (map[string]interface{}, error) {
	vetor, err := c.GerarEmbedding(ctx, query)
	if err != nil {
		return c.BuscaMultiColecao(colecoes, query, pagina, porPagina, nil)
	}
	
	return c.BuscaMultiColecao(colecoes, query, pagina, porPagina, vetor)
}

func (c *Client) BuscaMultiColecao(colecoes []string, query string, pagina int, porPagina int, vetor []float32) (map[string]interface{}, error) {
	ctx := context.Background()
	queryStr := query
	queryByStr := "search_content,titulo,descricao"
	includeFields := "*"
	excludeFields := "embedding"
	var vectorQuery *string
	if len(vetor) > 0 {
		vectorStr := "["
		for i, val := range vetor {
			if i > 0 {
				vectorStr += ", "
			}
			vectorStr += fmt.Sprintf("%.6f", val)
		}
		vectorStr += "]"
		
		alpha := 0.3
		vq := fmt.Sprintf("embedding:(%s, alpha:%.1f)", vectorStr, alpha)
		vectorQuery = &vq
	}
	
	searches := make([]api.MultiSearchCollectionParameters, 0, len(colecoes))
	
	for _, colecao := range colecoes {
		colecaoStr := colecao
		colecaoPtr := &colecaoStr
		collectionParams := api.MultiSearchCollectionParameters{
			Collection:    colecaoPtr,
			Q:             &queryStr,
			QueryBy:       &queryByStr,
			Page:          &pagina,
			PerPage:       &porPagina,
			IncludeFields: &includeFields,
			ExcludeFields: &excludeFields,
		}
		
		if vectorQuery != nil {
			collectionParams.VectorQuery = vectorQuery
		}
		
		searches = append(searches, collectionParams)
	}
	
	searchesParam := api.MultiSearchSearchesParameter{
		Searches: searches,
	}
	
	searchResult, err := c.client.MultiSearch.Perform(ctx, &api.MultiSearchParams{}, searchesParam)
	if err != nil {
		return nil, err
	}

	type hitWrapper struct {
		textMatch      int64
		vectorDistance float64
		raw            map[string]interface{}
	}

	var allHits []hitWrapper
	totalFound := 0

	for _, res := range searchResult.Results {
		if res.Found != nil {
			totalFound += int(*res.Found)
		}
		if res.Hits == nil {
			continue
		}
		for _, h := range *res.Hits {
			hb, _ := json.Marshal(h)
			var hMap map[string]interface{}
			_ = json.Unmarshal(hb, &hMap)

			var tm int64
			if v, ok := hMap["text_match"].(float64); ok {
				tm = int64(v)
			}

			var vd float64 = 999999.0
			if v, ok := hMap["vector_distance"].(float64); ok {
				vd = v
			}

			allHits = append(allHits, hitWrapper{
				textMatch:      tm,
				vectorDistance: vd,
				raw:            hMap,
			})
		}
	}

	sort.Slice(allHits, func(i, j int) bool {
		if allHits[i].textMatch == allHits[j].textMatch {
			return allHits[i].vectorDistance < allHits[j].vectorDistance
		}
		return allHits[i].textMatch > allHits[j].textMatch
	})

	// Aplica filtro nos resultados ordenados, removendo documentos da carioca-digital que estão no CSV
	filteredHits := make([]hitWrapper, 0, len(allHits))
	for _, hw := range allHits {
		shouldKeep := true
		
		// Verifica se é da collection carioca-digital e se deve ser excluído
		if document, ok := hw.raw["document"].(map[string]interface{}); ok {
			if id, ok := document["id"].(string); ok {
				// Para busca multi-collection, assumimos que documentos com IDs no CSV são da carioca-digital
				if c.filterService.ShouldExclude(id) {
					shouldKeep = false
				}
			}
		}
		
		if shouldKeep {
			filteredHits = append(filteredHits, hw)
		}
	}
	allHits = filteredHits

	startIdx := (pagina - 1) * porPagina
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + porPagina
	if endIdx > len(allHits) {
		endIdx = len(allHits)
	}

	if startIdx > len(allHits) {
		startIdx = len(allHits)
	}

	count := 0
	if endIdx > startIdx {
		count = endIdx - startIdx
	}

	pagedHits := make([]map[string]interface{}, 0, count)
	if count > 0 {
		for _, hw := range allHits[startIdx:endIdx] {
			pagedHits = append(pagedHits, hw.raw)
		}
	}

	resp := map[string]interface{}{
		"found":   totalFound,
		"out_of":  totalFound,
		"page":    pagina,
		"hits":    pagedHits,
	}

	return resp, nil
}

// BuscaPorCategoriaMultiColecao busca documentos por categoria em múltiplas coleções retornando informações completas
func (c *Client) BuscaPorCategoriaMultiColecao(colecoes []string, categoria string, pagina int, porPagina int) (map[string]interface{}, error) {
	ctx := context.Background()
	filterBy := fmt.Sprintf("category:=%s", categoria)
	includeFields := "*"
	excludeFields := "embedding"
	
	// Wrapper para hits com relevância
	type hitWithRelevance struct {
		relevancia int
		hit        map[string]interface{}
	}

	// Combina todos os resultados das coleções e adiciona relevância
	var allHitsWithRelevance []hitWithRelevance
	totalFound := 0

	// Para cada coleção, busca todos os resultados com paginação
	for _, colecao := range colecoes {
		page := 1
		perPageLimit := 250 // Máximo permitido pelo Typesense
		
		for {
			searchParams := &api.SearchCollectionParams{
				Q:             stringPtr("*"),
				FilterBy:      &filterBy,
				Page:          intPtr(page),
				PerPage:       intPtr(perPageLimit),
				IncludeFields: &includeFields,
				ExcludeFields: &excludeFields,
			}

			searchResult, err := c.client.Collection(colecao).Documents().Search(ctx, searchParams)
			if err != nil {
				// Se é erro 404 (coleção não encontrada), pula para próxima coleção
				if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
					log.Printf("Coleção %s não encontrada, pulando para próxima coleção", colecao)
					break // Sai do loop interno para ir para próxima coleção
				}
				// Log do erro mas continua com próxima coleção
				log.Printf("Erro ao buscar na coleção %s: %v", colecao, err)
				break // Sai do loop interno para ir para próxima coleção
			}

			var resultMap map[string]interface{}
			jsonData, err := json.Marshal(searchResult)
			if err != nil {
				log.Printf("Erro ao serializar resultado da coleção %s: %v", colecao, err)
				break // Sai do loop interno para ir para próxima coleção
			}
			
			if err := json.Unmarshal(jsonData, &resultMap); err != nil {
				log.Printf("Erro ao deserializar resultado da coleção %s: %v", colecao, err)
				break // Sai do loop interno para ir para próxima coleção
			}

			// Captura o total encontrado na primeira página
			if page == 1 {
				if found, ok := resultMap["found"].(float64); ok {
					totalFound += int(found)
				}
			}

			hitsCount := 0
			if hits, ok := resultMap["hits"].([]interface{}); ok {
				hitsCount = len(hits)
				for _, h := range hits {
					if hitMap, ok := h.(map[string]interface{}); ok {
						// Obtém relevância baseada no título
						relevancia := 0
						if document, ok := hitMap["document"].(map[string]interface{}); ok {
							if titulo, ok := document["titulo"].(string); ok {
								relevancia = c.relevanciaService.ObterRelevancia(titulo)
							}
						}
						
						allHitsWithRelevance = append(allHitsWithRelevance, hitWithRelevance{
							relevancia: relevancia,
							hit:        hitMap,
						})
					}
				}
			}
			
			// Se retornou menos que perPageLimit, chegamos ao fim desta coleção
			if hitsCount < perPageLimit {
				break
			}
			
			page++
		}
	}

	// Ordena por relevância (maior relevância primeiro)
	sort.Slice(allHitsWithRelevance, func(i, j int) bool {
		return allHitsWithRelevance[i].relevancia > allHitsWithRelevance[j].relevancia
	})

	// Aplica filtro nos resultados ordenados, removendo documentos da carioca-digital que estão no CSV
	filteredHitsWithRelevance := make([]hitWithRelevance, 0, len(allHitsWithRelevance))
	for _, hitWithRel := range allHitsWithRelevance {
		shouldKeep := true
		
		// Verifica se deve ser excluído
		if document, ok := hitWithRel.hit["document"].(map[string]interface{}); ok {
			if id, ok := document["id"].(string); ok {
				if c.filterService.ShouldExclude(id) {
					shouldKeep = false
				}
			}
		}
		
		if shouldKeep {
			filteredHitsWithRelevance = append(filteredHitsWithRelevance, hitWithRel)
		}
	}
	allHitsWithRelevance = filteredHitsWithRelevance

	// Paginação manual dos resultados combinados
	startIdx := (pagina - 1) * porPagina
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + porPagina
	if endIdx > len(allHitsWithRelevance) {
		endIdx = len(allHitsWithRelevance)
	}

	if startIdx > len(allHitsWithRelevance) {
		startIdx = len(allHitsWithRelevance)
	}

	count := 0
	if endIdx > startIdx {
		count = endIdx - startIdx
	}

	pagedHits := make([]map[string]interface{}, 0, count)
	if count > 0 {
		for _, hitWithRel := range allHitsWithRelevance[startIdx:endIdx] {
			pagedHits = append(pagedHits, hitWithRel.hit)
		}
	}

	resp := map[string]interface{}{
		"found":   totalFound,
		"out_of":  totalFound,
		"page":    pagina,
		"hits":    pagedHits,
	}

	return resp, nil
}

// BuscaPorCategoria busca documentos por categoria retornando informações completas
func (c *Client) BuscaPorCategoria(colecao string, categoria string, pagina int, porPagina int) (map[string]interface{}, error) {
	ctx := context.Background()
	filterBy := fmt.Sprintf("category:=%s", categoria)
	includeFields := "*"
	excludeFields := "embedding"
	
	// Wrapper para hits com relevância
	type hitWithRelevance struct {
		relevancia int
		hit        map[string]interface{}
	}

	// Extrai hits e adiciona relevância
	var allHitsWithRelevance []hitWithRelevance
	totalFound := 0
	
	// Busca todos os resultados com paginação para não ultrapassar limite do Typesense
	page := 1
	perPageLimit := 250 // Máximo permitido pelo Typesense
	
	for {
		searchParams := &api.SearchCollectionParams{
			Q:             stringPtr("*"),
			FilterBy:      &filterBy,
			Page:          intPtr(page),
			PerPage:       intPtr(perPageLimit),
			IncludeFields: &includeFields,
			ExcludeFields: &excludeFields,
		}

		searchResult, err := c.client.Collection(colecao).Documents().Search(ctx, searchParams)
		if err != nil {
			return nil, err
		}

		var resultMap map[string]interface{}
		jsonData, err := json.Marshal(searchResult)
		if err != nil {
			return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
		}
		
		if err := json.Unmarshal(jsonData, &resultMap); err != nil {
			return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
		}

		// Captura o total encontrado na primeira página
		if page == 1 {
			if found, ok := resultMap["found"].(float64); ok {
				totalFound = int(found)
			}
		}

		hitsCount := 0
		if hits, ok := resultMap["hits"].([]interface{}); ok {
			hitsCount = len(hits)
			for _, h := range hits {
				if hitMap, ok := h.(map[string]interface{}); ok {
					// Obtém relevância baseada no título
					relevancia := 0
					if document, ok := hitMap["document"].(map[string]interface{}); ok {
						if titulo, ok := document["titulo"].(string); ok {
							relevancia = c.relevanciaService.ObterRelevancia(titulo)
						}
					}
					
					allHitsWithRelevance = append(allHitsWithRelevance, hitWithRelevance{
						relevancia: relevancia,
						hit:        hitMap,
					})
				}
			}
		}
		
		// Se retornou menos que perPageLimit, chegamos ao fim
		if hitsCount < perPageLimit {
			break
		}
		
		page++
	}

	// Ordena por relevância (maior relevância primeiro)
	sort.Slice(allHitsWithRelevance, func(i, j int) bool {
		return allHitsWithRelevance[i].relevancia > allHitsWithRelevance[j].relevancia
	})

	// Aplica filtro nos resultados ordenados se for collection carioca-digital
	if colecao == "carioca-digital" {
		filteredHitsWithRelevance := make([]hitWithRelevance, 0, len(allHitsWithRelevance))
		for _, hitWithRel := range allHitsWithRelevance {
			shouldKeep := true
			
			// Verifica se deve ser excluído
			if document, ok := hitWithRel.hit["document"].(map[string]interface{}); ok {
				if id, ok := document["id"].(string); ok {
					if c.filterService.ShouldExclude(id) {
						shouldKeep = false
					}
				}
			}
			
			if shouldKeep {
				filteredHitsWithRelevance = append(filteredHitsWithRelevance, hitWithRel)
			}
		}
		allHitsWithRelevance = filteredHitsWithRelevance
	}

	// Paginação manual dos resultados ordenados
	startIdx := (pagina - 1) * porPagina
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + porPagina
	if endIdx > len(allHitsWithRelevance) {
		endIdx = len(allHitsWithRelevance)
	}

	if startIdx > len(allHitsWithRelevance) {
		startIdx = len(allHitsWithRelevance)
	}

	count := 0
	if endIdx > startIdx {
		count = endIdx - startIdx
	}

	pagedHits := make([]interface{}, 0, count)
	if count > 0 {
		for _, hitWithRel := range allHitsWithRelevance[startIdx:endIdx] {
			pagedHits = append(pagedHits, hitWithRel.hit)
		}
	}

	// Reconstrói o resultado com paginação ordenada
	finalResultMap := map[string]interface{}{
		"hits":  pagedHits,
		"found": totalFound,
		"page":  pagina,
	}
	
	return finalResultMap, nil
}

// BuscaPorID busca um documento específico por ID retornando todos os campos exceto embedding e normalizados
func (c *Client) BuscaPorID(colecao string, documentoID string) (map[string]interface{}, error) {
	ctx := context.Background()
	
	document, err := c.client.Collection(colecao).Document(documentoID).Retrieve(ctx)
	if err != nil {
		return nil, err
	}

	var resultMap map[string]interface{}
	jsonData, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}
	
	if err := json.Unmarshal(jsonData, &resultMap); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	// Remove o campo embedding do resultado
	delete(resultMap, "embedding")

	return resultMap, nil
}

// BuscarCategoriasRelevancia busca todas as categorias e calcula sua relevância baseada na volumetria dos serviços
func (c *Client) BuscarCategoriasRelevancia(colecoes []string) (*models.CategoriasRelevanciaResponse, error) {
	ctx := context.Background()
	
	// Mapa para acumular relevância por categoria
	categoriasMap := make(map[string]*models.CategoriaRelevancia)
	
	// Inicializa todas as categorias válidas com valores zerados
	for _, categoria := range constants.CategoriasValidas {
		categoriasMap[categoria] = &models.CategoriaRelevancia{
			Nome:               categoria,
			RelevanciaTotal:    0,
			QuantidadeServicos: 0,
			RelevanciaMedia:    0.0,
		}
	}
	
	// Para cada coleção, busca todas as categorias
	for _, colecao := range colecoes {
		// Busca usando facet para obter categorias únicas
		searchParams := &api.SearchCollectionParams{
			Q:         stringPtr("*"),
			FacetBy:   stringPtr("category"),
			Page:      intPtr(1),
			PerPage:   intPtr(0), // Só queremos os facets, não os documentos
		}
		
		searchResult, err := c.client.Collection(colecao).Documents().Search(ctx, searchParams)
		if err != nil {
			// Se é erro 404 (coleção não encontrada), pula para próxima coleção
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
				log.Printf("Coleção %s não encontrada, pulando para próxima coleção", colecao)
				continue
			}
			log.Printf("Erro ao buscar categorias na coleção %s: %v", colecao, err)
			continue
		}
		
		// Processa os facets para obter categorias
		var resultMap map[string]interface{}
		jsonData, _ := json.Marshal(searchResult)
		json.Unmarshal(jsonData, &resultMap)
		
		if facetCounts, ok := resultMap["facet_counts"].([]interface{}); ok {
			for _, facet := range facetCounts {
				if facetMap, ok := facet.(map[string]interface{}); ok {
					if fieldName, ok := facetMap["field_name"].(string); ok && fieldName == "category" {
						if counts, ok := facetMap["counts"].([]interface{}); ok {
							// Para cada categoria encontrada nos dados, calcula a relevância dos seus serviços
							for _, count := range counts {
								if countMap, ok := count.(map[string]interface{}); ok {
									if categoria, ok := countMap["value"].(string); ok {
										if categoria != "" {
											if err := c.calcularRelevanciaCategoria(colecao, categoria, categoriasMap); err != nil {
												log.Printf("Erro ao calcular relevância da categoria %s: %v", categoria, err)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	
	// Converte o mapa em slice e ordena por relevância
	var categorias []models.CategoriaRelevancia
	for _, categoria := range categoriasMap {
		// Calcula relevância média
		if categoria.QuantidadeServicos > 0 {
			categoria.RelevanciaMedia = float64(categoria.RelevanciaTotal) / float64(categoria.QuantidadeServicos)
		}
		
		// Adiciona nome normalizado
		categoria.NomeNormalizado = utils.NormalizarCategoria(categoria.Nome)
		
		categorias = append(categorias, *categoria)
	}
	
	// Ordena por relevância total (maior primeiro)
	sort.Slice(categorias, func(i, j int) bool {
		return categorias[i].RelevanciaTotal > categorias[j].RelevanciaTotal
	})
	
	response := &models.CategoriasRelevanciaResponse{
		Categorias:        categorias,
		TotalCategorias:   len(categorias),
		UltimaAtualizacao: time.Now().Format(time.RFC3339),
	}
	
	return response, nil
}

// calcularRelevanciaCategoria calcula a relevância de uma categoria específica
func (c *Client) calcularRelevanciaCategoria(colecao string, categoria string, categoriasMap map[string]*models.CategoriaRelevancia) error {
	ctx := context.Background()
	filterBy := fmt.Sprintf("category:=%s", categoria)
	
	relevanciaTotal := 0
	quantidadeServicos := 0
	page := 1
	perPage := 250 // Máximo permitido pelo Typesense
	
	for {
		searchParams := &api.SearchCollectionParams{
			Q:             stringPtr("*"),
			FilterBy:      &filterBy,
			Page:          intPtr(page),
			PerPage:       intPtr(perPage),
			IncludeFields: stringPtr("titulo"),
			ExcludeFields: stringPtr("embedding"),
		}
		
		searchResult, err := c.client.Collection(colecao).Documents().Search(ctx, searchParams)
		if err != nil {
			// Se é erro 404 (coleção não encontrada), pula esta coleção
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
				log.Printf("Coleção %s não encontrada para categoria %s, pulando", colecao, categoria)
				return nil
			}
			return err
		}
		
		var resultMap map[string]interface{}
		jsonData, _ := json.Marshal(searchResult)
		json.Unmarshal(jsonData, &resultMap)
		
		hitsCount := 0
		if hits, ok := resultMap["hits"].([]interface{}); ok {
			hitsCount = len(hits)
			for _, h := range hits {
				if hitMap, ok := h.(map[string]interface{}); ok {
					if document, ok := hitMap["document"].(map[string]interface{}); ok {
						if titulo, ok := document["titulo"].(string); ok {
							relevancia := c.relevanciaService.ObterRelevancia(titulo)
							relevanciaTotal += relevancia
							quantidadeServicos++
						}
					}
				}
			}
		}
		
		// Se retornou menos que perPage, chegamos ao fim
		if hitsCount < perPage {
			break
		}
		
		page++
	}
	
	// Acumula no mapa de categorias (pode existir em múltiplas coleções)
	if existente, exists := categoriasMap[categoria]; exists {
		existente.RelevanciaTotal += relevanciaTotal
		existente.QuantidadeServicos += quantidadeServicos
	} else {
		categoriasMap[categoria] = &models.CategoriaRelevancia{
			Nome:               categoria,
			RelevanciaTotal:    relevanciaTotal,
			QuantidadeServicos: quantidadeServicos,
		}
	}
	
	return nil
}

// DiagnosticarCategoriasExistentes lista todas as categorias que existem nos dados das coleções
func (c *Client) DiagnosticarCategoriasExistentes(colecoes []string) (map[string]int, error) {
	ctx := context.Background()
	categoriasEncontradas := make(map[string]int)
	
	// Para cada coleção, busca todas as categorias
	for _, colecao := range colecoes {
		// Busca usando facet para obter categorias únicas
		searchParams := &api.SearchCollectionParams{
			Q:         stringPtr("*"),
			FacetBy:   stringPtr("category"),
			Page:      intPtr(1),
			PerPage:   intPtr(0), // Só queremos os facets, não os documentos
		}
		
		searchResult, err := c.client.Collection(colecao).Documents().Search(ctx, searchParams)
		if err != nil {
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
				log.Printf("Coleção %s não encontrada, pulando para próxima coleção", colecao)
				continue
			}
			log.Printf("Erro ao buscar categorias na coleção %s: %v", colecao, err)
			continue
		}
		
		// Processa os facets para obter categorias
		var resultMap map[string]interface{}
		jsonData, _ := json.Marshal(searchResult)
		json.Unmarshal(jsonData, &resultMap)
		
		if facetCounts, ok := resultMap["facet_counts"].([]interface{}); ok {
			for _, facet := range facetCounts {
				if facetMap, ok := facet.(map[string]interface{}); ok {
					if fieldName, ok := facetMap["field_name"].(string); ok && fieldName == "category" {
						if counts, ok := facetMap["counts"].([]interface{}); ok {
							for _, count := range counts {
								if countMap, ok := count.(map[string]interface{}); ok {
									if categoria, ok := countMap["value"].(string); ok {
										if quantidade, ok := countMap["count"].(float64); ok {
											categoriasEncontradas[categoria] += int(quantidade)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	
	return categoriasEncontradas, nil
}

// stringPtr retorna um ponteiro para string
func stringPtr(s string) *string {
	return &s
}

// intPtr retorna um ponteiro para int
func intPtr(i int) *int {
	return &i
}

// EnsureCollectionExists verifica se a collection existe e a cria se necessário
func (c *Client) EnsureCollectionExists(collectionName string) error {
	ctx := context.Background()
	
	// Verifica se a collection já existe
	_, err := c.client.Collection(collectionName).Retrieve(ctx)
	if err == nil {
		// Collection já existe
		return nil
	}
	
	// Se não existe, cria a collection
	if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
		return c.createPrefRioServicesCollection(collectionName)
	}
	
	return err
}

// createPrefRioServicesCollection cria a collection prefrio_services_base com o schema apropriado
func (c *Client) createPrefRioServicesCollection(collectionName string) error {
	ctx := context.Background()
	
	schema := &api.CollectionSchema{
		Name: collectionName,
		Fields: []api.Field{
			{Name: "id", Type: "string", Optional: boolPtr(true)},
			{Name: "nome_servico", Type: "string", Facet: boolPtr(false)},
			{Name: "orgao_gestor", Type: "string[]", Facet: boolPtr(true)},
			{Name: "resumo", Type: "string", Facet: boolPtr(false)},
			{Name: "tempo_atendimento", Type: "string", Facet: boolPtr(false)},
			{Name: "custo_servico", Type: "string", Facet: boolPtr(true)},
			{Name: "resultado_solicitacao", Type: "string", Facet: boolPtr(true)},
			{Name: "descricao_completa", Type: "string", Facet: boolPtr(false)},
			{Name: "autor", Type: "string", Facet: boolPtr(true)},
			{Name: "documentos_necessarios", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "instrucoes_solicitante", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "canais_digitais", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "canais_presenciais", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "servico_nao_cobre", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "legislacao_relacionada", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "tema_geral", Type: "string", Facet: boolPtr(true)},
			{Name: "publico_especifico", Type: "string[]", Facet: boolPtr(true), Optional: boolPtr(true)},
			{Name: "fixar_destaque", Type: "bool", Facet: boolPtr(true)},
			{Name: "awaiting_approval", Type: "bool", Facet: boolPtr(true)},
			{Name: "published_at", Type: "int64", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "is_free", Type: "bool", Facet: boolPtr(true), Optional: boolPtr(true)},
			{Name: "agents", Type: "object", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "extra_fields", Type: "object", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "status", Type: "int32", Facet: boolPtr(true)},
			{Name: "created_at", Type: "int64", Facet: boolPtr(false)},
			{Name: "last_update", Type: "int64", Facet: boolPtr(false)},
			{Name: "search_content", Type: "string", Facet: boolPtr(false)},
			{Name: "embedding", Type: "float[]", Facet: boolPtr(false), Optional: boolPtr(true), NumDim: intPtr(768)},
		},
		DefaultSortingField:  stringPtr("last_update"),
		EnableNestedFields:   boolPtr(true),
	}
	
	_, err := c.client.Collections().Create(ctx, schema)
	if err != nil {
		return fmt.Errorf("erro ao criar collection %s: %v", collectionName, err)
	}
	
	log.Printf("Collection %s criada com sucesso", collectionName)
	return nil
}

// CreatePrefRioService cria um novo serviço na collection prefrio_services_base
func (c *Client) CreatePrefRioService(ctx context.Context, service *models.PrefRioService) (*models.PrefRioService, error) {
	collectionName := "prefrio_services_base"
	
	// Garante que a collection existe
	if err := c.EnsureCollectionExists(collectionName); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}
	
	// Define timestamps
	now := time.Now().Unix()
	service.CreatedAt = now
	service.LastUpdate = now
	
	// Gera o search_content combinando campos relevantes
	service.SearchContent = c.generateSearchContent(service)
	
	// Gera embedding se o cliente Gemini estiver disponível
	if c.geminiClient != nil {
		embedding, err := c.GerarEmbedding(ctx, service.SearchContent)
		if err != nil {
			log.Printf("Aviso: erro ao gerar embedding: %v", err)
		} else {
			// Converte []float32 para []float64
			service.Embedding = make([]float64, len(embedding))
			for i, v := range embedding {
				service.Embedding[i] = float64(v)
			}
		}
	}
	
	// Converte para map[string]interface{} para inserção
	serviceMap, err := c.structToMap(service)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter service para map: %v", err)
	}
	
	// Remove o ID se estiver vazio para auto-geração
	if service.ID == "" {
		delete(serviceMap, "id")
	}
	
	// Insere o documento
	result, err := c.client.Collection(collectionName).Documents().Create(ctx, serviceMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar serviço: %v", err)
	}
	
	// Converte o resultado de volta para o struct
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}
	
	var createdService models.PrefRioService
	if err := json.Unmarshal(resultBytes, &createdService); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}
	
	return &createdService, nil
}

// UpdatePrefRioService atualiza um serviço existente na collection prefrio_services_base
func (c *Client) UpdatePrefRioService(ctx context.Context, id string, service *models.PrefRioService) (*models.PrefRioService, error) {
	collectionName := "prefrio_services_base"
	
	// Verifica se o documento existe
	_, err := c.client.Collection(collectionName).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("serviço não encontrado: %v", err)
	}
	
	// Define o ID e atualiza o timestamp
	service.ID = id
	service.LastUpdate = time.Now().Unix()
	
	// Gera o search_content combinando campos relevantes
	service.SearchContent = c.generateSearchContent(service)
	
	// Gera embedding se o cliente Gemini estiver disponível
	if c.geminiClient != nil {
		embedding, err := c.GerarEmbedding(ctx, service.SearchContent)
		if err != nil {
			log.Printf("Aviso: erro ao gerar embedding: %v", err)
		} else {
			// Converte []float32 para []float64
			service.Embedding = make([]float64, len(embedding))
			for i, v := range embedding {
				service.Embedding[i] = float64(v)
			}
		}
	}
	
	// Converte para map[string]interface{} para atualização
	serviceMap, err := c.structToMap(service)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter service para map: %v", err)
	}
	
	// Atualiza o documento
	result, err := c.client.Collection(collectionName).Document(id).Update(ctx, serviceMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar serviço: %v", err)
	}
	
	// Converte o resultado de volta para o struct
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}
	
	var updatedService models.PrefRioService
	if err := json.Unmarshal(resultBytes, &updatedService); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}
	
	return &updatedService, nil
}

// DeletePrefRioService deleta um serviço da collection prefrio_services_base
func (c *Client) DeletePrefRioService(ctx context.Context, id string) error {
	collectionName := "prefrio_services_base"
	
	// Verifica se o documento existe
	_, err := c.client.Collection(collectionName).Document(id).Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("serviço não encontrado: %v", err)
	}
	
	// Deleta o documento
	_, err = c.client.Collection(collectionName).Document(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("erro ao deletar serviço: %v", err)
	}
	
	return nil
}

// GetPrefRioService busca um serviço específico por ID
func (c *Client) GetPrefRioService(ctx context.Context, id string) (*models.PrefRioService, error) {
	collectionName := "prefrio_services_base"
	
	result, err := c.client.Collection(collectionName).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("serviço não encontrado: %v", err)
	}
	
	// Converte o resultado para o struct
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}
	
	var service models.PrefRioService
	if err := json.Unmarshal(resultBytes, &service); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}
	
	return &service, nil
}

// ListPrefRioServices lista serviços com paginação e filtros
func (c *Client) ListPrefRioServices(ctx context.Context, page, perPage int, filters map[string]interface{}) (*models.PrefRioServiceResponse, error) {
	collectionName := "prefrio_services_base"

	// Extrai nome_servico para busca textual
	var nomeServico string
	if nomeServicoValue, exists := filters["nome_servico"]; exists {
		if str, ok := nomeServicoValue.(string); ok && str != "" {
			nomeServico = str
			// Remove nome_servico dos filtros normais para não aplicar correspondência exata
			delete(filters, "nome_servico")
		}
	}

	// Constrói filtros (sem nome_servico)
	var filterBy string
	if len(filters) > 0 {
		var filterParts []string
		for key, value := range filters {
			switch v := value.(type) {
			case string:
				if v != "" {
					// Normaliza strings para melhor busca
					normalizedValue := utils.NormalizarCategoria(v)
					filterParts = append(filterParts, fmt.Sprintf("%s:=%s", key, normalizedValue))
				}
			case int:
				filterParts = append(filterParts, fmt.Sprintf("%s:=%d", key, v))
			case int64:
				filterParts = append(filterParts, fmt.Sprintf("%s:=%d", key, v))
			case bool:
				filterParts = append(filterParts, fmt.Sprintf("%s:=%t", key, v))
			}
		}
		if len(filterParts) > 0 {
			filterBy = strings.Join(filterParts, " && ")
		}
	}

	// Parâmetros de busca
	searchParams := &api.SearchCollectionParams{
		Page:          intPtr(page),
		PerPage:       intPtr(perPage),
		IncludeFields: stringPtr("*"),
		ExcludeFields: stringPtr("embedding"),
		SortBy:        stringPtr("last_update:desc"),
	}

	// Se há busca por nome do serviço, usa busca textual
	if nomeServico != "" {
		searchParams.Q = stringPtr(nomeServico)
		searchParams.QueryBy = stringPtr("nome_servico,search_content")
	} else {
		// Busca genérica se não há termo específico
		searchParams.Q = stringPtr("*")
	}
	
	if filterBy != "" {
		searchParams.FilterBy = &filterBy
	}
	
	// Executa a busca
	searchResult, err := c.client.Collection(collectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar serviços: %v", err)
	}
	
	// Converte resultado
	var resultMap map[string]interface{}
	jsonData, err := json.Marshal(searchResult)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}
	
	if err := json.Unmarshal(jsonData, &resultMap); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}
	
	// Extrai serviços
	var services []models.PrefRioService
	if hits, ok := resultMap["hits"].([]interface{}); ok {
		for _, hit := range hits {
			if hitMap, ok := hit.(map[string]interface{}); ok {
				if document, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(document)
					var service models.PrefRioService
					if err := json.Unmarshal(docBytes, &service); err == nil {
						services = append(services, service)
					}
				}
			}
		}
	}
	
	// Monta resposta
	found := 0
	outOf := 0
	if foundFloat, ok := resultMap["found"].(float64); ok {
		found = int(foundFloat)
		outOf = found
	}
	
	response := &models.PrefRioServiceResponse{
		Found:    found,
		OutOf:    outOf,
		Page:     page,
		Services: services,
	}
	
	return response, nil
}

// generateSearchContent gera o conteúdo de busca combinando campos relevantes
func (c *Client) generateSearchContent(service *models.PrefRioService) string {
	var content []string
	
	if service.NomeServico != "" {
		content = append(content, service.NomeServico)
	}
	if service.Resumo != "" {
		content = append(content, service.Resumo)
	}
	if service.DescricaoCompleta != "" {
		content = append(content, service.DescricaoCompleta)
	}
	if service.TemaGeral != "" {
		content = append(content, service.TemaGeral)
	}
	
	// Adiciona órgãos gestores
	content = append(content, service.OrgaoGestor...)
	
	// Adiciona público específico
	content = append(content, service.PublicoEspecifico...)
	
	// Adiciona documentos necessários
	content = append(content, service.DocumentosNecessarios...)
	
	return strings.Join(content, " ")
}

// structToMap converte um struct para map[string]interface{}
func (c *Client) structToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}
	
	return result, nil
}

// boolPtr retorna um ponteiro para bool
func boolPtr(b bool) *bool {
	return &b
}