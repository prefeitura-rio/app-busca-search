package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	internalts "github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

// Mensagem que o Typesense falso devolve nos erros: nunca pode chegar ao cliente da API.
const versionFakeTypesenseLeak = "detalhe-interno-do-typesense"

// Documentos sintéticos do Typesense falso das rotas de versão, por collection e id.
var versionFixtures = map[string]map[string]map[string]interface{}{
	"prefrio_services_base": {
		"svc-1": {"id": "svc-1", "nome_servico": "Servico Versionado", "resumo": "resumo", "status": 1},
	},
	"service_versions": {
		"svc-1-v1": {
			"id": "svc-1-v1", "service_id": "svc-1", "version_number": 1, "change_type": "create",
			"created_by": "Autor Teste", "nome_servico": "Servico Versionado", "resumo": "resumo",
		},
		"svc-1-v2": {
			"id": "svc-1-v2", "service_id": "svc-1", "version_number": 2, "change_type": "update",
			"created_by": "Autor Teste", "nome_servico": "Servico Versionado 2", "resumo": "resumo",
		},
	},
}

// newVersionFakeTypesense responde como o Typesense para leitura por id e busca com filter_by
// a partir de versionFixtures. Com failAll, toda requisição responde 503, como um Typesense fora.
func newVersionFakeTypesense(t *testing.T, failAll bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if failAll {
			http.Error(w, `{"message":"`+versionFakeTypesenseLeak+`"}`, http.StatusServiceUnavailable)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method != http.MethodGet || len(parts) != 4 || parts[0] != "collections" || parts[2] != "documents" {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		docs := versionFixtures[parts[1]]
		if parts[3] == "search" {
			perPage := 0
			if v := r.URL.Query().Get("per_page"); v == "1" {
				perPage = 1
			}
			writeDetailSearch(w, docs, r.URL.Query().Get("filter_by"), perPage)
			return
		}
		doc, ok := docs[parts[3]]
		if !ok {
			http.Error(w, `{"message":"Could not find a document with id: `+parts[3]+` `+versionFakeTypesenseLeak+`"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(server.Close)
	return server
}

// versionRouter monta as rotas de versão como em routes.go, sem middleware, apontando para o
// Typesense falso.
func versionRouter(t *testing.T, failAll bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := newVersionFakeTypesense(t, failAll)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("URL do Typesense falso: %v", err)
	}
	cfg := &config.Config{
		TypesenseProtocol: u.Scheme,
		TypesenseHost:     u.Hostname(),
		TypesensePort:     u.Port(),
		TypesenseAPIKey:   "test-key",
		GatewayBaseURL:    "https://gateway.test",
	}
	handler := NewVersionHandler(internalts.NewClient(cfg))

	r := gin.New()
	r.GET("/api/v1/admin/services/:id/versions", handler.ListServiceVersions)
	r.GET("/api/v1/admin/services/:id/versions/:version", handler.GetServiceVersion)
	r.GET("/api/v1/admin/services/:id/versions/compare", handler.CompareServiceVersions)
	return r
}

func TestVersionRoutesNotFoundAndInternalErrors(t *testing.T) {
	const missing = "00000000-0000-0000-0000-000000000000"
	tests := []struct {
		name      string
		failAll   bool
		path      string
		wantCode  int
		wantError string
	}{
		{
			name:      "listar versões de serviço inexistente responde 404",
			path:      "/api/v1/admin/services/" + missing + "/versions",
			wantCode:  http.StatusNotFound,
			wantError: "Serviço não encontrado",
		},
		{
			name:      "comparar versões de serviço inexistente responde 404",
			path:      "/api/v1/admin/services/" + missing + "/versions/compare?from_version=1&to_version=2",
			wantCode:  http.StatusNotFound,
			wantError: "Serviço não encontrado",
		},
		{
			name:      "comparar com versão inexistente de serviço existente responde 404",
			path:      "/api/v1/admin/services/svc-1/versions/compare?from_version=1&to_version=9",
			wantCode:  http.StatusNotFound,
			wantError: "Versão não encontrada",
		},
		{
			name:      "listar versões com Typesense fora responde 500 sem detalhe interno",
			failAll:   true,
			path:      "/api/v1/admin/services/svc-1/versions",
			wantCode:  http.StatusInternalServerError,
			wantError: "Erro ao listar versões",
		},
		{
			name:      "comparar versões com Typesense fora responde 500 sem detalhe interno",
			failAll:   true,
			path:      "/api/v1/admin/services/svc-1/versions/compare?from_version=1&to_version=2",
			wantCode:  http.StatusInternalServerError,
			wantError: "Erro ao comparar versões",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := versionRouter(t, tt.failAll)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))

			body, _ := io.ReadAll(w.Body)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, esperado %d; corpo: %s", w.Code, tt.wantCode, body)
			}
			var resp map[string]string
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("corpo não é JSON: %v; corpo: %s", err, body)
			}
			if resp["error"] != tt.wantError {
				t.Errorf("error = %q, esperado %q", resp["error"], tt.wantError)
			}
			for _, leak := range []string{versionFakeTypesenseLeak, "status:", "response:", "Could not find"} {
				if strings.Contains(string(body), leak) {
					t.Errorf("corpo vaza erro interno (%q): %s", leak, body)
				}
			}
		})
	}
}

func TestVersionRoutesExistingServiceStillWork(t *testing.T) {
	router := versionRouter(t, false)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/svc-1/versions", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("listar: status = %d, esperado 200; corpo: %s", w.Code, w.Body.String())
	}
	var history struct {
		Found    int `json:"found"`
		Versions []struct {
			ServiceID string `json:"service_id"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &history); err != nil {
		t.Fatalf("listar: corpo não é JSON: %v", err)
	}
	if history.Found != 2 || len(history.Versions) != 2 || history.Versions[0].ServiceID != "svc-1" {
		t.Errorf("listar: histórico inesperado: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/svc-1/versions/compare?from_version=1&to_version=2", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("comparar: status = %d, esperado 200; corpo: %s", w.Code, w.Body.String())
	}
	var diff struct {
		FromVersion int64 `json:"from_version"`
		ToVersion   int64 `json:"to_version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &diff); err != nil {
		t.Fatalf("comparar: corpo não é JSON: %v", err)
	}
	if diff.FromVersion != 1 || diff.ToVersion != 2 {
		t.Errorf("comparar: diff inesperado: %s", w.Body.String())
	}
}
