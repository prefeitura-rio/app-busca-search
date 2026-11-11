package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search"
	"github.com/prefeitura-rio/app-busca-search/internal/search/adapter"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	"github.com/typesense/typesense-go/v3/typesense"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/genai"
)

// retiredModel é o modelo que a API do Gemini desligou em 14/01/2026 (DEV-511).
const retiredModel = "text-embedding-004"

// geminiRetiredMessage é a mensagem que a API do Gemini devolve para o modelo desligado;
// nenhum pedaço dela pode chegar ao cliente da busca.
const geminiRetiredMessage = "models/text-embedding-004 is not found for API version v1beta, or is not supported for embedContent. Call ModelService.ListModels to see the list of available models and their supported methods."

// fakeGemini responde como a API do Gemini: 404 para modelo desligado e um vetor de
// 768 dimensões para o vigente, guardando o modelo e a dimensão pedidos.
type fakeGemini struct {
	mu         sync.Mutex
	models     []string
	dimensions []int
	server     *httptest.Server
}

func newFakeGemini(t *testing.T) *fakeGemini {
	t.Helper()
	f := &fakeGemini{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		const prefix = "/v1beta/models/"
		const suffix = ":batchEmbedContents"
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":generateContent") {
			// Análise da busca AI (v1): o LLM escolhe a estratégia semantic
			analysis := "```json\n{\"intent\":\"buscar_servico\",\"keywords\":[\"exame\"],\"search_strategy\":\"semantic\",\"confidence\":0.9}\n```"
			_ = json.NewEncoder(w).Encode(map[string]any{"candidates": []map[string]any{{
				"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": analysis}}},
			}}})
			return
		}
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, suffix) {
			http.Error(w, "rota inesperada: "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
			return
		}
		model := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)

		var body struct {
			Requests []struct {
				OutputDimensionality int `json:"outputDimensionality"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Requests) != 1 {
			http.Error(w, "corpo inesperado", http.StatusBadRequest)
			return
		}
		f.record(model, body.Requests[0].OutputDimensionality)

		if model != config.DefaultGeminiEmbeddingModel {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"code": 404, "message": geminiRetiredMessage, "status": "NOT_FOUND",
			}})
			return
		}

		values := make([]float64, body.Requests[0].OutputDimensionality)
		for i := range values {
			values[i] = 0.01
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": []map[string]any{{"values": values}}})
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGemini) record(model string, dim int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.models = append(f.models, model)
	f.dimensions = append(f.dimensions, dim)
}

func (f *fakeGemini) calls() ([]string, []int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.models...), append([]int(nil), f.dimensions...)
}

func (f *fakeGemini) client(t *testing.T) *genai.Client {
	t.Helper()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: f.server.URL},
	})
	if err != nil {
		t.Fatalf("genai.NewClient: %v", err)
	}
	return client
}

// vectorTypesense responde como o Typesense com resultados vazios e conta as buscas que
// levaram vector_query, para provar que a busca usou de fato o componente vetorial.
type vectorTypesense struct {
	mu            sync.Mutex
	vectorQueries int
	textSearches  int
	server        *httptest.Server
}

func newVectorTypesense(t *testing.T) *vectorTypesense {
	t.Helper()
	f := &vectorTypesense{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/documents/search"):
			f.count(false)
			_, _ = io.WriteString(w, `{"found":0,"out_of":0,"page":1,"search_time_ms":0,"hits":[],"facet_counts":[],"request_params":{"collection_name":"x","per_page":0,"q":"*"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/multi_search":
			var body struct {
				Searches []map[string]interface{} `json:"searches"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			results := make([]string, 0, len(body.Searches))
			for _, s := range body.Searches {
				vq, _ := s["vector_query"].(string)
				f.count(strings.HasPrefix(vq, "embedding:(["))
				results = append(results, `{"found":0,"hits":[]}`)
			}
			_, _ = io.WriteString(w, `{"results":[`+strings.Join(results, ",")+`]}`)
		default:
			http.Error(w, "rota inesperada: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *vectorTypesense) count(vector bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if vector {
		f.vectorQueries++
	} else {
		f.textSearches++
	}
}

func (f *vectorTypesense) vectorCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vectorQueries
}

func (f *vectorTypesense) client() *typesense.Client {
	return typesense.NewClient(typesense.WithServer(f.server.URL), typesense.WithAPIKey("test-key"))
}

// embeddingRouter monta /api/v1, /api/v2 e /api/v3/search como em routes.go, com o
// modelo de embeddings dado, contra o Gemini e o Typesense falsos.
func embeddingRouter(t *testing.T, ts *vectorTypesense, gemini *fakeGemini, embeddingModel string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tsClient := ts.client()
	geminiClient := gemini.client(t)
	cache := services.NewLRUCache(10)

	searchService := services.NewSearchService(tsClient, geminiClient, embeddingModel, "chat-model", cache, ts.server.URL, "test-key")
	v1 := NewSearchHandler(searchService, nil)

	cfg := &config.Config{
		SearchableCollections: []string{"prefrio_services_base"},
		CollectionConfigs: map[string]*config.CollectionConfig{
			"prefrio_services_base": {Type: "service", FilterField: "status", FilterValue: "1"},
		},
	}
	v2Service := services.NewSearchServiceV2(tsClient, services.NewGeminiEmbeddingProvider(geminiClient, embeddingModel, cache), cfg)
	v2 := NewSearchHandlerV2(v2Service)

	engine := search.NewEngine(
		adapter.NewTypesenseAdapter(tsClient, ts.server.URL, "test-key"),
		adapter.NewGeminiAdapter(geminiClient, adapter.GeminiConfig{EmbeddingModel: embeddingModel}),
		nil,
		nil,
		map[string]*v3.CollectionConfig{"prefrio_services_base": v3.DefaultPrefRioConfig()},
		"prefrio_services_base",
	)
	v3Handler := NewSearchHandlerV3(engine)

	r := gin.New()
	r.GET("/api/v1/search", v1.Search)
	r.GET("/api/v2/search", v2.Search)
	r.GET("/api/v3/search", v3Handler.Search)
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// captureLog devolve o que o pacote log escrever durante o teste.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buf
}

// responseSearchType devolve o search_type da resposta do v1 e do v2; a resposta do v3
// não tem esse campo (ok=false), e lá a prova é o vector_query no Typesense.
func responseSearchType(t *testing.T, version string, w *httptest.ResponseRecorder) (string, bool) {
	t.Helper()
	if version == "/api/v3/search" {
		return "", false
	}
	var body struct {
		SearchType string `json:"search_type"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	return body.SearchType, true
}

var searchVersions = []string{"/api/v1/search", "/api/v2/search", "/api/v3/search"}

// DEV-511: com o modelo vindo da configuração (default ou GEMINI_EMBEDDING_MODEL com o
// modelo desligado), type=semantic responde 200, o Gemini recebe o modelo vigente com
// 768 dimensões e o Typesense recebe a busca vetorial.
func TestSemanticSearch_ConfiguredModelWorks(t *testing.T) {
	for _, configured := range []string{"", retiredModel} {
		model := config.ResolveGeminiEmbeddingModel(configured)
		for _, version := range searchVersions {
			t.Run(version+" GEMINI_EMBEDDING_MODEL="+configured, func(t *testing.T) {
				ts := newVectorTypesense(t)
				gemini := newFakeGemini(t)
				r := embeddingRouter(t, ts, gemini, model)

				w := get(r, version+"?q=exame&type=semantic&per_page=2")
				if w.Code != http.StatusOK {
					t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
				}

				models, dims := gemini.calls()
				if len(models) == 0 {
					t.Fatal("o Gemini não recebeu nenhum pedido de embedding")
				}
				for i := range models {
					if models[i] != config.DefaultGeminiEmbeddingModel || dims[i] != 768 {
						t.Fatalf("pedido ao Gemini com modelo %q e %d dimensões, want %q e 768", models[i], dims[i], config.DefaultGeminiEmbeddingModel)
					}
				}
				if ts.vectorCount() == 0 {
					t.Fatal("o Typesense não recebeu nenhuma busca com vector_query")
				}
			})
		}
	}
}

// DEV-511: a busca híbrida volta a combinar texto e vetor (search_type=hybrid e
// vector_query enviado ao Typesense), sem cair para keyword.
func TestHybridSearch_UsesVectorWithConfiguredModel(t *testing.T) {
	model := config.ResolveGeminiEmbeddingModel(retiredModel)
	for _, version := range searchVersions {
		t.Run(version, func(t *testing.T) {
			ts := newVectorTypesense(t)
			gemini := newFakeGemini(t)
			r := embeddingRouter(t, ts, gemini, model)
			logs := captureLog(t)

			w := get(r, version+"?q=exame&type=hybrid&per_page=2")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			if ts.vectorCount() == 0 {
				t.Fatal("a busca híbrida não enviou vector_query ao Typesense")
			}
			if got, ok := responseSearchType(t, version, w); ok && got != "hybrid" {
				t.Fatalf("search_type = %q, want hybrid", got)
			}
			if strings.Contains(logs.String(), "search_degraded") {
				t.Fatalf("busca híbrida registrou degradação sem falha de embedding: %s", logs.String())
			}
		})
	}
}

// DEV-511: quando o embedding falha, type=semantic responde 503 sem vazar a mensagem
// do Gemini, e o erro original fica no log do servidor.
func TestSemanticSearch_EmbeddingFailureIsNotLeaked(t *testing.T) {
	for _, version := range searchVersions {
		t.Run(version, func(t *testing.T) {
			ts := newVectorTypesense(t)
			gemini := newFakeGemini(t)
			// Modelo passado direto, sem a resolução da configuração: o Gemini responde 404
			r := embeddingRouter(t, ts, gemini, retiredModel)
			logs := captureLog(t)

			w := get(r, version+"?q=exame&type=semantic&per_page=2")
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503; body = %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, leak := range []string{retiredModel, "404", "NOT_FOUND", "ListModels", "v1beta", "embedContent"} {
				if strings.Contains(body, leak) {
					t.Fatalf("resposta vaza %q do Gemini: %s", leak, body)
				}
			}
			if !strings.Contains(strings.ToLower(body), "busca semântica indisponível") {
				t.Fatalf("resposta sem a mensagem genérica: %s", body)
			}
			if !strings.Contains(logs.String(), "search_embedding_failed") || !strings.Contains(logs.String(), "NOT_FOUND") {
				t.Fatalf("log sem o erro original do embedding: %s", logs.String())
			}
			if ts.vectorCount() != 0 {
				t.Fatal("busca semântica sem embedding não deveria chegar ao Typesense")
			}
		})
	}
}

// DEV-511: no v1 e no v2 a falha 404 do modelo não é repetida (erro do pedido); o v3
// já não repetia.
func TestSemanticSearch_ClientErrorIsNotRetried(t *testing.T) {
	for _, version := range []string{"/api/v1/search", "/api/v2/search"} {
		t.Run(version, func(t *testing.T) {
			ts := newVectorTypesense(t)
			gemini := newFakeGemini(t)
			r := embeddingRouter(t, ts, gemini, retiredModel)
			captureLog(t)

			_ = get(r, version+"?q=exame&type=semantic&per_page=2")
			if models, _ := gemini.calls(); len(models) != 1 {
				t.Fatalf("o Gemini recebeu %d pedidos, want 1 (404 não se repete)", len(models))
			}
		})
	}
}

// DEV-511: quando o embedding falha, a busca híbrida cai para keyword, responde 200 com
// search_type=keyword e registra a degradação num formato estável (search_degraded).
func TestHybridSearch_FallbackIsVisible(t *testing.T) {
	for _, version := range searchVersions {
		t.Run(version, func(t *testing.T) {
			ts := newVectorTypesense(t)
			gemini := newFakeGemini(t)
			r := embeddingRouter(t, ts, gemini, retiredModel)
			logs := captureLog(t)

			w := get(r, version+"?q=exame&type=hybrid&per_page=2")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			if got, ok := responseSearchType(t, version, w); ok && got != "keyword" {
				t.Fatalf("search_type = %q, want keyword", got)
			}
			if ts.vectorCount() != 0 {
				t.Fatal("busca que caiu para keyword não deveria enviar vector_query")
			}
			out := logs.String()
			if !strings.Contains(out, "WARN search_degraded") || !strings.Contains(out, "requested=hybrid executed=keyword") {
				t.Fatalf("log sem a degradação padronizada: %s", out)
			}
			if strings.Contains(w.Body.String(), "NOT_FOUND") {
				t.Fatalf("resposta vaza o erro do Gemini: %s", w.Body.String())
			}
		})
	}
}

// DEV-511: no v1, a busca AI cuja estratégia é semantic não falha quando o embedding
// falha: segue pela híbrida, que cai para keyword e registra a degradação.
func TestAISearch_SemanticStrategyFallsBackWhenEmbeddingFails(t *testing.T) {
	ts := newVectorTypesense(t)
	gemini := newFakeGemini(t)
	r := embeddingRouter(t, ts, gemini, retiredModel)
	logs := captureLog(t)

	w := get(r, "/api/v1/search?q=exame&type=ai&per_page=2")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), "WARN search_degraded api=v1 requested=ai executed=keyword") {
		t.Fatalf("log sem a degradação da busca AI: %s", logs.String())
	}
	if strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("resposta vaza o erro do Gemini: %s", w.Body.String())
	}
}

// DEV-511: sem serviço de embeddings configurado, a busca híbrida cai para keyword e
// registra a degradação nos três endpoints.
func TestHybridSearch_NoEmbeddingServiceFallbackIsVisible(t *testing.T) {
	ts := newVectorTypesense(t)
	gin.SetMode(gin.TestMode)
	tsClient := ts.client()
	cfg := &config.Config{
		SearchableCollections: []string{"prefrio_services_base"},
		CollectionConfigs: map[string]*config.CollectionConfig{
			"prefrio_services_base": {Type: "service", FilterField: "status", FilterValue: "1"},
		},
	}
	engine := search.NewEngine(
		adapter.NewTypesenseAdapter(tsClient, ts.server.URL, "test-key"),
		nil, nil, nil,
		map[string]*v3.CollectionConfig{"prefrio_services_base": v3.DefaultPrefRioConfig()},
		"prefrio_services_base",
	)
	r := gin.New()
	r.GET("/api/v1/search", NewSearchHandler(services.NewSearchService(tsClient, nil, "", "", services.NewLRUCache(10), ts.server.URL, "test-key"), nil).Search)
	r.GET("/api/v2/search", NewSearchHandlerV2(services.NewSearchServiceV2(tsClient, nil, cfg)).Search)
	r.GET("/api/v3/search", NewSearchHandlerV3(engine).Search)

	for _, version := range searchVersions {
		t.Run(version, func(t *testing.T) {
			logs := captureLog(t)
			w := get(r, version+"?q=exame&type=hybrid&per_page=2")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			if !strings.Contains(logs.String(), "WARN search_degraded") || !strings.Contains(logs.String(), "reason=embedding_service_not_configured") {
				t.Fatalf("log sem a degradação padronizada: %s", logs.String())
			}
		})
	}
}

// DEV-511: a degradação do hybrid também fica no trace (search.degraded=true e o motivo),
// para alerta no SigNoz, nos três endpoints.
func TestHybridSearch_FallbackIsMarkedOnSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	ts := newVectorTypesense(t)
	gemini := newFakeGemini(t)
	inner := embeddingRouter(t, ts, gemini, retiredModel)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx, span := tp.Tracer("test").Start(c.Request.Context(), "http.request")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		span.End()
	})
	r.NoRoute(func(c *gin.Context) { inner.HandleContext(c) })
	captureLog(t)

	for _, version := range searchVersions {
		t.Run(version, func(t *testing.T) {
			before := len(recorder.Ended())
			w := get(r, version+"?q=exame&type=hybrid&per_page=2")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			for _, span := range recorder.Ended()[before:] {
				degraded, reason := false, ""
				for _, kv := range span.Attributes() {
					switch string(kv.Key) {
					case "search.degraded":
						degraded = kv.Value.AsBool()
					case "search.fallback_reason":
						reason = kv.Value.AsString()
					}
				}
				if degraded && reason == "embedding_unavailable" {
					return
				}
			}
			t.Fatal("nenhum span com search.degraded=true e search.fallback_reason=embedding_unavailable")
		})
	}
}
