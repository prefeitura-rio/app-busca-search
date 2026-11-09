package services

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/typesense/typesense-go/v3/typesense/api"
)

// bindPublicSearchRequest monta o SearchRequest exatamente como os handlers
// públicos (/api/v1/search e /api/v2/search) o fazem: ShouldBindQuery da query string.
func bindPublicSearchRequest(t *testing.T, rawQuery string) *models.SearchRequest {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/search?"+rawQuery, nil)

	var req models.SearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		t.Fatalf("bind da query %q falhou: %v", rawQuery, err)
	}
	return &req
}

// DEV-454: a busca pública v1 sempre filtra por serviços publicados,
// inclusive quando o cliente anônimo envia include_inactive=true.
func TestBuildFilterBy_AlwaysPublishedOnly(t *testing.T) {
	queries := []string{
		"q=a&type=keyword",
		"q=a&type=keyword&include_inactive=false",
		"q=a&type=keyword&include_inactive=true",
		"q=a&type=hybrid&include_inactive=true&exclude_agent_exclusive=true",
	}

	for _, rawQuery := range queries {
		t.Run(rawQuery, func(t *testing.T) {
			req := bindPublicSearchRequest(t, rawQuery)
			filterBy := buildFilterBy(req)

			if !containsFilter(filterBy, "status:=1") {
				t.Fatalf("filter_by %q não contém status:=1 para a query %q", filterBy, rawQuery)
			}
		})
	}
}

// DEV-454: o filtro de agentes continua combinado ao filtro de publicação.
func TestBuildFilterBy_KeepsAgentExclusiveFilter(t *testing.T) {
	req := bindPublicSearchRequest(t, "q=a&type=keyword&include_inactive=true&exclude_agent_exclusive=true")
	filterBy := buildFilterBy(req)

	if filterBy != "status:=1 && agents.exclusive_for_agents:=false" {
		t.Fatalf("filter_by inesperado: %q", filterBy)
	}
}

// DEV-454: a busca pública v2 sempre aplica o FilterField da collection nos três
// tipos de busca, inclusive quando o cliente envia include_inactive=true.
func TestSearchV2Params_AlwaysApplyCollectionFilter(t *testing.T) {
	ss := &SearchServiceV2{}
	collName := "prefrio_services_base"
	withFilter := &config.CollectionConfig{
		Type:        "service",
		TitleField:  "nome_servico",
		DescField:   "resumo",
		FilterField: "status",
		FilterValue: "1",
	}
	vectorQuery := "embedding:([0.1,0.2], k:10)"

	builders := map[string]func(req *models.SearchRequest) api.MultiSearchCollectionParameters{
		"keyword": func(req *models.SearchRequest) api.MultiSearchCollectionParameters {
			return ss.buildKeywordSearchParams(collName, withFilter, req)
		},
		"semantic": func(req *models.SearchRequest) api.MultiSearchCollectionParameters {
			return ss.buildSemanticSearchParams(collName, withFilter, req, vectorQuery)
		},
		"hybrid": func(req *models.SearchRequest) api.MultiSearchCollectionParameters {
			return ss.buildHybridSearchParams(collName, withFilter, req, vectorQuery)
		},
	}

	for _, includeInactive := range []string{"", "&include_inactive=false", "&include_inactive=true"} {
		for name, build := range builders {
			t.Run(name+includeInactive, func(t *testing.T) {
				req := bindPublicSearchRequest(t, "q=a&type="+name+includeInactive)
				params := build(req)

				if params.FilterBy == nil || *params.FilterBy != "status:=1" {
					got := "<nil>"
					if params.FilterBy != nil {
						got = *params.FilterBy
					}
					t.Fatalf("filter_by = %s, esperado status:=1", got)
				}
			})
		}
	}
}

// Collections sem FilterField (ex.: cursos e vagas) continuam sem filtro de status.
func TestSearchV2Params_NoFilterFieldMeansNoFilter(t *testing.T) {
	ss := &SearchServiceV2{}
	noFilter := &config.CollectionConfig{Type: "course", TitleField: "titulo", DescField: "descricao"}
	req := bindPublicSearchRequest(t, "q=a&type=keyword&include_inactive=true")

	params := ss.buildKeywordSearchParams("hub_search", noFilter, req)
	if params.FilterBy != nil {
		t.Fatalf("filter_by deveria ser nil, veio %q", *params.FilterBy)
	}
}

func containsFilter(filterBy, clause string) bool {
	for _, part := range strings.Split(filterBy, "&&") {
		if strings.TrimSpace(part) == clause {
			return true
		}
	}
	return false
}
