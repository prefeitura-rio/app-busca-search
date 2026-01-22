package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/prefeitura-rio/app-busca-search/internal/search/content"
	"google.golang.org/genai"
)

type ReindexConfig struct {
	Collection  string
	Mode        string // all, content-only, embedding-only
	BatchSize   int
	Workers     int
	DryRun      bool
	MissingOnly bool
	DocumentID  string
	Force       bool
}

type ReindexStats struct {
	Total     int64
	Processed int64
	Skipped   int64
	Errors    int64
	StartTime time.Time
}

type Reindexer struct {
	config         *ReindexConfig
	contentGen     *content.Generator
	geminiClient   *genai.Client
	embeddingModel string
	typesenseURL   string
	typesenseKey   string
	stats          *ReindexStats
}

func main() {
	// Flags
	collection := flag.String("collection", "prefrio_services_base", "Collection alvo")
	mode := flag.String("mode", "all", "Modo: all, content-only, embedding-only")
	batchSize := flag.Int("batch", 50, "Documentos por batch")
	workers := flag.Int("workers", 3, "Workers paralelos")
	dryRun := flag.Bool("dry-run", false, "Simular sem alterar")
	missingOnly := flag.Bool("missing-only", false, "Apenas documentos sem embedding")
	documentID := flag.String("id", "", "Reindexar documento específico")
	force := flag.Bool("force", false, "Forçar mesmo se já existe")

	flag.Parse()

	// Carrega .env
	_ = godotenv.Load()

	cfg := config.LoadConfig()

	reindexCfg := &ReindexConfig{
		Collection:  *collection,
		Mode:        *mode,
		BatchSize:   *batchSize,
		Workers:     *workers,
		DryRun:      *dryRun,
		MissingOnly: *missingOnly,
		DocumentID:  *documentID,
		Force:       *force,
	}

	reindexer, err := NewReindexer(reindexCfg, cfg)
	if err != nil {
		log.Fatalf("Erro ao criar reindexer: %v", err)
	}

	ctx := context.Background()
	if err := reindexer.Run(ctx); err != nil {
		log.Fatalf("Erro na reindexação: %v", err)
	}
}

func NewReindexer(cfg *ReindexConfig, appCfg *config.Config) (*Reindexer, error) {
	// Cliente Gemini
	var geminiClient *genai.Client
	if cfg.Mode != "content-only" {
		ctx := context.Background()
		client, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey: appCfg.GeminiAPIKey,
		})
		if err != nil {
			return nil, fmt.Errorf("erro ao criar cliente Gemini: %w", err)
		}
		geminiClient = client
	}

	typesenseURL := fmt.Sprintf("%s://%s:%s",
		appCfg.TypesenseProtocol,
		appCfg.TypesenseHost,
		appCfg.TypesensePort,
	)

	return &Reindexer{
		config:         cfg,
		contentGen:     content.NewGenerator(content.DefaultConfig()),
		geminiClient:   geminiClient,
		embeddingModel: appCfg.GeminiEmbeddingModel,
		typesenseURL:   typesenseURL,
		typesenseKey:   appCfg.TypesenseAPIKey,
		stats:          &ReindexStats{StartTime: time.Now()},
	}, nil
}

func (r *Reindexer) Run(ctx context.Context) error {
	log.Printf("Iniciando reindexação...")
	log.Printf("Collection: %s", r.config.Collection)
	log.Printf("Modo: %s", r.config.Mode)
	log.Printf("Batch size: %d", r.config.BatchSize)
	log.Printf("Workers: %d", r.config.Workers)
	log.Printf("Dry-run: %v", r.config.DryRun)
	log.Printf("Missing-only: %v", r.config.MissingOnly)

	if r.config.DocumentID != "" {
		return r.reindexDocument(ctx, r.config.DocumentID)
	}

	return r.reindexAll(ctx)
}

func (r *Reindexer) reindexDocument(ctx context.Context, id string) error {
	log.Printf("Reindexando documento: %s", id)

	doc, err := r.getDocument(ctx, id)
	if err != nil {
		return fmt.Errorf("erro ao buscar documento: %w", err)
	}

	return r.processDocument(ctx, doc)
}

func (r *Reindexer) reindexAll(ctx context.Context) error {
	page := 1
	perPage := r.config.BatchSize

	var wg sync.WaitGroup
	docChan := make(chan map[string]interface{}, r.config.Workers*2)

	// Inicia workers
	for i := 0; i < r.config.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for doc := range docChan {
				if err := r.processDocument(ctx, doc); err != nil {
					log.Printf("Worker %d - Erro: %v", workerID, err)
					atomic.AddInt64(&r.stats.Errors, 1)
				}
			}
		}(i)
	}

	// Busca e enfileira documentos
	for {
		docs, total, err := r.searchDocuments(ctx, page, perPage)
		if err != nil {
			return fmt.Errorf("erro ao buscar documentos: %w", err)
		}

		if page == 1 {
			atomic.StoreInt64(&r.stats.Total, int64(total))
			log.Printf("Total de documentos: %d", total)
		}

		if len(docs) == 0 {
			break
		}

		for _, doc := range docs {
			if r.config.MissingOnly {
				if _, hasEmbedding := doc["embedding"]; hasEmbedding && !r.config.Force {
					atomic.AddInt64(&r.stats.Skipped, 1)
					continue
				}
			}
			docChan <- doc
		}

		// Log de progresso
		processed := atomic.LoadInt64(&r.stats.Processed)
		skipped := atomic.LoadInt64(&r.stats.Skipped)
		errors := atomic.LoadInt64(&r.stats.Errors)
		log.Printf("Progresso: %d/%d processados, %d ignorados, %d erros",
			processed, total, skipped, errors)

		page++
	}

	close(docChan)
	wg.Wait()

	r.printStats()
	return nil
}

