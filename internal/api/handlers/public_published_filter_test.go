package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search"
	"github.com/prefeitura-rio/app-busca-search/internal/search/adapter"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	"github.com/typesense/typesense-go/v3/typesense"
)

// fakeTypesense é um servidor HTTP que responde como o Typesense com resultados vazios
// e guarda cada filter_by recebido, para provar o filtro que as rotas públicas enviam.
type fakeTypesense struct {
	mu      sync.Mutex
	filters []string
	server  *httptest.Server
}

func newFakeTypesense(t *testing.T) *fakeTypesense {
	t.Helper()
	f := &fakeTypesense{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/documents/search"):
			f.record(r.URL.Query().Get("filter_by"))
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
				filterBy, _ := s["filter_by"].(string)
				f.record(filterBy)
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

func (f *fakeTypesense) record(filterBy string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.filters = append(f.filters, filterBy)
}

func (f *fakeTypesense) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.filters...)
}

func (f *fakeTypesense) client() *typesense.Client {
	return typesense.NewClient(typesense.WithServer(f.server.URL), typesense.WithAPIKey("test-key"))
}

// publicRouter monta as rotas públicas de categorias, subcategorias e busca v3 como em
// routes.go, sem nenhum middleware de autenticação, apontando para o Typesense falso.
func publicRouter(f *fakeTypesense) *gin.Engine {
	gin.SetMode(gin.TestMode)
	client := f.client()
	popularity := services.NewPopularityService()

	categoryHandler := NewCategoryHandler(services.NewCategoryService(client, popularity))
	subcategoryHandler := NewSubcategoryHandler(services.NewSubcategoryService(client, popularity))

	engine := search.NewEngine(
		adapter.NewTypesenseAdapter(client, f.server.URL, "test-key"),
		nil, // sem Gemini: hybrid/semantic caem para keyword
		nil,
		nil,
		map[string]*v3.CollectionConfig{"prefrio_services_base": v3.DefaultPrefRioConfig()},
		"prefrio_services_base",
	)
	searchHandlerV3 := NewSearchHandlerV3(engine)

	r := gin.New()
	api := r.Group("/api/v1")
	api.GET("/categories", categoryHandler.GetCategories)
	api.GET("/categories/:category/subcategories", subcategoryHandler.GetSubcategories)
	api.GET("/subcategories/:subcategory/services", subcategoryHandler.GetServicesBySubcategory)
	r.GET("/api/v3/search", searchHandlerV3.Search)
	return r
}

