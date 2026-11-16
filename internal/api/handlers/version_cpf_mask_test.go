package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// CPFs sintéticos de quem editou as versões do Typesense falso (versionFixtures): o completo
// nunca pode sair nas respostas das rotas de versão.
const (
	versionFixtureCPF1 = "12345678909"
	versionFixtureCPF2 = "987.654.321-00"
)

// assertNoFixtureCPF falha se o corpo traz algum CPF completo das versões sintéticas, com ou sem
// pontuação.
func assertNoFixtureCPF(t *testing.T, body string) {
	t.Helper()
	for _, cpf := range []string{versionFixtureCPF1, "123.456.789-09", versionFixtureCPF2, "98765432100"} {
		if strings.Contains(body, cpf) {
			t.Errorf("resposta expõe o CPF completo %q: %s", cpf, body)
		}
	}
}

func TestVersionRoutesMaskEditorCPF(t *testing.T) {
	router := versionRouter(t, false)

	t.Run("listar versões devolve created_by_cpf mascarado e mantém o nome", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/svc-1/versions", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, esperado 200; corpo: %s", w.Code, w.Body.String())
		}
		assertNoFixtureCPF(t, w.Body.String())

		var history struct {
			Versions []struct {
				VersionNumber int64  `json:"version_number"`
				CreatedBy     string `json:"created_by"`
				CreatedByCPF  string `json:"created_by_cpf"`
			} `json:"versions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &history); err != nil {
			t.Fatalf("corpo não é JSON: %v", err)
		}
		want := map[int64]string{1: "***.456.789-**", 2: "***.654.321-**"}
		if len(history.Versions) != len(want) {
			t.Fatalf("versões = %d, esperado %d; corpo: %s", len(history.Versions), len(want), w.Body.String())
		}
		for _, v := range history.Versions {
			if v.CreatedByCPF != want[v.VersionNumber] {
				t.Errorf("versão %d: created_by_cpf = %q, esperado %q", v.VersionNumber, v.CreatedByCPF, want[v.VersionNumber])
			}
			if v.CreatedBy != "Autor Teste" {
				t.Errorf("versão %d: created_by = %q, esperado o nome do autor", v.VersionNumber, v.CreatedBy)
			}
		}
	})

	t.Run("versão específica devolve created_by_cpf mascarado", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/svc-1/versions/2", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, esperado 200; corpo: %s", w.Code, w.Body.String())
		}
		assertNoFixtureCPF(t, w.Body.String())

		var version struct {
			VersionNumber int64  `json:"version_number"`
			CreatedBy     string `json:"created_by"`
			CreatedByCPF  string `json:"created_by_cpf"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &version); err != nil {
			t.Fatalf("corpo não é JSON: %v", err)
		}
		if version.VersionNumber != 2 || version.CreatedByCPF != "***.654.321-**" || version.CreatedBy != "Autor Teste" {
			t.Errorf("versão inesperada: %s", w.Body.String())
		}
	})

	t.Run("comparar versões não traz CPF, só o nome de quem mudou", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/svc-1/versions/compare?from_version=1&to_version=2", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, esperado 200; corpo: %s", w.Code, w.Body.String())
		}
		assertNoFixtureCPF(t, w.Body.String())
		if strings.Contains(w.Body.String(), "cpf") {
			t.Errorf("diff traz campo de CPF: %s", w.Body.String())
		}
		var diff struct {
			ChangedBy string `json:"changed_by"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &diff); err != nil {
			t.Fatalf("corpo não é JSON: %v", err)
		}
		if diff.ChangedBy != "Autor Teste" {
			t.Errorf("changed_by = %q, esperado o nome do autor", diff.ChangedBy)
		}
	})
}
