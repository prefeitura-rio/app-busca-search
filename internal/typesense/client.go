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
	client         *typesense.Client
	geminiClient   *genai.Client
	embeddingModel string
	versionService *services.VersionService
	gatewayBaseURL string
	// relevanciaService and filterService REMOVED - no longer used
}

func NewClient(cfg *config.Config) *Client {
	// Validate gateway configuration
	if cfg.GatewayBaseURL == "" {
		log.Fatal("GATEWAY_BASE_URL environment variable is required but not set")
	}

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

	// REMOVED: relevanciaService and filterService initialization
	// These services have been removed from the codebase

	// Inicializa o serviço de versionamento (passa o client interno)
	versionService := services.NewVersionService(typesenseClient)

	client := &Client{
		client:         typesenseClient,
		geminiClient:   geminiClient,
		embeddingModel: cfg.GeminiEmbeddingModel,
		versionService: versionService,
		gatewayBaseURL: cfg.GatewayBaseURL,
	}

	// Garante que a collection de tombamentos existe
	if err := client.EnsureTombamentosCollectionExists(); err != nil {
		log.Printf("Aviso: não foi possível criar/verificar collection tombamentos_overlay: %v", err)
	} else {
		log.Println("Collection tombamentos_overlay verificada/criada com sucesso")
	}

	// Garante que a collection prefrio_services_base existe
	if err := client.EnsureCollectionExists("prefrio_services_base"); err != nil {
		log.Printf("Aviso: não foi possível criar/verificar collection prefrio_services_base: %v", err)
	} else {
		log.Println("Collection prefrio_services_base verificada/criada com sucesso")
	}

	// Garante que a collection service_versions existe
	if err := client.EnsureCollectionExists("service_versions"); err != nil {
		log.Printf("Aviso: não foi possível criar/verificar collection service_versions: %v", err)
	} else {
		log.Println("Collection service_versions verificada/criada com sucesso")
	}

	// Garante que a collection hub_search existe
	if err := client.EnsureCollectionExists("hub_search"); err != nil {
		log.Printf("Aviso: não foi possível criar/verificar collection hub_search: %v", err)
	} else {
		log.Println("Collection hub_search verificada/criada com sucesso")
	}

	return client
}

// GetClient retorna o cliente Typesense interno (para uso com hub services)
func (c *Client) GetClient() *typesense.Client {
	return c.client
}

