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

// Analyze analisa a query com LLM
func (a *Analyzer) Analyze(ctx context.Context, query string) (*QueryAnalysis, error) {
	if a.client == nil {
		return a.defaultAnalysis(query), nil
	}

	// Verifica cache
	if cached, ok := a.getFromCache(query); ok {
		return cached, nil
	}

	// Timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`Analise esta query de busca de serviços públicos da Prefeitura do Rio de Janeiro:

Query: "%s"

Retorne JSON com:
{
  "intent": "buscar_servico|listar_categoria|esclarecer_duvida",
  "keywords": ["palavra1", "palavra2"],
  "categories": ["Educação", "Saúde", "Transporte"],
  "refined_queries": ["variação 1", "variação 2"],
  "search_strategy": "keyword|semantic|hybrid",
  "confidence": 0.85
}

Regras:
- intent: o que o usuário quer fazer
- keywords: termos-chave principais (max 5)
- categories: categorias prováveis do serviço buscado
- refined_queries: max 2 reformulações mais claras da query
- search_strategy: keyword para termos exatos, semantic para conceituais, hybrid para misto
- confidence: 0-1 (quão claro é o intent)

Retorne APENAS o JSON.`, query)

	content := genai.NewContentFromText(prompt, genai.RoleUser)

	resp, err := a.client.Models.GenerateContent(ctx, a.model, []*genai.Content{content}, nil)
	if err != nil {
		log.Printf("Erro ao analisar query: %v", err)
		return a.defaultAnalysis(query), nil
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return a.defaultAnalysis(query), nil
	}

	// Parse JSON
	part := resp.Candidates[0].Content.Parts[0]
	jsonStr := extractJSON(fmt.Sprintf("%v", part))

	var analysis QueryAnalysis
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		log.Printf("Erro ao parsear análise: %v", err)
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

// extractJSON extrai JSON de uma resposta que pode ter markdown
func extractJSON(s string) string {
	// Remove markdown code blocks
	if idx := strings.Index(s, "```json"); idx != -1 {
		s = s[idx+7:]
		if endIdx := strings.Index(s, "```"); endIdx != -1 {
			s = s[:endIdx]
		}
	} else if idx := strings.Index(s, "```"); idx != -1 {
		s = s[idx+3:]
		if endIdx := strings.Index(s, "```"); endIdx != -1 {
			s = s[:endIdx]
		}
	}

	// Encontra início do JSON
	if idx := strings.Index(s, "{"); idx != -1 {
		s = s[idx:]
	}

	return strings.TrimSpace(s)
}