func (r *Reindexer) processDocument(ctx context.Context, doc map[string]interface{}) error {
	id, ok := doc["id"].(string)
	if !ok {
		return fmt.Errorf("documento sem ID")
	}

	updates := make(map[string]interface{})

	// Gera search_content se necessário
	if r.config.Mode == "all" || r.config.Mode == "content-only" {
		searchContent := r.contentGen.GenerateFromMap(doc)
		updates["search_content"] = searchContent
	}

	// Gera embedding se necessário
	if r.config.Mode == "all" || r.config.Mode == "embedding-only" {
		searchContent, ok := doc["search_content"].(string)
		if !ok && r.config.Mode == "embedding-only" {
			// Usa o conteúdo atual se disponível
			if sc, exists := updates["search_content"]; exists {
				searchContent = sc.(string)
			} else {
				searchContent = r.contentGen.GenerateFromMap(doc)
			}
		} else if !ok {
			searchContent = updates["search_content"].(string)
		}

		if r.geminiClient != nil && searchContent != "" {
			embedding, err := r.generateEmbedding(ctx, searchContent)
			if err != nil {
				log.Printf("Erro ao gerar embedding para %s: %v", id, err)
			} else {
				updates["embedding"] = embedding
			}
		}
	}

	if len(updates) == 0 {
		atomic.AddInt64(&r.stats.Skipped, 1)
		return nil
	}

	if r.config.DryRun {
		log.Printf("[DRY-RUN] Atualizaria documento %s com %d campos", id, len(updates))
		atomic.AddInt64(&r.stats.Processed, 1)
		return nil
	}

	if err := r.updateDocument(ctx, id, updates); err != nil {
		return fmt.Errorf("erro ao atualizar documento %s: %w", id, err)
	}

	atomic.AddInt64(&r.stats.Processed, 1)
	return nil
}

func (r *Reindexer) getDocument(ctx context.Context, id string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/collections/%s/documents/%s", r.typesenseURL, r.config.Collection, id)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-TYPESENSE-API-KEY", r.typesenseKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var doc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}

	return doc, nil
}

func (r *Reindexer) searchDocuments(ctx context.Context, page, perPage int) ([]map[string]interface{}, int, error) {
	url := fmt.Sprintf("%s/collections/%s/documents/search?q=*&per_page=%d&page=%d",
		r.typesenseURL, r.config.Collection, perPage, page)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-TYPESENSE-API-KEY", r.typesenseKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Found int `json:"found"`
		Hits  []struct {
			Document map[string]interface{} `json:"document"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	docs := make([]map[string]interface{}, len(result.Hits))
	for i, hit := range result.Hits {
		docs[i] = hit.Document
	}

	return docs, result.Found, nil
}

func (r *Reindexer) updateDocument(ctx context.Context, id string, updates map[string]interface{}) error {
	url := fmt.Sprintf("%s/collections/%s/documents/%s", r.typesenseURL, r.config.Collection, id)

	jsonBody, err := json.Marshal(updates)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("X-TYPESENSE-API-KEY", r.typesenseKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (r *Reindexer) generateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if r.geminiClient == nil {
		return nil, fmt.Errorf("cliente Gemini não disponível")
	}

	// Limita tamanho do texto
	if len(text) > 10000 {
		text = text[:10000]
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	model := r.embeddingModel
	if model == "" {
		model = "text-embedding-004"
	}

	content := genai.NewContentFromText(text, genai.RoleUser)
	outputDim := int32(768)
	embedConfig := &genai.EmbedContentConfig{
		OutputDimensionality: &outputDim,
	}

	result, err := r.geminiClient.Models.EmbedContent(ctx, model, []*genai.Content{content}, embedConfig)
	if err != nil {
		return nil, err
	}

	if result.Embeddings == nil || len(result.Embeddings) == 0 || len(result.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("embedding vazio")
	}

	return result.Embeddings[0].Values, nil
}

func (r *Reindexer) printStats() {
	duration := time.Since(r.stats.StartTime)
	log.Println("\n=== Estatísticas de Reindexação ===")
	log.Printf("Total de documentos: %d", r.stats.Total)
	log.Printf("Processados: %d", r.stats.Processed)
	log.Printf("Ignorados: %d", r.stats.Skipped)
	log.Printf("Erros: %d", r.stats.Errors)
	log.Printf("Tempo total: %v", duration)
	if r.stats.Processed > 0 {
		log.Printf("Tempo médio por documento: %v", duration/time.Duration(r.stats.Processed))
	}

	if r.config.DryRun {
		log.Println("\n⚠️  Modo DRY-RUN - nenhuma alteração foi feita")
	}
}
