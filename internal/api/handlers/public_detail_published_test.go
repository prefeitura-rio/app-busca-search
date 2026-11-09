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
	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search"
	"github.com/prefeitura-rio/app-busca-search/internal/search/adapter"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	internalts "github.com/prefeitura-rio/app-busca-search/internal/typesense"
	"github.com/typesense/typesense-go/v3/typesense"
)

// Documentos sintéticos do Typesense falso, por collection e id.
var detailFixtures = map[string]map[string]map[string]interface{}{
	"prefrio_services_base": {
		"pub-1": {
			"id": "pub-1", "nome_servico": "Servico Publicado", "resumo": "publicado",
			"status": 1, "slug": "servico-publicado", "slug_history": []string{"servico-publicado-antigo"},
		},
		"draft-1": {
			"id": "draft-1", "nome_servico": "Servico Rascunho", "resumo": "rascunho",
			"status": 0, "slug": "servico-rascunho", "slug_history": []string{"servico-rascunho-antigo"},
		},
	},
	// Collection configurada sem filtro de publicação: não tem estado de publicação.
	"cursos": {
		"curso-1": {"id": "curso-1", "title": "Curso", "description": "curso"},
	},
	// Collection interna, não configurada para a busca pública.
	"service_versions": {
		"ver-1": {"id": "ver-1", "nome_servico": "Servico Rascunho", "status": 1},
	},
}

