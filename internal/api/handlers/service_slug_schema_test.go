package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	internalts "github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

// Mensagem que o Typesense falso devolve nos erros: nunca pode chegar ao cliente da API.
const slugFakeTypesenseLeak = "detalhe-interno-do-typesense"

// Collection prefrio_services_base sintética com schema anterior ao v3: os documentos guardam
// slug e slug_history, mas esses campos não são indexados e não podem entrar no filter_by.
var slugLegacyFixtures = map[string]map[string]interface{}{
	"pub-1": {
		"id": "pub-1", "nome_servico": "Servico Publicado", "resumo": "publicado", "status": 1,
		"slug": "servico-publicado", "slug_history": []string{"servico-publicado-antigo", "slug-compartilhado"},
		"embedding": []float64{0.1, 0.2},
	},
	"draft-1": {
		"id": "draft-1", "nome_servico": "Servico Rascunho", "resumo": "rascunho", "status": 0,
		"slug": "servico-rascunho", "slug_history": []string{"servico-rascunho-antigo", "slug-compartilhado"},
	},
	// Documento anterior ao v3 sem slug gravado: o slug é o que a migração v3 geraria.
	"0a1b2c3d-sem-slug": {
		"id": "0a1b2c3d-sem-slug", "nome_servico": "Certidão de Óbito", "resumo": "sem slug", "status": 1,
	},
}

// newSlugLegacyFakeTypesense responde como o Typesense de uma collection sem os campos do schema v3:
// filter_by com slug ou slug_history é recusado com 400, e o resto (status, id, leitura por id,
// paginação e include_fields) funciona. Com failAll, toda requisição responde 503.
func newSlugLegacyFakeTypesense(t *testing.T, failAll bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if failAll {
			http.Error(w, `{"message":"`+slugFakeTypesenseLeak+`"}`, http.StatusServiceUnavailable)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method != http.MethodGet || len(parts) != 4 || parts[0] != "collections" ||
			parts[1] != "prefrio_services_base" || parts[2] != "documents" {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		if parts[3] != "search" {
			doc, ok := slugLegacyFixtures[parts[3]]
			if !ok {
				http.Error(w, `{"message":"Could not find a document with id: `+parts[3]+`"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(doc)
			return
		}

		query := r.URL.Query()
		filterBy := query.Get("filter_by")
		for _, field := range []string{"slug_history", "slug"} {
			if strings.HasPrefix(filterBy, field+":") {
				http.Error(w, "{\"message\": \"Could not find a filter field named `"+field+"` in the schema.\"}", http.StatusBadRequest)
				return
			}
		}
		clauses := map[string]string{}
		for _, clause := range strings.Split(filterBy, " && ") {
			field, value, ok := strings.Cut(clause, ":=")
			if !ok {
				http.Error(w, `{"message":"filter_by inesperado"}`, http.StatusBadRequest)
				return
			}
			clauses[field] = strings.Trim(value, "`")
		}
		page, _ := strconv.Atoi(query.Get("page"))
		perPage, _ := strconv.Atoi(query.Get("per_page"))
		if page < 1 {
			page = 1
		}
		if perPage < 1 {
			perPage = 10
		}
		var include map[string]bool
		if fields := query.Get("include_fields"); fields != "" && fields != "*" {
			include = map[string]bool{}
			for _, f := range strings.Split(fields, ",") {
				include[strings.TrimSpace(f)] = true
			}
		}

		ids := []string{"0a1b2c3d-sem-slug", "draft-1", "pub-1"}
		matched := make([]map[string]interface{}, 0)
		for _, id := range ids {
			if detailDocMatches(slugLegacyFixtures[id], clauses) {
				matched = append(matched, slugLegacyFixtures[id])
			}
		}
		hits := make([]map[string]interface{}, 0)
		for i := (page - 1) * perPage; i < len(matched) && i < page*perPage; i++ {
			doc := map[string]interface{}{}
			for k, v := range matched[i] {
				if (include == nil || include[k]) && k != "embedding" {
					doc[k] = v
				}
			}
			hits = append(hits, map[string]interface{}{"document": doc})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"found": len(matched), "out_of": len(slugLegacyFixtures), "page": page, "search_time_ms": 0, "hits": hits,
			"request_params": map[string]interface{}{"collection_name": "prefrio_services_base", "per_page": perPage, "q": "*"},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func slugLegacyRouter(t *testing.T, failAll bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := newSlugLegacyFakeTypesense(t, failAll)
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
	handler := NewSearchHandler(nil, internalts.NewClient(cfg))

	r := gin.New()
	r.GET("/api/v1/services/:slug", handler.GetServiceBySlug)
	return r
}

func TestServiceBySlugWithoutSlugFieldInSchema(t *testing.T) {
	tests := []struct {
		name      string
		failAll   bool
		path      string
		wantCode  int
		wantBody  string // trecho que a resposta deve conter
		wantError string // campo "error" esperado quando não é 200/301
	}{
		{name: "slug publicado responde 200", path: "/api/v1/services/servico-publicado", wantCode: http.StatusOK, wantBody: "Servico Publicado"},
		{name: "slug gerado de documento sem slug gravado responde 200", path: "/api/v1/services/certidao-de-obito-0a1b2c3d", wantCode: http.StatusOK, wantBody: "0a1b2c3d-sem-slug"},
		{name: "slug histórico publicado responde 301", path: "/api/v1/services/servico-publicado-antigo", wantCode: http.StatusMovedPermanently, wantBody: "servico-publicado"},
		{name: "slug histórico compartilhado com rascunho responde 301 para o publicado", path: "/api/v1/services/slug-compartilhado", wantCode: http.StatusMovedPermanently, wantBody: "servico-publicado"},
		{name: "slug de rascunho responde 404", path: "/api/v1/services/servico-rascunho", wantCode: http.StatusNotFound, wantError: "Serviço não encontrado"},
		{name: "slug histórico de rascunho responde 404", path: "/api/v1/services/servico-rascunho-antigo", wantCode: http.StatusNotFound, wantError: "Serviço não encontrado"},
		{name: "slug inexistente responde 404", path: "/api/v1/services/abc-123-nope", wantCode: http.StatusNotFound, wantError: "Serviço não encontrado"},
		{name: "Typesense fora responde 500 sem detalhe interno", failAll: true, path: "/api/v1/services/servico-publicado", wantCode: http.StatusInternalServerError, wantError: "Erro ao buscar serviço"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := slugLegacyRouter(t, tt.failAll)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))

			body := w.Body.String()
			if w.Code != tt.wantCode {
				t.Fatalf("GET %s: status %d, esperado %d; corpo: %s", tt.path, w.Code, tt.wantCode, body)
			}
			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Errorf("GET %s: corpo sem %q: %s", tt.path, tt.wantBody, body)
			}
			if tt.wantError != "" {
				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("corpo não é JSON: %v; corpo: %s", err, body)
				}
				if resp["error"] != tt.wantError {
					t.Errorf("error = %v, esperado %q", resp["error"], tt.wantError)
				}
				if _, ok := resp["details"]; ok {
					t.Errorf("resposta traz details: %s", body)
				}
			}
			for _, leak := range []string{slugFakeTypesenseLeak, "status:", "response:", "Could not find", "Servico Rascunho", "embedding"} {
				if strings.Contains(body, leak) {
					t.Errorf("GET %s: corpo vaza %q: %s", tt.path, leak, body)
				}
			}
		})
	}
}
