package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// GET /api/v1/search/:id é público: para id inexistente responde 404 com mensagem genérica,
// sem repassar o erro cru do Typesense (DEV-558).
func TestGetDocumentByIDNotFoundDoesNotLeakTypesenseError(t *testing.T) {
	router := detailPublicRouter(t)

	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "draft-1"} {
		t.Run(id, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/search/"+id, nil))

			if w.Code != http.StatusNotFound {
				t.Fatalf("status %d, esperado 404; corpo: %s", w.Code, w.Body.String())
			}

			var body map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("corpo não é JSON: %v; corpo: %s", err, w.Body.String())
			}
			if len(body) != 1 || body["error"] != "Serviço não encontrado" {
				t.Errorf("corpo = %s, esperado só {\"error\": \"Serviço não encontrado\"}", w.Body.String())
			}
			raw := w.Body.String()
			for _, leak := range []string{"details", "Could not find", "status: 404", "response:"} {
				if strings.Contains(raw, leak) {
					t.Errorf("resposta vaza %q do erro interno: %s", leak, raw)
				}
			}
		})
	}
}
