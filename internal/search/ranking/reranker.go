package ranking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"google.golang.org/genai"
)

// Reranker re-ordena resultados usando LLM
type Reranker struct {
	client *genai.Client
	model  string
}

// NewReranker cria um novo reranker
func NewReranker(client *genai.Client, model string) *Reranker {
	return &Reranker{
		client: client,
		model:  model,
	}
}

// getRerankSchema retorna o schema JSON para saída estruturada do reranking
func getRerankSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"ranked_ids": {
				Type:        genai.TypeArray,
				Description: "Array de IDs dos serviços ordenados por relevância (mais relevante primeiro)",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
		},
		Required: []string{"ranked_ids"},
	}
}

// Rerank re-ordena os top N resultados usando LLM com saída estruturada
func (r *Reranker) Rerank(ctx context.Context, query string, docs []v3.Document, topN int) ([]v3.Document, error) {
	if r.client == nil || len(docs) == 0 {
		return docs, nil
	}

	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}
	if topN > 5 {
		topN = 5 // Reduzido para melhorar performance e latência
	}

	// Prepara lista para o LLM (formato mais compacto)
	topDocs := docs[:topN]
	services := make([]string, len(topDocs))
	for i, doc := range topDocs {
		// Limita descrição para reduzir tokens
		desc := doc.Description
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		services[i] = fmt.Sprintf("%d. [%s] %s: %s", i+1, doc.ID, doc.Title, desc)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`Reordene estes serviços por relevância para a query do usuário.

Query: "%s"

Serviços:
%s

Retorne os IDs na ordem de relevância (mais relevante primeiro).`, query, strings.Join(services, "\n"))

	content := genai.NewContentFromText(prompt, genai.RoleUser)

	// Configuração para saída estruturada
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   getRerankSchema(),
	}

	resp, err := r.client.Models.GenerateContent(ctx, r.model, []*genai.Content{content}, config)
	if err != nil {
		log.Printf("Erro no rerank: %v", err)
		return docs, nil
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return docs, nil
	}

	// Com saída estruturada, o JSON é garantido
	part := resp.Candidates[0].Content.Parts[0]
	jsonStr := fmt.Sprintf("%v", part)

	var result struct {
		RankedIDs []string `json:"ranked_ids"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Printf("Erro ao parsear rerank estruturado: %v", err)
		return docs, nil
	}

	// Reordena baseado nos IDs
	reranked := make([]v3.Document, 0, len(docs))
	idMap := make(map[string]v3.Document)
	for _, doc := range topDocs {
		idMap[doc.ID] = doc
	}

	for _, id := range result.RankedIDs {
		if doc, exists := idMap[id]; exists {
			reranked = append(reranked, doc)
			delete(idMap, id)
		}
	}

	// Adiciona docs não ranqueados
	for _, doc := range topDocs {
		if _, exists := idMap[doc.ID]; exists {
			reranked = append(reranked, doc)
		}
	}

	// Adiciona o resto (além do topN)
	if len(docs) > topN {
		reranked = append(reranked, docs[topN:]...)
	}

	return reranked, nil
}

