package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	internalts "github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

// fakeTypesenseSearch imita o Typesense: recusa com 400 a busca GET cuja query string passa
// de 4000 caracteres e responde uma busca vazia nas demais. calls conta as buscas recebidas
// (as chamadas de verificação de coleções feitas na criação do cliente não contam).
func fakeTypesenseSearch(t *testing.T, calls *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/documents/search") {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		atomic.AddInt32(calls, 1)
		if len(r.URL.RawQuery) > 4000 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Query string exceeds max allowed length of 4000. Use the /multi_search end-point for larger payloads."}`))
			return
		}
		_, _ = w.Write([]byte(`{"found":0,"hits":[],"out_of":0,"page":1,"search_time_ms":0}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// searchV1Router monta só GET /api/v1/search com o serviço de busca apontando para typesenseURL,
// sem Gemini (hybrid e ai caem na busca keyword).
func searchV1Router(t *testing.T, typesenseURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	u, err := url.Parse(typesenseURL)
	if err != nil {
		t.Fatalf("URL do Typesense: %v", err)
	}
	cfg := &config.Config{
		TypesenseProtocol: u.Scheme,
		TypesenseHost:     u.Hostname(),
		TypesensePort:     u.Port(),
		TypesenseAPIKey:   "test-key",
		GatewayBaseURL:    "https://gateway.test",
	}
	tsClient := internalts.NewClient(cfg)
	searchService := services.NewSearchService(tsClient.GetClient(), nil, "", "", services.NewLRUCache(10), typesenseURL, "test-key")
	r := gin.New()
	r.GET("/api/v1/search", NewSearchHandler(searchService, tsClient).Search)
	return r
}

func doSearchV1(router *gin.Engine, q, searchType string, extra ...string) *httptest.ResponseRecorder {
	params := url.Values{}
	params.Set("q", q)
	params.Set("type", searchType)
	for i := 0; i+1 < len(extra); i += 2 {
		params.Set(extra[i], extra[i+1])
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/search?"+params.Encode(), nil))
	return w
}

func assertNoTypesenseLeak(t *testing.T, raw string) {
	t.Helper()
	for _, leak := range []string{"Query string exceeds", "multi_search", "status: 400", "erro ao executar busca keyword", "127.0.0.1"} {
		if strings.Contains(raw, leak) {
			t.Errorf("resposta vaza %q do erro interno: %s", leak, raw)
		}
	}
}

// Texto de busca acima do limite responde 400 com mensagem genérica, sem chamar o Typesense
// e sem vazar o erro dele, em todos os tipos de busca da v1 (DEV-681).
func TestSearchV1QueryTooLongIsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		q    string
	}{
		{"5000 caracteres ASCII", strings.Repeat("a", 5000)},
		{"limite + 1 caractere", strings.Repeat("a", MaxSearchQueryLength+1)},
		{"abaixo do limite em caracteres, multibyte acima do limite codificado", strings.Repeat("😀", 292)},
	}
	for _, tt := range tests {
		for _, searchType := range []string{"keyword", "hybrid", "semantic", "ai"} {
			t.Run(tt.name+"/"+searchType, func(t *testing.T) {
				var calls int32
				router := searchV1Router(t, fakeTypesenseSearch(t, &calls).URL)
				w := doSearchV1(router, tt.q, searchType)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("status %d, esperado 400; corpo: %.300s", w.Code, w.Body.String())
				}
				var body map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("corpo não é JSON: %v; corpo: %s", err, w.Body.String())
				}
				if body["error"] != "Texto da busca muito longo" {
					t.Errorf("error = %v, esperado \"Texto da busca muito longo\"", body["error"])
				}
				assertNoTypesenseLeak(t, w.Body.String())
				if n := atomic.LoadInt32(&calls); n != 0 {
					t.Errorf("Typesense recebeu %d buscas, esperado nenhuma", n)
				}
			})
		}
	}
}

// Texto de busca no limite segue para o Typesense e responde 200, mesmo com os parâmetros
// que mais alongam a query string enviada ao Typesense.
func TestSearchV1QueryAtLimitIsSearched(t *testing.T) {
	portugues := []rune(strings.Repeat("certidão de nascimento e emissão de 2ª via ", 30))
	tests := []struct {
		name string
		q    string
	}{
		{"limite em caracteres ASCII", strings.Repeat("a", MaxSearchQueryLength)},
		{"limite em caracteres com acentos", string(portugues[:MaxSearchQueryLength])},
		{"multibyte no limite codificado", strings.Repeat("😀", 291)},
	}
	for _, tt := range tests {
		for _, searchType := range []string{"keyword", "hybrid"} {
			t.Run(tt.name+"/"+searchType, func(t *testing.T) {
				var calls int32
				router := searchV1Router(t, fakeTypesenseSearch(t, &calls).URL)
				w := doSearchV1(router, tt.q, searchType, "page", "999", "per_page", "100", "exclude_agent_exclusive", "true")

				if w.Code != http.StatusOK {
					t.Fatalf("status %d, esperado 200; corpo: %.300s", w.Code, w.Body.String())
				}
				if n := atomic.LoadInt32(&calls); n != 1 {
					t.Errorf("Typesense recebeu %d buscas, esperado 1", n)
				}
			})
		}
	}
}

// Falha do Typesense responde 500 com mensagem genérica, sem o campo details.
func TestSearchV1TypesenseFailureHasNoDetails(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Query string exceeds max allowed length of 4000. Use the /multi_search end-point for larger payloads."}`))
	}))
	t.Cleanup(failing.Close)

	router := searchV1Router(t, failing.URL)
	w := doSearchV1(router, "iptu", "keyword")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, esperado 500; corpo: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("corpo não é JSON: %v; corpo: %s", err, w.Body.String())
	}
	if len(body) != 1 || body["error"] != "Erro ao executar busca" {
		t.Errorf("corpo = %s, esperado só {\"error\": \"Erro ao executar busca\"}", w.Body.String())
	}
	assertNoTypesenseLeak(t, w.Body.String())
}