// newDetailFakeTypesense responde como o Typesense para leitura por id, busca por slug
// (slug:= e slug_history:=) e verificação de collections, a partir de detailFixtures.
func newDetailFakeTypesense(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method != http.MethodGet || len(parts) < 2 || parts[0] != "collections" {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		docs, known := detailFixtures[parts[1]]
		switch {
		case len(parts) == 2 && known:
			_, _ = io.WriteString(w, `{"name":"`+parts[1]+`","fields":[],"num_documents":0}`)
		case len(parts) == 4 && parts[2] == "documents" && parts[3] == "search":
			writeDetailSearch(w, docs, r.URL.Query().Get("filter_by"))
		case len(parts) == 4 && parts[2] == "documents":
			doc, ok := docs[parts[3]]
			if !ok {
				http.Error(w, `{"message":"Could not find a document with id: `+parts[3]+`"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(doc)
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeDetailSearch(w http.ResponseWriter, docs map[string]map[string]interface{}, filterBy string) {
	hits := make([]map[string]interface{}, 0)
	for _, doc := range docs {
		switch {
		case strings.HasPrefix(filterBy, "slug:="):
			if doc["slug"] == strings.TrimPrefix(filterBy, "slug:=") {
				hits = append(hits, map[string]interface{}{"document": doc})
			}
		case strings.HasPrefix(filterBy, "slug_history:="):
			history, _ := doc["slug_history"].([]string)
			for _, old := range history {
				if old == strings.TrimPrefix(filterBy, "slug_history:=") {
					hits = append(hits, map[string]interface{}{"document": doc})
				}
			}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"found": len(hits), "out_of": len(docs), "page": 1, "search_time_ms": 0, "hits": hits,
		"request_params": map[string]interface{}{"collection_name": "x", "per_page": 1, "q": "*"},
	})
}

// detailPublicRouter monta as rotas públicas de detalhe como em routes.go, sem nenhum
// middleware de autenticação, apontando para o Typesense falso.
func detailPublicRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := newDetailFakeTypesense(t)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("URL do Typesense falso: %v", err)
	}

	cfg := &config.Config{
		TypesenseProtocol:     u.Scheme,
		TypesenseHost:         u.Hostname(),
		TypesensePort:         u.Port(),
		TypesenseAPIKey:       "test-key",
		GatewayBaseURL:        "https://gateway.test",
		SearchableCollections: []string{"prefrio_services_base", "cursos"},
		CollectionConfigs: map[string]*config.CollectionConfig{
			"prefrio_services_base": {Type: "service", TitleField: "nome_servico", DescField: "resumo", FilterField: "status", FilterValue: "1"},
			"cursos":                {Type: "course", TitleField: "title", DescField: "description"},
		},
	}
	rawClient := typesense.NewClient(typesense.WithServer(server.URL), typesense.WithAPIKey("test-key"))

	searchHandler := NewSearchHandler(nil, internalts.NewClient(cfg))
	searchHandlerV2 := NewSearchHandlerV2(services.NewSearchServiceV2(rawClient, nil, cfg))
	engine := search.NewEngine(
		adapter.NewTypesenseAdapter(rawClient, server.URL, "test-key"),
		nil,
		nil,
		nil,
		map[string]*v3.CollectionConfig{
			"prefrio_services_base": v3.DefaultPrefRioConfig(),
			"cursos":                {Name: "cursos", Type: "course", TitleField: "title", DescField: "description"},
		},
		"prefrio_services_base",
	)
	searchHandlerV3 := NewSearchHandlerV3(engine)

	r := gin.New()
	r.GET("/api/v1/search/:id", searchHandler.GetDocumentByID)
	r.GET("/api/v1/services/:slug", searchHandler.GetServiceBySlug)
	r.GET("/api/v2/search/:id", searchHandlerV2.GetDocumentByID)
	r.GET("/api/v3/search/:id", searchHandlerV3.GetDocument)
	return r
}

func TestPublicDetailRoutesOnlyReturnPublishedServices(t *testing.T) {
	router := detailPublicRouter(t)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string // trecho que a resposta deve conter
	}{
		{"v1 id publicado", "/api/v1/search/pub-1", http.StatusOK, "Servico Publicado"},
		{"v1 id rascunho", "/api/v1/search/draft-1", http.StatusNotFound, ""},
		{"v1 id inexistente", "/api/v1/search/nao-existe", http.StatusNotFound, ""},
		{"v1 slug publicado", "/api/v1/services/servico-publicado", http.StatusOK, "Servico Publicado"},
		{"v1 slug rascunho", "/api/v1/services/servico-rascunho", http.StatusNotFound, ""},
		{"v1 slug historico publicado", "/api/v1/services/servico-publicado-antigo", http.StatusMovedPermanently, "servico-publicado"},
		{"v1 slug historico rascunho", "/api/v1/services/servico-rascunho-antigo", http.StatusNotFound, ""},
		{"v2 id publicado", "/api/v2/search/pub-1", http.StatusOK, "Servico Publicado"},
		{"v2 id rascunho", "/api/v2/search/draft-1", http.StatusNotFound, ""},
		{"v2 id rascunho com hint", "/api/v2/search/draft-1?collection=prefrio_services_base", http.StatusNotFound, ""},
		{"v2 collection sem filtro de publicacao", "/api/v2/search/curso-1", http.StatusOK, "curso-1"},
		{"v2 hint de collection nao configurada", "/api/v2/search/ver-1?collection=service_versions", http.StatusNotFound, ""},
		{"v3 id publicado", "/api/v3/search/pub-1", http.StatusOK, "Servico Publicado"},
		{"v3 id rascunho", "/api/v3/search/draft-1", http.StatusNotFound, ""},
		{"v3 id rascunho com hint", "/api/v3/search/draft-1?collection=prefrio_services_base", http.StatusNotFound, ""},
		{"v3 collection sem filtro de publicacao", "/api/v3/search/curso-1", http.StatusOK, "curso-1"},
		{"v3 hint de collection nao configurada", "/api/v3/search/ver-1?collection=service_versions", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if w.Code != tt.wantStatus {
				t.Fatalf("GET %s: status %d, esperado %d; corpo: %s", tt.path, w.Code, tt.wantStatus, w.Body.String())
			}
			body := w.Body.String()
			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Errorf("GET %s: corpo sem %q: %s", tt.path, tt.wantBody, body)
			}
			// Nenhuma resposta pública expõe o conteúdo do rascunho
			if strings.Contains(body, "Servico Rascunho") || strings.Contains(body, "servico-rascunho") {
				t.Errorf("GET %s: resposta expõe o rascunho: %s", tt.path, body)
			}
		})
	}
}
