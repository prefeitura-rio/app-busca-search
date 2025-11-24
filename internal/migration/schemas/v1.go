package schemas

import (
	"github.com/typesense/typesense-go/v3/typesense/api"
)

// SchemaV1 retorna o schema baseline da collection prefrio_services_base
// Este é o schema atual em produção
func SchemaV1() *SchemaDefinition {
	return &SchemaDefinition{
		Version:      "v1",
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
		},
		Transform: nil, // V1 é o baseline, não precisa de transformação
	}
}

