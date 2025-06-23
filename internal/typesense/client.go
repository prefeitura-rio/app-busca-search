package typesense

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
	"google.golang.org/genai"
)

type Client struct {
	client *typesense.Client
	geminiClient *genai.Client
	embeddingModel string
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
		return &Client{
			client: typesenseClient,
			geminiClient: nil,
			embeddingModel: cfg.GeminiEmbeddingModel,
		}
	}

	return &Client{
		client: typesenseClient,
		geminiClient: geminiClient,
		embeddingModel: cfg.GeminiEmbeddingModel,
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

func (c *Client) BuscaComTexto(ctx context.Context, colecao string, query string, pagina int, porPagina int) (map[string]interface{}, error) {
	vetor, err := c.GerarEmbedding(ctx, query)
	if err != nil {
		return c.Busca(colecao, query, pagina, porPagina, nil)
	}
	
	return c.Busca(colecao, query, pagina, porPagina, vetor)
}

func (c *Client) BuscaMultiColecaoComTexto(ctx context.Context, colecoes []string, query string, pagina int, porPagina int) (map[string]interface{}, error) {
	vetor, err := c.GerarEmbedding(ctx, query)
	if err != nil {
		return c.BuscaMultiColecao(colecoes, query, pagina, porPagina, nil)
	}
	
	return c.BuscaMultiColecao(colecoes, query, pagina, porPagina, vetor)
}

func (c *Client) Busca(colecao string, query string, pagina int, porPagina int, vetor []float32) (map[string]interface{}, error) {
	ctx := context.Background()
	queryStr := query
	queryByStr := "titulo_texto_normalizado,descricao_texto_normalizado"
	includeFields := "*"
	excludeFields := "embedding" // Exclui o campo embedding da resposta para economizar largura de banda

	searchParams := &api.SearchCollectionParams{
		Q:             &queryStr,
		QueryBy:       &queryByStr,
		Page:          &pagina,
		PerPage:       &porPagina,
		IncludeFields: &includeFields,
		ExcludeFields: &excludeFields,
	}

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
		vectorQuery := fmt.Sprintf("embedding:%s=>alpha:%.1f", vectorStr, alpha)
		searchParams.VectorQuery = &vectorQuery
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

	return resultMap, nil
}

func (c *Client) BuscaVetorial(colecao string, vetor []float32, pagina int, porPagina int) (map[string]interface{}, error) {
	ctx := context.Background()
	includeFields := "*"
	excludeFields := "embedding"
	
	if len(vetor) == 0 {
		return nil, fmt.Errorf("vetor de embedding é obrigatório para busca vetorial")
	}
	
	vectorStr := "["
	for i, val := range vetor {
		if i > 0 {
			vectorStr += ", "
		}
		vectorStr += fmt.Sprintf("%.6f", val)
	}
	vectorStr += "]"
	
	vectorQuery := fmt.Sprintf("embedding:%s", vectorStr)
	
	searchParams := &api.SearchCollectionParams{
		Page:          &pagina,
		PerPage:       &porPagina,
		IncludeFields: &includeFields,
		ExcludeFields: &excludeFields,
		VectorQuery:   &vectorQuery,
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

	return resultMap, nil
}

func (c *Client) BuscaVetorialComTexto(ctx context.Context, colecao string, query string, pagina int, porPagina int) (map[string]interface{}, error) {
	vetor, err := c.GerarEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("não foi possível gerar embedding para busca vetorial: %v", err)
	}
	
	return c.BuscaVetorial(colecao, vetor, pagina, porPagina)
}

func (c *Client) BuscaMultiColecao(colecoes []string, query string, pagina int, porPagina int, vetor []float32) (map[string]interface{}, error) {
	ctx := context.Background()
	queryStr := query
	queryByStr := "titulo_texto_normalizado,descricao_texto_normalizado"
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