package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	internalts "github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

// searchByIDRouter monta só GET /api/v1/search/:id com o cliente apontando para typesenseURL.
func searchByIDRouter(t *testing.T, typesenseURL string) *gin.Engine {
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
	r := gin.New()
	r.GET("/api/v1/search/:id", NewSearchHandler(nil, internalts.NewClient(cfg)).GetDocumentByID)
	return r
}

// Falha do Typesense que não é documento inexistente (5xx, servidor fora do ar) responde
// 500 com mensagem genérica, sem o detalhe do erro, e não 404 (DEV-627).
func TestGetDocumentByIDTypesenseFailureIsInternalError(t *testing.T) {
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"message":"Not Ready or Lagging"}`, http.StatusServiceUnavailable)
	}))
	t.Cleanup(unavailable.Close)

	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()

	tests := []struct {
		name string
		url  string
	}{
		{"typesense responde 503", unavailable.URL},
		{"typesense fora do ar", downURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := searchByIDRouter(t, tt.url)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/search/pub-1", nil))

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, esperado 500; corpo: %s", w.Code, w.Body.String())
			}
			var body map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("corpo não é JSON: %v; corpo: %s", err, w.Body.String())
			}
			if len(body) != 1 || body["error"] != "Erro ao buscar serviço" {
				t.Errorf("corpo = %s, esperado só {\"error\": \"Erro ao buscar serviço\"}", w.Body.String())
			}
			raw := w.Body.String()
			for _, leak := range []string{"details", "Not Ready", "status: 503", "connection refused", "127.0.0.1"} {
				if strings.Contains(raw, leak) {
					t.Errorf("resposta vaza %q do erro interno: %s", leak, raw)
				}
			}
		})
	}
}
