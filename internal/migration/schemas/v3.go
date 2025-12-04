package schemas

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/typesense/typesense-go/v3/typesense/api"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func SchemaV3() *SchemaDefinition {
	return &SchemaDefinition{
		Version:      "v3",
		Name:         "prefrio_services_base",
		SortingField: "last_update",
		NestedFields: true,
		Fields: []api.Field{
			{Name: "id", Type: "string", Optional: BoolPtr(true)},
			{Name: "nome_servico", Type: "string", Facet: BoolPtr(false)},
			{Name: "orgao_gestor", Type: "string[]", Facet: BoolPtr(true)},
			{Name: "resumo", Type: "string", Facet: BoolPtr(false)},
			{Name: "tempo_atendimento", Type: "string", Facet: BoolPtr(false)},
			{Name: "custo_servico", Type: "string", Facet: BoolPtr(true)},
			{Name: "resultado_solicitacao", Type: "string", Facet: BoolPtr(true)},
			{Name: "descricao_completa", Type: "string", Facet: BoolPtr(false)},
			{Name: "autor", Type: "string", Facet: BoolPtr(true)},
			{Name: "documentos_necessarios", Type: "string[]", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "instrucoes_solicitante", Type: "string", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "canais_digitais", Type: "string[]", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "canais_presenciais", Type: "string[]", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "servico_nao_cobre", Type: "string", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "legislacao_relacionada", Type: "string[]", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "tema_geral", Type: "string", Facet: BoolPtr(true)},
			{Name: "sub_categoria", Type: "string", Facet: BoolPtr(true), Optional: BoolPtr(true)},
			{Name: "publico_especifico", Type: "string[]", Facet: BoolPtr(true), Optional: BoolPtr(true)},
			{Name: "fixar_destaque", Type: "bool", Facet: BoolPtr(true)},
			{Name: "awaiting_approval", Type: "bool", Facet: BoolPtr(true)},
			{Name: "published_at", Type: "int64", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "is_free", Type: "bool", Facet: BoolPtr(true), Optional: BoolPtr(true)},
			{Name: "agents", Type: "object", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "extra_fields", Type: "object", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "status", Type: "int32", Facet: BoolPtr(true)},
			{Name: "created_at", Type: "int64", Facet: BoolPtr(false)},
			{Name: "last_update", Type: "int64", Facet: BoolPtr(false)},
			{Name: "search_content", Type: "string", Facet: BoolPtr(false)},
			{Name: "buttons", Type: "object[]", Facet: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "embedding", Type: "float[]", Facet: BoolPtr(false), Optional: BoolPtr(true), NumDim: IntPtr(768)},
			// Novos campos para SEO-friendly URLs
			{Name: "slug", Type: "string", Facet: BoolPtr(true)},
			{Name: "slug_history", Type: "string[]", Facet: BoolPtr(false), Optional: BoolPtr(true)},
		},
		Transform: transformV3,
	}
}

// transformV3 gera slugs para documentos existentes durante a migração
func transformV3(doc map[string]interface{}) (map[string]interface{}, error) {
	id, _ := doc["id"].(string)
	nomeServico, _ := doc["nome_servico"].(string)

	if id != "" && nomeServico != "" {
		doc["slug"] = generateSlugForMigration(nomeServico, id)
	}

	if _, exists := doc["slug_history"]; !exists {
		doc["slug_history"] = []string{}
	}

	return doc, nil
}

// generateSlugForMigration gera slug durante migração (lógica duplicada para evitar dependência circular)
func generateSlugForMigration(nomeServico, serviceID string) string {
	if nomeServico == "" || serviceID == "" {
		return ""
	}

	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, nomeServico)
	normalized = strings.ToLower(normalized)

	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug := reg.ReplaceAllString(normalized, "-")
	slug = strings.Trim(slug, "-")

	const maxSlugLength = 50
	if len(slug) > maxSlugLength {
		slug = slug[:maxSlugLength]
		if lastHyphen := strings.LastIndex(slug, "-"); lastHyphen > 0 {
			slug = slug[:lastHyphen]
		}
	}

	shortID := serviceID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	if slug == "" {
		return shortID
	}

	return slug + "-" + shortID
}

