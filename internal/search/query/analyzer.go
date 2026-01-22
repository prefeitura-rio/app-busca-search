package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/genai"
)

// QueryAnalysis representa a análise de uma query pelo LLM
type QueryAnalysis struct {
	Intent         string   `json:"intent"`          // buscar_servico, listar_categoria, esclarecer_duvida
	Keywords       []string `json:"keywords"`        // palavras-chave extraídas
	Categories     []string `json:"categories"`      // categorias inferidas
	RefinedQueries []string `json:"refined_queries"` // variações da query
	SearchStrategy string   `json:"search_strategy"` // hybrid, semantic, keyword
	Confidence     float64  `json:"confidence"`      // 0-1
}

// Analyzer analisa queries usando LLM
type Analyzer struct {
	client    *genai.Client
	model     string
	cache     map[string]*QueryAnalysis
	cacheTTL  time.Duration
	cacheTime map[string]time.Time
}

// NewAnalyzer cria um novo analyzer
func NewAnalyzer(client *genai.Client, model string) *Analyzer {
	return &Analyzer{
		client:    client,
		model:     model,
		cache:     make(map[string]*QueryAnalysis),
		cacheTTL:  5 * time.Minute,
		cacheTime: make(map[string]time.Time),
	}
}

// getAnalysisSchema retorna o schema JSON para saída estruturada
func getAnalysisSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"intent": {
				Type:        genai.TypeString,
				Description: "O que o usuário quer fazer: buscar_servico, listar_categoria, ou esclarecer_duvida",
				Enum:        []string{"buscar_servico", "listar_categoria", "esclarecer_duvida"},
			},
			"keywords": {
				Type:        genai.TypeArray,
				Description: "Palavras-chave principais extraídas da query (máximo 5)",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"categories": {
				Type:        genai.TypeArray,
				Description: "Categorias prováveis do serviço buscado",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"refined_queries": {
				Type:        genai.TypeArray,
				Description: "Até 2 reformulações mais claras da query",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"search_strategy": {
				Type:        genai.TypeString,
				Description: "Estratégia de busca recomendada",
				Enum:        []string{"keyword", "semantic", "hybrid"},
			},
			"confidence": {
				Type:        genai.TypeNumber,
				Description: "Confiança na análise, de 0 a 1",
			},
		},
		Required: []string{"intent", "keywords", "search_strategy", "confidence"},
	}
}

// Analyze analisa a query com LLM usando saída estruturada
func (a *Analyzer) Analyze(ctx context.Context, query string) (*QueryAnalysis, error) {
	if a.client == nil {
		return a.defaultAnalysis(query), nil
	}

	// Verifica cache
	if cached, ok := a.getFromCache(query); ok {
		return cached, nil
	}

	// Timeout reduzido para não bloquear a busca
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`Analise esta query de busca de serviços públicos da Prefeitura do Rio de Janeiro:

Query: "%s"

Regras:
- intent: o que o usuário quer fazer
- keywords: termos-chave principais (max 5)
- categories: categorias prováveis do serviço buscado (ex: Educação, Saúde, Transporte, Tributos)
- refined_queries: max 2 reformulações mais claras da query
- search_strategy: keyword para termos exatos, semantic para conceituais, hybrid para misto
- confidence: 0-1 (quão claro é o intent)`, query)

	content := genai.NewContentFromText(prompt, genai.RoleUser)

	// Configuração para saída estruturada (structured output)
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   getAnalysisSchema(),
	}

	resp, err := a.client.Models.GenerateContent(ctx, a.model, []*genai.Content{content}, config)
	if err != nil {
		log.Printf("Erro ao analisar query: %v", err)
		return a.defaultAnalysis(query), nil
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return a.defaultAnalysis(query), nil
	}

	// Com saída estruturada, o JSON é garantido
	part := resp.Candidates[0].Content.Parts[0]
	jsonStr := fmt.Sprintf("%v", part)

	var analysis QueryAnalysis
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		log.Printf("Erro ao parsear análise estruturada: %v", err)
		return a.defaultAnalysis(query), nil
	}

	// Cache
	a.setCache(query, &analysis)

	return &analysis, nil
}

// defaultAnalysis retorna análise padrão quando LLM não está disponível
func (a *Analyzer) defaultAnalysis(query string) *QueryAnalysis {
	return &QueryAnalysis{
		Intent:         "buscar_servico",
		Keywords:       strings.Fields(query),
		Categories:     []string{},
		RefinedQueries: []string{},
		SearchStrategy: "hybrid",
		Confidence:     0.5,
	}
}

func (a *Analyzer) getFromCache(query string) (*QueryAnalysis, bool) {
	if cached, ok := a.cache[query]; ok {
		if time.Since(a.cacheTime[query]) < a.cacheTTL {
			return cached, true
		}
		delete(a.cache, query)
		delete(a.cacheTime, query)
	}
	return nil, false
}

func (a *Analyzer) setCache(query string, analysis *QueryAnalysis) {
	a.cache[query] = analysis
	a.cacheTime[query] = time.Now()
}