func (c *Client) GerarEmbedding(ctx context.Context, texto string) ([]float32, error) {
	if c.geminiClient == nil {
		return nil, fmt.Errorf("cliente Gemini não inicializado")
	}

	// Trunca texto muito longo
	maxLength := 10000
	if len(texto) > maxLength {
		texto = texto[:maxLength]
	}

	content := genai.NewContentFromText(texto, genai.RoleUser)

	// Configurar para gerar embeddings com 768 dimensões
	outputDim := int32(768)
	config := &genai.EmbedContentConfig{
		OutputDimensionality: &outputDim,
	}

	resp, err := c.geminiClient.Models.EmbedContent(ctx, c.embeddingModel, []*genai.Content{content}, config)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar embedding: %v", err)
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("nenhum embedding foi gerado")
	}

	embedding := resp.Embeddings[0].Values

	// Valida dimensões (sempre 768)
	if len(embedding) != 768 {
		log.Printf("AVISO: Embedding de query tem %d dimensões (esperado: 768)", len(embedding))
		return nil, fmt.Errorf("embedding com dimensões incorretas: %d", len(embedding))
	}

	return embedding, nil
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

		// Aplica filtro status:=1 (publicado) para prefrio_services_base
		if colecao == "prefrio_services_base" {
			filterBy := "status:=1"
			collectionParams.FilterBy = &filterBy
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
		collection     string
		raw            map[string]interface{}
	}

	var allHits []hitWrapper
	totalFound := 0

	for i, res := range searchResult.Results {
		if res.Found != nil {
			totalFound += int(*res.Found)
		}
		if res.Hits == nil {
			continue
		}

		// Identifica qual collection este resultado pertence
		currentCollection := ""
		if i < len(colecoes) {
			currentCollection = colecoes[i]
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
				collection:     currentCollection,
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

	// Primeiro filtro: Remove documentos legados que foram tombados
	tombamentoFilteredHits := make([]hitWrapper, 0, len(allHits))
	for _, hw := range allHits {
		shouldKeep := true

		// Extrai ID do documento
		if document, ok := hw.raw["document"].(map[string]interface{}); ok {
			if id, ok := document["id"].(string); ok {
				// Verifica se documento legado foi tombado
				if c.isLegacyCollectionTombado(ctx, hw.collection, id) {
					shouldKeep = false
					log.Printf("Removendo serviço tombado: collection=%s, id=%s", hw.collection, id)
				}
			}
		}

		if shouldKeep {
			tombamentoFilteredHits = append(tombamentoFilteredHits, hw)
		}
	}
	allHits = tombamentoFilteredHits

	// REMOVED: filterService - CSV-based filtering no longer used
	// Legacy code that filtered documents from carioca-digital based on CSV

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
		"found":  totalFound,
		"out_of": totalFound,
		"page":   pagina,
		"hits":   pagedHits,
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

		// Prepara o filtro para esta coleção específica
		collectionFilterBy := filterBy
		if colecao == "prefrio_services_base" {
			// Adiciona filtro status:=1 (publicado) para prefrio_services_base
			collectionFilterBy = fmt.Sprintf("%s && status:=1", filterBy)
		}

		for {
			searchParams := &api.SearchCollectionParams{
				Q:             stringPtr("*"),
				FilterBy:      &collectionFilterBy,
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
						// Verifica se documento legado foi tombado
						shouldKeep := true
						if document, ok := hitMap["document"].(map[string]interface{}); ok {
							if id, ok := document["id"].(string); ok {
								if c.isLegacyCollectionTombado(ctx, colecao, id) {
									shouldKeep = false
									log.Printf("Removendo serviço tombado da categoria: collection=%s, id=%s", colecao, id)
								}
							}
						}

						if !shouldKeep {
							continue // Pula este documento
						}

						// REMOVED: relevanciaService - volumetry-based relevance no longer used
						// Legacy code that calculated relevance based on CSV volumetry data
						relevancia := 0

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

	// REMOVED: filterService - CSV-based filtering no longer used
	// Legacy code that filtered documents from carioca-digital based on CSV

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
		"found":  totalFound,
		"out_of": totalFound,
		"page":   pagina,
		"hits":   pagedHits,
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
					// REMOVED: relevanciaService - volumetry-based relevance no longer used
					// Legacy code that calculated relevance based on CSV volumetry data
					relevancia := 0

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

	// REMOVED: filterService - CSV-based filtering no longer used
	// Legacy code that filtered documents from carioca-digital based on CSV

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
// Se o documento for de collection legada e foi tombado, retorna o documento novo
func (c *Client) BuscaPorID(colecao string, documentoID string) (map[string]interface{}, error) {
	ctx := context.Background()

	// Verifica se documento legado foi tombado
	if c.isLegacyCollectionTombado(ctx, colecao, documentoID) {
		// Busca o tombamento para obter o ID do serviço novo
		tombamento, err := c.GetTombamentoByOldServiceID(ctx, colecao, documentoID)
		if err == nil && tombamento != nil {
			log.Printf("Documento %s da collection %s foi tombado, retornando serviço novo %s",
				documentoID, colecao, tombamento.IDServicoNovo)

			// Retorna o documento novo da prefrio_services_base
			return c.BuscaPorID("prefrio_services_base", tombamento.IDServicoNovo)
		}
	}

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
			Q:       stringPtr("*"),
			FacetBy: stringPtr("category"),
			Page:    intPtr(1),
			PerPage: intPtr(0), // Só queremos os facets, não os documentos
		}

		// Adiciona filtro status:=1 (publicado) para prefrio_services_base
		if colecao == "prefrio_services_base" {
			filterBy := "status:=1"
			searchParams.FilterBy = &filterBy
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

	// Adiciona filtro status:=1 (publicado) para prefrio_services_base
	if colecao == "prefrio_services_base" {
		filterBy = fmt.Sprintf("%s && status:=1", filterBy)
	}

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
						if _, ok := document["titulo"].(string); ok {
							// REMOVED: relevanciaService - volumetry-based relevance no longer used
							// Legacy code that calculated relevance based on CSV volumetry data
							relevancia := 0
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
			Q:       stringPtr("*"),
			FacetBy: stringPtr("category"),
			Page:    intPtr(1),
			PerPage: intPtr(0), // Só queremos os facets, não os documentos
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

	// Se não existe, cria a collection baseado no nome
	if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
		switch collectionName {
		case "service_versions":
			return c.createServiceVersionsCollection(collectionName)
		case "prefrio_services_base":
			return c.createPrefRioServicesCollection(collectionName)
		case "hub_search":
			return c.createHubSearchCollection(collectionName)
		default:
			// Para outras collections, assume schema de prefrio_services_base
			return c.createPrefRioServicesCollection(collectionName)
		}
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
			{Name: "buttons", Type: "object[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "embedding", Type: "float[]", Facet: boolPtr(false), Optional: boolPtr(true), NumDim: intPtr(768)},
		},
		DefaultSortingField: stringPtr("last_update"),
		EnableNestedFields:  boolPtr(true),
	}

	_, err := c.client.Collections().Create(ctx, schema)
	if err != nil {
		return fmt.Errorf("erro ao criar collection %s: %v", collectionName, err)
	}

	log.Printf("Collection %s criada com sucesso", collectionName)
	return nil
}

// createServiceVersionsCollection cria a collection service_versions com o schema apropriado
func (c *Client) createServiceVersionsCollection(collectionName string) error {
	ctx := context.Background()

	schema := &api.CollectionSchema{
		Name: collectionName,
		Fields: []api.Field{
			{Name: "id", Type: "string", Optional: boolPtr(true)},
			{Name: "service_id", Type: "string", Facet: boolPtr(true)},
			{Name: "version_number", Type: "int64", Facet: boolPtr(true)},
			{Name: "created_at", Type: "int64", Facet: boolPtr(false)},
			{Name: "created_by", Type: "string", Facet: boolPtr(true)},
			{Name: "created_by_cpf", Type: "string", Facet: boolPtr(true)},
			{Name: "change_type", Type: "string", Facet: boolPtr(true)},
			{Name: "change_reason", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "previous_version", Type: "int64", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "is_rollback", Type: "bool", Facet: boolPtr(true)},
			{Name: "rollback_to_version", Type: "int64", Facet: boolPtr(false), Optional: boolPtr(true)},

			// Snapshot do serviço (campos principais)
			{Name: "nome_servico", Type: "string", Facet: boolPtr(false)},
			{Name: "orgao_gestor", Type: "string[]", Facet: boolPtr(false)},
			{Name: "resumo", Type: "string", Facet: boolPtr(false)},
			{Name: "tempo_atendimento", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "custo_servico", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "resultado_solicitacao", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "descricao_completa", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "autor", Type: "string", Facet: boolPtr(false)},
			{Name: "documentos_necessarios", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "instrucoes_solicitante", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "canais_digitais", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "canais_presenciais", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "servico_nao_cobre", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "legislacao_relacionada", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "tema_geral", Type: "string", Facet: boolPtr(false)},
			{Name: "publico_especifico", Type: "string[]", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "fixar_destaque", Type: "bool", Facet: boolPtr(false)},
			{Name: "awaiting_approval", Type: "bool", Facet: boolPtr(false)},
			{Name: "published_at", Type: "int64", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "is_free", Type: "bool", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "status", Type: "int32", Facet: boolPtr(true)},
			{Name: "search_content", Type: "string", Facet: boolPtr(false)},

			// Campos de controle de versão
			{Name: "embedding_hash", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "changed_fields_json", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
		},
		DefaultSortingField: stringPtr("created_at"),
		EnableNestedFields:  boolPtr(true),
	}

	_, err := c.client.Collections().Create(ctx, schema)
	if err != nil {
		return fmt.Errorf("erro ao criar collection %s: %v", collectionName, err)
	}

	log.Printf("Collection %s criada com sucesso", collectionName)
	return nil
}

// createHubSearchCollection cria a collection hub_search com o schema apropriado
func (c *Client) createHubSearchCollection(collectionName string) error {
	ctx := context.Background()

	schema := &api.CollectionSchema{
		Name: collectionName,
		Fields: []api.Field{
			// Identity
			{Name: "id", Type: "string", Optional: boolPtr(true)},
			{Name: "hub_id", Type: "string", Facet: boolPtr(true)},
			{Name: "source_type", Type: "string", Facet: boolPtr(true)},
			{Name: "source_collection", Type: "string", Facet: boolPtr(true)},
			{Name: "source_id", Type: "string", Facet: boolPtr(true)},

			// Segmentation
			{Name: "portal_tags", Type: "string[]", Facet: boolPtr(true)},
			{Name: "context_tags", Type: "string[]", Facet: boolPtr(true)},

			// Search Fields
			{Name: "title", Type: "string", Facet: boolPtr(false)},
			{Name: "description", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "summary", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "content", Type: "string", Facet: boolPtr(false)},

			// Categorization
			{Name: "category", Type: "string", Facet: boolPtr(true), Optional: boolPtr(true)},
			{Name: "subcategories", Type: "string[]", Facet: boolPtr(true), Optional: boolPtr(true)},
			{Name: "tags", Type: "string[]", Facet: boolPtr(true), Optional: boolPtr(true)},

			// Metadata
			{Name: "status", Type: "int32", Facet: boolPtr(true)},
			{Name: "priority", Type: "int32", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "relevance_score", Type: "int32", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "created_at", Type: "int64", Facet: boolPtr(false)},
			{Name: "updated_at", Type: "int64", Facet: boolPtr(false)},

			// Embeddings (768-dimensional vector for semantic search with gemini-embedding-001)
			{Name: "embedding", Type: "float[]", NumDim: intPtr(768), Optional: boolPtr(true)},
		},
		DefaultSortingField: stringPtr("updated_at"),
		EnableNestedFields:  boolPtr(true),
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
	return c.CreatePrefRioServiceWithVersion(ctx, service, "", "")
}

// CreatePrefRioServiceWithVersion cria um novo serviço e captura a primeira versão
func (c *Client) CreatePrefRioServiceWithVersion(ctx context.Context, service *models.PrefRioService, userName, userCPF string) (*models.PrefRioService, error) {
	collectionName := "prefrio_services_base"

	// Garante que a collection existe
	if err := c.EnsureCollectionExists(collectionName); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	// Define timestamps
	now := time.Now().Unix()
	service.CreatedAt = now
	service.LastUpdate = now

	// Wrap service URLs through gateway
	c.wrapServiceURLs(service)

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

	// Captura versão 1 se informações do usuário forem fornecidas
	if userName != "" && userCPF != "" {
		_, err = c.versionService.CaptureVersion(
			ctx,
			&createdService,
			"create",
			userName,
			userCPF,
			"Criação inicial do serviço",
			nil, // Não há versão anterior
		)
		if err != nil {
			log.Printf("Aviso: erro ao capturar versão inicial: %v", err)
			// Não falha a criação do serviço se a versão falhar
		}
	}

	return &createdService, nil
}

// UpdatePrefRioService atualiza um serviço existente na collection prefrio_services_base
func (c *Client) UpdatePrefRioService(ctx context.Context, id string, service *models.PrefRioService) (*models.PrefRioService, error) {
	return c.UpdatePrefRioServiceWithVersion(ctx, id, service, "", "", "")
}

// UpdatePrefRioServiceWithVersion atualiza um serviço e captura a nova versão
func (c *Client) UpdatePrefRioServiceWithVersion(ctx context.Context, id string, service *models.PrefRioService, userName, userCPF, changeReason string) (*models.PrefRioService, error) {
	collectionName := "prefrio_services_base"

	// Verifica se o documento existe
	_, err := c.client.Collection(collectionName).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("serviço não encontrado: %v", err)
	}

	// Busca a versão anterior (sempre, para rastrear mudanças)
	previousVersion, err := c.versionService.GetLatestVersion(ctx, id)
	if err != nil {
		log.Printf("Aviso: erro ao buscar versão anterior: %v", err)
		// Não é erro crítico, versão anterior pode não existir
		previousVersion = nil
	}

	// Define o ID e atualiza o timestamp
	service.ID = id
	service.LastUpdate = time.Now().Unix()

	// Wrap service URLs through gateway
	c.wrapServiceURLs(service)

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

	// Valida que temos informações do usuário
	if userName == "" || userCPF == "" {
		log.Printf("ERRO: Tentativa de atualizar serviço sem informações do usuário! userName='%s' userCPF='%s'", userName, userCPF)
		return nil, fmt.Errorf("informações do usuário não fornecidas - userName ou userCPF vazios")
	}

	// Captura nova versão (sempre)
	if changeReason == "" {
		changeReason = "Atualização do serviço"
	}
	_, err = c.versionService.CaptureVersion(
		ctx,
		&updatedService,
		"update",
		userName,
		userCPF,
		changeReason,
		previousVersion,
	)
	if err != nil {
		log.Printf("Aviso: erro ao capturar nova versão: %v", err)
		// Não falha a atualização se a versão falhar
	}

	return &updatedService, nil
}

// DeletePrefRioService deleta um serviço da collection prefrio_services_base
func (c *Client) DeletePrefRioService(ctx context.Context, id string) error {
	return c.DeletePrefRioServiceWithVersion(ctx, id, "", "")
}

// DeletePrefRioServiceWithVersion deleta um serviço e captura versão de deleção
func (c *Client) DeletePrefRioServiceWithVersion(ctx context.Context, id string, userName, userCPF string) error {
	collectionName := "prefrio_services_base"

	// Busca o serviço antes de deletar para capturar versão
	service, err := c.GetPrefRioService(ctx, id)
	if err != nil {
		return fmt.Errorf("serviço não encontrado: %v", err)
	}

	// Busca versão anterior se usuário fornecido
	var previousVersion *models.ServiceVersion
	if userName != "" && userCPF != "" {
		previousVersion, err = c.versionService.GetLatestVersion(ctx, id)
		if err != nil {
			log.Printf("Aviso: erro ao buscar versão anterior: %v", err)
		}
	}

	// Deleta o documento
	_, err = c.client.Collection(collectionName).Document(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("erro ao deletar serviço: %v", err)
	}

	// Captura versão de deleção se informações do usuário forem fornecidas
	if userName != "" && userCPF != "" {
		_, err = c.versionService.CaptureVersion(
			ctx,
			service,
			"delete",
			userName,
			userCPF,
			"Deleção do serviço",
			previousVersion,
		)
		if err != nil {
			log.Printf("Aviso: erro ao capturar versão de deleção: %v", err)
			// Não falha a deleção se a versão falhar
		}
	}

	return nil
}

// ListServiceVersions lista todas as versões de um serviço
// Se o serviço não tiver histórico de versões (serviços criados antes do sistema de versionamento),
// cria automaticamente a versão 1 a partir do estado atual
func (c *Client) ListServiceVersions(ctx context.Context, serviceID string, page, perPage int) (*models.VersionHistory, error) {
	log.Printf("[ListServiceVersions] Iniciando para serviceID=%s, page=%d, perPage=%d", serviceID, page, perPage)

	history, err := c.versionService.ListVersions(ctx, serviceID, page, perPage)

	log.Printf("[ListServiceVersions] ListVersions retornou: err=%v, history.Found=%d (se history != nil)",
		err, func() int {
			if history != nil {
				return history.Found
			}
			return -1
		}())

	// Se houve erro OU se não há versões registradas, tenta criar a versão 1 automaticamente (lazy migration)
	shouldCreateInitialVersion := (err != nil) || (history != nil && history.Found == 0)

	log.Printf("[ListServiceVersions] shouldCreateInitialVersion=%v", shouldCreateInitialVersion)

	if shouldCreateInitialVersion {
		log.Printf("[ListServiceVersions] Tentando criar versão inicial para serviceID=%s", serviceID)

		// Busca o serviço atual
		service, getErr := c.GetPrefRioService(ctx, serviceID)
		if getErr != nil {
			log.Printf("[ListServiceVersions] Erro ao buscar serviço: %v", getErr)
			// Se o serviço não existe, retorna o erro original (se houver) ou erro de serviço não encontrado
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("serviço não encontrado: %v", getErr)
		}

		log.Printf("[ListServiceVersions] Serviço encontrado: ID=%s, NomeServico=%s, Autor=%s",
			service.ID, service.NomeServico, service.Autor)

		// Cria a versão 1 inicial
		initialVersion, createErr := c.versionService.CaptureVersion(
			ctx,
			service,
			"create",
			service.Autor,
			"", // CPF não disponível para serviços legados
			"Versão inicial (criada automaticamente para serviço pré-existente)",
			nil, // Sem versão anterior
		)
		if createErr != nil {
			log.Printf("[ListServiceVersions] ERRO ao criar versão inicial para serviço %s: %v", serviceID, createErr)
			// Retorna histórico vazio ao invés de erro
			return &models.VersionHistory{
				Found:    0,
				OutOf:    0,
				Page:     page,
				Versions: []models.ServiceVersion{},
			}, nil
		}

		log.Printf("[ListServiceVersions] Versão inicial criada com sucesso: ID=%s, VersionNumber=%d",
			initialVersion.ID, initialVersion.VersionNumber)

		// Retorna a versão recém-criada como histórico
		return &models.VersionHistory{
			Found:    1,
			OutOf:    1,
			Page:     1,
			Versions: []models.ServiceVersion{*initialVersion},
		}, nil
	}

	log.Printf("[ListServiceVersions] Retornando histórico existente com %d versões", history.Found)
	return history, nil
}

// GetServiceVersionByNumber busca uma versão específica de um serviço
// Se for versão 1 e não existir, tenta criar automaticamente para serviços legados
func (c *Client) GetServiceVersionByNumber(ctx context.Context, serviceID string, versionNumber int64) (*models.ServiceVersion, error) {
	version, err := c.versionService.GetVersionByNumber(ctx, serviceID, versionNumber)

	// Se não encontrou e é versão 1, tenta criar automaticamente (lazy migration)
	if err != nil && versionNumber == 1 && strings.Contains(err.Error(), "não encontrada") {
		// Busca o serviço atual
		service, getErr := c.GetPrefRioService(ctx, serviceID)
		if getErr != nil {
			// Se o serviço não existe, retorna erro original
			return nil, err
		}

		// Cria a versão 1 inicial
		initialVersion, createErr := c.versionService.CaptureVersion(
			ctx,
			service,
			"create",
			service.Autor,
			"", // CPF não disponível para serviços legados
			"Versão inicial (criada automaticamente para serviço pré-existente)",
			nil, // Sem versão anterior
		)
		if createErr != nil {
			log.Printf("Aviso: não foi possível criar versão inicial para serviço %s: %v", serviceID, createErr)
			return nil, err // Retorna erro original
		}

		return initialVersion, nil
	}

	return version, err
}

// GetLatestServiceVersion busca a última versão de um serviço
func (c *Client) GetLatestServiceVersion(ctx context.Context, serviceID string) (*models.ServiceVersion, error) {
	return c.versionService.GetLatestVersion(ctx, serviceID)
}

// CompareServiceVersions compara duas versões de um serviço
func (c *Client) CompareServiceVersions(ctx context.Context, serviceID string, fromVersion, toVersion int64) (*models.VersionDiff, error) {
	return c.versionService.CompareVersions(ctx, serviceID, fromVersion, toVersion)
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
// wrapServiceURLs aplica o gateway wrapper em todas as URLs do serviço
func (c *Client) wrapServiceURLs(service *models.PrefRioService) {
	// Wrap URLs in buttons
	for i := range service.Buttons {
		service.Buttons[i].URLService = utils.WrapURLIfNeeded(service.Buttons[i].URLService, c.gatewayBaseURL)
	}

	// Wrap URLs in CanaisDigitais
	service.CanaisDigitais = utils.WrapURLsInArray(service.CanaisDigitais, c.gatewayBaseURL)
}

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

// ========== Funções de Tombamento ==========

// createTombamentosCollection cria a collection tombamentos_overlay com o schema apropriado
func (c *Client) createTombamentosCollection() error {
	ctx := context.Background()
	collectionName := "tombamentos_overlay"

	schema := &api.CollectionSchema{
		Name: collectionName,
		Fields: []api.Field{
			{Name: "id", Type: "string", Optional: boolPtr(true)},
			{Name: "origem", Type: "string", Facet: boolPtr(true)},
			{Name: "id_servico_antigo", Type: "string", Facet: boolPtr(false)},
			{Name: "id_servico_novo", Type: "string", Facet: boolPtr(false)},
			{Name: "criado_em", Type: "int64", Facet: boolPtr(false)},
			{Name: "criado_por", Type: "string", Facet: boolPtr(true)},
			{Name: "observacoes", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
		},
		DefaultSortingField: stringPtr("criado_em"),
	}

	_, err := c.client.Collections().Create(ctx, schema)
	if err != nil {
		return fmt.Errorf("erro ao criar collection %s: %v", collectionName, err)
	}

	log.Printf("Collection %s criada com sucesso", collectionName)
	return nil
}

// EnsureTombamentosCollectionExists verifica se a collection tombamentos_overlay existe e a cria se necessário
func (c *Client) EnsureTombamentosCollectionExists() error {
	ctx := context.Background()
	collectionName := "tombamentos_overlay"

	// Verifica se a collection já existe
	_, err := c.client.Collection(collectionName).Retrieve(ctx)
	if err == nil {
		// Collection já existe
		return nil
	}

	// Se não existe, cria a collection
	if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
		return c.createTombamentosCollection()
	}

	return err
}

// CreateTombamento cria um novo tombamento na collection tombamentos_overlay
func (c *Client) CreateTombamento(ctx context.Context, tombamento *models.Tombamento) (*models.Tombamento, error) {
	collectionName := "tombamentos_overlay"

	// Garante que a collection existe
	if err := c.EnsureTombamentosCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	// Define timestamp
	tombamento.CriadoEm = time.Now().Unix()

	// Converte para map[string]interface{} para inserção
	tombamentoMap, err := c.structToMap(tombamento)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter tombamento para map: %v", err)
	}

	// Remove o ID se estiver vazio para auto-geração
	if tombamento.ID == "" {
		delete(tombamentoMap, "id")
	}

	// Insere o documento
	result, err := c.client.Collection(collectionName).Documents().Create(ctx, tombamentoMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar tombamento: %v", err)
	}

	// Converte o resultado de volta para o struct
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var createdTombamento models.Tombamento
	if err := json.Unmarshal(resultBytes, &createdTombamento); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	return &createdTombamento, nil
}

// GetTombamento busca um tombamento específico por ID
func (c *Client) GetTombamento(ctx context.Context, id string) (*models.Tombamento, error) {
	collectionName := "tombamentos_overlay"

	// Garante que a collection existe
	if err := c.EnsureTombamentosCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	result, err := c.client.Collection(collectionName).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("tombamento não encontrado: %v", err)
	}

	// Converte o resultado para o struct
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var tombamento models.Tombamento
	if err := json.Unmarshal(resultBytes, &tombamento); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	return &tombamento, nil
}

// UpdateTombamento atualiza um tombamento existente na collection tombamentos_overlay
func (c *Client) UpdateTombamento(ctx context.Context, id string, tombamento *models.Tombamento) (*models.Tombamento, error) {
	collectionName := "tombamentos_overlay"

	// Verifica se o documento existe
	_, err := c.client.Collection(collectionName).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("tombamento não encontrado: %v", err)
	}

	// Define o ID
	tombamento.ID = id

	// Converte para map[string]interface{} para atualização
	tombamentoMap, err := c.structToMap(tombamento)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter tombamento para map: %v", err)
	}

	// Atualiza o documento
	result, err := c.client.Collection(collectionName).Document(id).Update(ctx, tombamentoMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar tombamento: %v", err)
	}

	// Converte o resultado de volta para o struct
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var updatedTombamento models.Tombamento
	if err := json.Unmarshal(resultBytes, &updatedTombamento); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	return &updatedTombamento, nil
}

// DeleteTombamento deleta um tombamento da collection tombamentos_overlay
func (c *Client) DeleteTombamento(ctx context.Context, id string) error {
	collectionName := "tombamentos_overlay"

	// Verifica se o documento existe
	_, err := c.client.Collection(collectionName).Document(id).Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("tombamento não encontrado: %v", err)
	}

	// Deleta o documento
	_, err = c.client.Collection(collectionName).Document(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("erro ao deletar tombamento: %v", err)
	}

	return nil
}

// ListTombamentos lista tombamentos com paginação e filtros
func (c *Client) ListTombamentos(ctx context.Context, page, perPage int, filters map[string]interface{}) (*models.TombamentoResponse, error) {
	collectionName := "tombamentos_overlay"

	// Garante que a collection existe
	if err := c.EnsureTombamentosCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	// Constrói filtros
	var filterBy string
	if len(filters) > 0 {
		var filterParts []string
		for key, value := range filters {
			switch v := value.(type) {
			case string:
				if v != "" {
					filterParts = append(filterParts, fmt.Sprintf("%s:=%s", key, v))
				}
			case int64:
				filterParts = append(filterParts, fmt.Sprintf("%s:=%d", key, v))
			}
		}
		if len(filterParts) > 0 {
			filterBy = strings.Join(filterParts, " && ")
		}
	}

	// Parâmetros de busca
	searchParams := &api.SearchCollectionParams{
		Q:       stringPtr("*"),
		Page:    intPtr(page),
		PerPage: intPtr(perPage),
		SortBy:  stringPtr("criado_em:desc"),
	}

	if filterBy != "" {
		searchParams.FilterBy = &filterBy
	}

	// Executa a busca
	searchResult, err := c.client.Collection(collectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar tombamentos: %v", err)
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

	// Extrai tombamentos
	var tombamentos []models.Tombamento
	if hits, ok := resultMap["hits"].([]interface{}); ok {
		for _, hit := range hits {
			if hitMap, ok := hit.(map[string]interface{}); ok {
				if document, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(document)
					var tombamento models.Tombamento
					if err := json.Unmarshal(docBytes, &tombamento); err == nil {
						tombamentos = append(tombamentos, tombamento)
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

	response := &models.TombamentoResponse{
		Found:       found,
		OutOf:       outOf,
		Page:        page,
		Tombamentos: tombamentos,
	}

	return response, nil
}

// GetTombamentoByOldServiceID busca um tombamento pelo ID do serviço antigo
func (c *Client) GetTombamentoByOldServiceID(ctx context.Context, origem, idServicoAntigo string) (*models.Tombamento, error) {
	collectionName := "tombamentos_overlay"

	// Garante que a collection existe
	if err := c.EnsureTombamentosCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	// Constrói filtro por origem e id_servico_antigo
	filterBy := fmt.Sprintf("origem:=%s && id_servico_antigo:=%s", origem, idServicoAntigo)

	searchParams := &api.SearchCollectionParams{
		Q:        stringPtr("*"),
		FilterBy: &filterBy,
		Page:     intPtr(1),
		PerPage:  intPtr(1),
	}

	searchResult, err := c.client.Collection(collectionName).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar tombamento: %v", err)
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

	// Verifica se encontrou algum resultado
	if found, ok := resultMap["found"].(float64); ok && found > 0 {
		if hits, ok := resultMap["hits"].([]interface{}); ok && len(hits) > 0 {
			if hitMap, ok := hits[0].(map[string]interface{}); ok {
				if document, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(document)
					var tombamento models.Tombamento
					if err := json.Unmarshal(docBytes, &tombamento); err == nil {
						return &tombamento, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("tombamento não encontrado para origem=%s e id_servico_antigo=%s", origem, idServicoAntigo)
}

// isLegacyCollectionTombado verifica se um documento de collection legada foi tombado
// Retorna true se foi tombado (deve ser removido dos resultados)
func (c *Client) isLegacyCollectionTombado(ctx context.Context, collection, documentID string) bool {
	// Se não é collection legada, não filtra
	if collection != "1746_v2_llm" && collection != "carioca-digital_v2_llm" {
		return false
	}

	// Verifica se existe tombamento para este documento
	_, err := c.GetTombamentoByOldServiceID(ctx, collection, documentID)

	// Se encontrou tombamento, retorna true (deve ser removido)
	return err == nil
}

// ========== Funções de Controle de Migração ==========

const MigrationControlCollection = "_migration_control"

// createMigrationControlCollection cria a collection _migration_control com o schema apropriado
func (c *Client) createMigrationControlCollection() error {
	ctx := context.Background()

	schema := &api.CollectionSchema{
		Name: MigrationControlCollection,
		Fields: []api.Field{
			{Name: "id", Type: "string", Optional: boolPtr(true)},
			{Name: "status", Type: "string", Facet: boolPtr(true)},
			{Name: "source_collection", Type: "string", Facet: boolPtr(false)},
			{Name: "target_collection", Type: "string", Facet: boolPtr(false)},
			{Name: "backup_collection", Type: "string", Facet: boolPtr(false)},
			{Name: "schema_version", Type: "string", Facet: boolPtr(true)},
			{Name: "previous_schema_version", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "started_at", Type: "int64", Facet: boolPtr(false)},
			{Name: "completed_at", Type: "int64", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "started_by", Type: "string", Facet: boolPtr(true)},
			{Name: "started_by_cpf", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "total_documents", Type: "int32", Facet: boolPtr(false)},
			{Name: "migrated_documents", Type: "int32", Facet: boolPtr(false)},
			{Name: "error_message", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
			{Name: "is_locked", Type: "bool", Facet: boolPtr(true)},
		},
		DefaultSortingField: stringPtr("started_at"),
	}

	_, err := c.client.Collections().Create(ctx, schema)
	if err != nil {
		return fmt.Errorf("erro ao criar collection %s: %v", MigrationControlCollection, err)
	}

	log.Printf("Collection %s criada com sucesso", MigrationControlCollection)
	return nil
}

// EnsureMigrationControlCollectionExists verifica se a collection _migration_control existe e a cria se necessário
func (c *Client) EnsureMigrationControlCollectionExists() error {
	ctx := context.Background()

	_, err := c.client.Collection(MigrationControlCollection).Retrieve(ctx)
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
		return c.createMigrationControlCollection()
	}

	return err
}

// CreateMigrationControl cria um novo registro de controle de migração
func (c *Client) CreateMigrationControl(ctx context.Context, migration *models.MigrationControl) (*models.MigrationControl, error) {
	if err := c.EnsureMigrationControlCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	migrationMap, err := c.structToMap(migration)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter migration para map: %v", err)
	}

	if migration.ID == "" {
		delete(migrationMap, "id")
	}

	result, err := c.client.Collection(MigrationControlCollection).Documents().Create(ctx, migrationMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar migration control: %v", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var createdMigration models.MigrationControl
	if err := json.Unmarshal(resultBytes, &createdMigration); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	return &createdMigration, nil
}

// GetMigrationControl busca um registro de migração por ID
func (c *Client) GetMigrationControl(ctx context.Context, id string) (*models.MigrationControl, error) {
	if err := c.EnsureMigrationControlCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	result, err := c.client.Collection(MigrationControlCollection).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration control não encontrado: %v", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var migration models.MigrationControl
	if err := json.Unmarshal(resultBytes, &migration); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	return &migration, nil
}

// UpdateMigrationControl atualiza um registro de migração existente
func (c *Client) UpdateMigrationControl(ctx context.Context, id string, migration *models.MigrationControl) (*models.MigrationControl, error) {
	_, err := c.client.Collection(MigrationControlCollection).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration control não encontrado: %v", err)
	}

	migration.ID = id

	migrationMap, err := c.structToMap(migration)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter migration para map: %v", err)
	}

	result, err := c.client.Collection(MigrationControlCollection).Document(id).Update(ctx, migrationMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, fmt.Errorf("erro ao atualizar migration control: %v", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var updatedMigration models.MigrationControl
	if err := json.Unmarshal(resultBytes, &updatedMigration); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	return &updatedMigration, nil
}

// GetActiveMigration busca a migração ativa (status = in_progress)
func (c *Client) GetActiveMigration(ctx context.Context) (*models.MigrationControl, error) {
	if err := c.EnsureMigrationControlCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	filterBy := "status:=in_progress"
	searchParams := &api.SearchCollectionParams{
		Q:        stringPtr("*"),
		FilterBy: &filterBy,
		Page:     intPtr(1),
		PerPage:  intPtr(1),
		SortBy:   stringPtr("started_at:desc"),
	}

	searchResult, err := c.client.Collection(MigrationControlCollection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar migração ativa: %v", err)
	}

	var resultMap map[string]interface{}
	jsonData, err := json.Marshal(searchResult)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	if err := json.Unmarshal(jsonData, &resultMap); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	if found, ok := resultMap["found"].(float64); ok && found > 0 {
		if hits, ok := resultMap["hits"].([]interface{}); ok && len(hits) > 0 {
			if hitMap, ok := hits[0].(map[string]interface{}); ok {
				if document, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(document)
					var migration models.MigrationControl
					if err := json.Unmarshal(docBytes, &migration); err == nil {
						return &migration, nil
					}
				}
			}
		}
	}

	return nil, nil
}

// IsMigrationLocked verifica se existe uma migração em andamento (sistema bloqueado)
func (c *Client) IsMigrationLocked(ctx context.Context) (bool, error) {
	migration, err := c.GetActiveMigration(ctx)
	if err != nil {
		return false, err
	}

	if migration != nil && migration.IsLocked {
		return true, nil
	}

	return false, nil
}

// ListMigrationHistory lista o histórico de migrações
func (c *Client) ListMigrationHistory(ctx context.Context, page, perPage int) (*models.MigrationHistoryResponse, error) {
	if err := c.EnsureMigrationControlCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	searchParams := &api.SearchCollectionParams{
		Q:       stringPtr("*"),
		Page:    intPtr(page),
		PerPage: intPtr(perPage),
		SortBy:  stringPtr("started_at:desc"),
	}

	searchResult, err := c.client.Collection(MigrationControlCollection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar histórico de migrações: %v", err)
	}

	var resultMap map[string]interface{}
	jsonData, err := json.Marshal(searchResult)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	if err := json.Unmarshal(jsonData, &resultMap); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	var migrations []models.MigrationHistoryItem
	if hits, ok := resultMap["hits"].([]interface{}); ok {
		for _, hit := range hits {
			if hitMap, ok := hit.(map[string]interface{}); ok {
				if document, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(document)
					var migration models.MigrationHistoryItem
					if err := json.Unmarshal(docBytes, &migration); err == nil {
						migrations = append(migrations, migration)
					}
				}
			}
		}
	}

	found := 0
	outOf := 0
	if foundFloat, ok := resultMap["found"].(float64); ok {
		found = int(foundFloat)
		outOf = found
	}

	return &models.MigrationHistoryResponse{
		Found:      found,
		OutOf:      outOf,
		Page:       page,
		Migrations: migrations,
	}, nil
}

// GetLatestCompletedMigration busca a última migração completada com sucesso
func (c *Client) GetLatestCompletedMigration(ctx context.Context) (*models.MigrationControl, error) {
	if err := c.EnsureMigrationControlCollectionExists(); err != nil {
		return nil, fmt.Errorf("erro ao verificar/criar collection: %v", err)
	}

	filterBy := "status:=completed"
	searchParams := &api.SearchCollectionParams{
		Q:        stringPtr("*"),
		FilterBy: &filterBy,
		Page:     intPtr(1),
		PerPage:  intPtr(1),
		SortBy:   stringPtr("completed_at:desc"),
	}

	searchResult, err := c.client.Collection(MigrationControlCollection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar última migração: %v", err)
	}

	var resultMap map[string]interface{}
	jsonData, err := json.Marshal(searchResult)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	if err := json.Unmarshal(jsonData, &resultMap); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	if found, ok := resultMap["found"].(float64); ok && found > 0 {
		if hits, ok := resultMap["hits"].([]interface{}); ok && len(hits) > 0 {
			if hitMap, ok := hits[0].(map[string]interface{}); ok {
				if document, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(document)
					var migration models.MigrationControl
					if err := json.Unmarshal(docBytes, &migration); err == nil {
						return &migration, nil
					}
				}
			}
		}
	}

	return nil, nil
}