// outsideLiterals devolve o filter_by sem o conteúdo dos literais entre backticks,
// ou seja, a estrutura da expressão que o Typesense vai avaliar.
func outsideLiterals(t *testing.T, filterBy string) string {
	t.Helper()
	if strings.Count(filterBy, "`")%2 != 0 {
		t.Fatalf("filter_by com backticks desbalanceados: %q", filterBy)
	}
	var b strings.Builder
	inside := false
	for _, r := range filterBy {
		if r == '`' {
			inside = !inside
			b.WriteRune(r)
			continue
		}
		if !inside {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DEV-455: sem token, as rotas públicas de categorias e subcategorias mandam sempre
// status:=1 ao Typesense, inclusive com include_inactive=true, e um nome hostil com
// backticks não consegue injetar outra expressão no filtro.
func TestPublicCategoryRoutes_AlwaysFilterPublished(t *testing.T) {
	hostile := "x` || status:=0 || tema_geral:=`y"

	tests := []struct {
		name string
		path string
		// estrutura esperada de cada filter_by enviado (fora dos literais entre backticks)
		want []string
	}{
		{
			name: "categorias",
			path: "/api/v1/categories?include_inactive=true",
			want: []string{"status:=1"},
		},
		{
			name: "categorias com filter_category",
			path: "/api/v1/categories?include_inactive=true&filter_category=" + url.QueryEscape("Educação"),
			want: []string{"status:=1", "tema_geral:=`` && status:=1"},
		},
		{
			name: "categorias com filter_category hostil",
			path: "/api/v1/categories?include_inactive=true&filter_category=" + url.QueryEscape(hostile),
			want: []string{"status:=1", "tema_geral:=`` && status:=1"},
		},
		{
			name: "subcategorias",
			path: "/api/v1/categories/" + url.PathEscape("Educação") + "/subcategories?include_inactive=true",
			want: []string{"tema_geral:=`` && status:=1"},
		},
		{
			name: "subcategorias com categoria hostil",
			path: "/api/v1/categories/" + url.PathEscape(hostile) + "/subcategories?include_inactive=true",
			want: []string{"tema_geral:=`` && status:=1"},
		},
		{
			name: "serviços da subcategoria",
			path: "/api/v1/subcategories/" + url.PathEscape("Ensino Fundamental") + "/services?include_inactive=true",
			want: []string{"sub_categoria:=`` && status:=1"},
		},
		{
			name: "serviços da subcategoria com categoria",
			path: "/api/v1/subcategories/" + url.PathEscape("Certidões") + "/services?include_inactive=true&category=Documentos",
			want: []string{"tema_geral:=`` && sub_categoria:=`` && status:=1"},
		},
		{
			name: "serviços da subcategoria hostil",
			path: "/api/v1/subcategories/" + url.PathEscape(hostile) + "/services?include_inactive=true&category=" + url.QueryEscape(hostile),
			want: []string{"tema_geral:=`` && sub_categoria:=`` && status:=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeTypesense(t)
			r := publicRouter(f)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d, corpo %s", tt.path, w.Code, w.Body.String())
			}

			got := f.recorded()
			if len(got) != len(tt.want) {
				t.Fatalf("GET %s: %d buscas no Typesense (%q), esperado %d", tt.path, len(got), got, len(tt.want))
			}
			for i, filterBy := range got {
				if structure := outsideLiterals(t, filterBy); structure != tt.want[i] {
					t.Errorf("GET %s: filter_by %q (estrutura %q), esperado %q", tt.path, filterBy, structure, tt.want[i])
				}
			}
		})
	}
}

// DEV-455: sem token, GET /api/v3/search manda sempre status:=1 ao Typesense, inclusive
// com status=0, e filtros de texto hostis não conseguem injetar outra expressão.
func TestPublicSearchV3_AlwaysFilterPublished(t *testing.T) {
	hostile := url.QueryEscape("documentos || status:=0")
	hostileTick := url.QueryEscape("a` || status:=0 || tema_geral:=`b")

	queries := []string{
		"q=a&type=keyword",
		"q=a&type=keyword&status=0",
		"q=a&type=hybrid&status=0&mode=agent",
		"q=a&type=keyword&status=0&category=" + hostile,
		"q=a&type=keyword&status=0&category=" + hostileTick + "&sub_category=" + hostileTick + "&orgao=" + hostile + "&tempo_max=" + hostile,
	}

	for _, rawQuery := range queries {
		t.Run(rawQuery, func(t *testing.T) {
			f := newFakeTypesense(t)
			r := publicRouter(f)

			path := "/api/v3/search?expand=false&" + rawQuery
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d, corpo %s", path, w.Code, w.Body.String())
			}

			got := f.recorded()
			if len(got) == 0 {
				t.Fatalf("GET %s: nenhuma busca chegou ao Typesense", path)
			}
			for _, filterBy := range got {
				structure := outsideLiterals(t, filterBy)
				if strings.Contains(structure, "||") {
					t.Errorf("GET %s: filter_by %q tem expressão injetada (estrutura %q)", path, filterBy, structure)
				}
				if strings.Contains(structure, "status:=0") {
					t.Errorf("GET %s: filter_by %q pede rascunhos", path, filterBy)
				}
				clauses := strings.Split(structure, " && ")
				hasPublished := false
				for _, c := range clauses {
					if c == "status:=1" {
						hasPublished = true
					}
				}
				if !hasPublished {
					t.Errorf("GET %s: filter_by %q sem a cláusula status:=1", path, filterBy)
				}
			}
		})
	}
}
