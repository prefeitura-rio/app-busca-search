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

	return &Client{
		client: typesenseClient,
		geminiClient: geminiClient,
		embeddingModel: cfg.GeminiEmbeddingModel,
		relevanciaService: relevanciaService,
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
							// Para cada categoria, calcula a relevância dos seus serviços
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

// stringPtr retorna um ponteiro para string
func stringPtr(s string) *string {
	return &s
}

// intPtr retorna um ponteiro para int
func intPtr(i int) *int {
	return &i
}