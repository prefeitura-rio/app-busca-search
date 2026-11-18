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
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	internalts "github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

// Mensagem que o Typesense falso devolve nos erros: nunca pode chegar ao cliente da API.
const adminListFakeTypesenseLeak = "Could not parse the filter query: detalhe-interno"

// adminListFakeTypesense responde como o Typesense à busca de prefrio_services_base e guarda
// cada filter_by recebido. Com failAll, toda busca responde 400, como um filter_by inválido.
type adminListFakeTypesense struct {
	mu       sync.Mutex
	filters  []string
	requests int
}

func (f *adminListFakeTypesense) recorded() ([]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.filters...), f.requests
}

func adminListRouter(t *testing.T, failAll bool) (*gin.Engine, *adminListFakeTypesense) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	fake := &adminListFakeTypesense{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/collections/prefrio_services_base/documents/search" {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		fake.mu.Lock()
		fake.requests++
		fake.filters = append(fake.filters, r.URL.Query().Get("filter_by"))
		fake.mu.Unlock()
		if failAll {
			http.Error(w, `{"message":"`+adminListFakeTypesenseLeak+`"}`, http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"found":0,"out_of":0,"page":1,"search_time_ms":0,"hits":[],"facet_counts":[],"request_params":{"collection_name":"prefrio_services_base","per_page":10,"q":"*"}}`)
	}))
	t.Cleanup(server.Close)

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
	handler := NewAdminHandler(internalts.NewClient(cfg))

	r := gin.New()
	r.GET("/api/v1/admin/services", handler.ListServices)
	return r, fake
}

func doAdminList(t *testing.T, r *gin.Engine, query url.Values) (int, map[string]interface{}, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/services?"+query.Encode(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("resposta não é JSON: %v (%s)", err, w.Body.String())
	}
	return w.Code, body, w.Body.String()
}

// filterStructure devolve o filter_by sem o conteúdo dos literais entre backticks: a estrutura
// que o Typesense avalia. Falha se os backticks não fecham.
func filterStructure(t *testing.T, filterBy string) string {
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

func TestAdminListServices_DynamicFilterRejectsUnknownOrHostileField(t *testing.T) {
	fields := []string{
		"status:=1||status",
		"status:=1 || status",
		"id:!=''",
		"(status",
		"embedding",
		"cpf",
		"Status",
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			r, fake := adminListRouter(t, false)
			code, body, raw := doAdminList(t, r, url.Values{"field": {field}, "value": {"0"}})
			if code != http.StatusBadRequest {
				t.Fatalf("field %q: status %d, esperado 400 (%s)", field, code, raw)
			}
			if msg, _ := body["error"].(string); msg == "" {
				t.Fatalf("field %q: resposta 400 sem mensagem de erro: %s", field, raw)
			}
			if _, requests := fake.recorded(); requests != 0 {
				t.Fatalf("field %q: o Typesense não devia ser chamado, recebeu %d buscas", field, requests)
			}
		})
	}
}

func TestAdminListServices_DynamicFilterRejectsInvalidTypedValue(t *testing.T) {
	cases := []url.Values{
		{"field": {"status"}, "value": {"1) || (id:!=''"}},
		{"field": {"status"}, "value": {"7"}},
		{"field": {"awaiting_approval"}, "value": {"talvez"}},
		{"field": {"is_free"}, "value": {"true || status:=0"}},
		{"field": {"published_at"}, "value": {"ontem"}},
	}
	for _, q := range cases {
		t.Run(q.Encode(), func(t *testing.T) {
			r, fake := adminListRouter(t, false)
			code, _, raw := doAdminList(t, r, q)
			if code != http.StatusBadRequest {
				t.Fatalf("%s: status %d, esperado 400 (%s)", q.Encode(), code, raw)
			}
			if _, requests := fake.recorded(); requests != 0 {
				t.Fatalf("%s: o Typesense não devia ser chamado, recebeu %d buscas", q.Encode(), requests)
			}
		})
	}
}

func TestAdminListServices_DynamicFilterAllowedFields(t *testing.T) {
	cases := []struct {
		field, value string
		want         string
	}{
		{"status", "1", "status:=1"},
		{"status", "0", "status:=0"},
		{"awaiting_approval", "true", "awaiting_approval:=true"},
		{"is_free", "false", "is_free:=false"},
		{"published_at", "1700000000", "published_at:=1700000000"},
		{"tema_geral", "Saúde", "tema_geral:=`saude`"},
		{"sub_categoria", "Vacinação", "sub_categoria:=`vacinacao`"},
		{"autor", "Autor Teste", "autor:=`autor teste`"},
		{"author", "Autor Teste", "autor:=`autor teste`"},
	}
	for _, tc := range cases {
		t.Run(tc.field+"="+tc.value, func(t *testing.T) {
			r, fake := adminListRouter(t, false)
			code, _, raw := doAdminList(t, r, url.Values{"field": {tc.field}, "value": {tc.value}})
			if code != http.StatusOK {
				t.Fatalf("status %d, esperado 200 (%s)", code, raw)
			}
			filters, _ := fake.recorded()
			if len(filters) != 1 || filters[0] != tc.want {
				t.Fatalf("filter_by = %q, esperado %q", filters, tc.want)
			}
		})
	}
}

func TestAdminListServices_StringValuesCannotInjectExpressions(t *testing.T) {
	hostile := "x` || status:=0 || tema_geral:=`y"
	queries := []url.Values{
		{"field": {"tema_geral"}, "value": {hostile}},
		{"tema_geral": {hostile}},
		{"author": {"a) || (id:!=''"}},
		{"sub_categoria": {"b && status:=0"}},
	}
	for _, q := range queries {
		t.Run(q.Encode(), func(t *testing.T) {
			r, fake := adminListRouter(t, false)
			code, _, raw := doAdminList(t, r, q)
			if code != http.StatusOK {
				t.Fatalf("status %d, esperado 200 (%s)", code, raw)
			}
			filters, _ := fake.recorded()
			if len(filters) != 1 {
				t.Fatalf("esperada uma busca, recebidas %d", len(filters))
			}
			structure := filterStructure(t, filters[0])
			for _, op := range []string{"||", "&&", "(", ")", "!="} {
				if strings.Contains(structure, op) {
					t.Fatalf("filter_by %q tem %q fora dos literais", filters[0], op)
				}
			}
			if strings.Count(structure, ":=") != 1 {
				t.Fatalf("filter_by %q devia ter um único filtro", filters[0])
			}
		})
	}
}

func TestAdminListServices_TypesenseErrorIsNotLeaked(t *testing.T) {
	r, _ := adminListRouter(t, true)
	code, body, raw := doAdminList(t, r, url.Values{"status": {"1"}})
	if code != http.StatusInternalServerError {
		t.Fatalf("status %d, esperado 500 (%s)", code, raw)
	}
	if msg, _ := body["error"].(string); msg != "Erro ao listar serviços" {
		t.Fatalf("mensagem de erro = %q, esperada genérica", msg)
	}
	if strings.Contains(raw, "detalhe-interno") || strings.Contains(raw, "filter query") || strings.Contains(raw, "status: 400") {
		t.Fatalf("resposta vaza detalhe interno do Typesense: %s", raw)
	}
}

func TestAdminListServices_NamedFiltersUnchanged(t *testing.T) {
	r, fake := adminListRouter(t, false)
	code, _, raw := doAdminList(t, r, url.Values{"status": {"1"}, "is_free": {"true"}})
	if code != http.StatusOK {
		t.Fatalf("status %d, esperado 200 (%s)", code, raw)
	}
	filters, _ := fake.recorded()
	if len(filters) != 1 {
		t.Fatalf("esperada uma busca, recebidas %d", len(filters))
	}
	parts := strings.Split(filters[0], " && ")
	got := map[string]bool{}
	for _, p := range parts {
		got[p] = true
	}
	if len(parts) != 2 || !got["status:=1"] || !got["is_free:=true"] {
		t.Fatalf("filter_by = %q, esperado status:=1 && is_free:=true", filters[0])
	}
}
