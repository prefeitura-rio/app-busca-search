#!/bin/bash

# Script to create the service_versions collection in Typesense
# This collection stores the version history for services in prefrio_services_base

set -e

# Configuration (can be overridden by environment variables)
TYPESENSE_HOST="${TYPESENSE_HOST:-localhost}"
TYPESENSE_PORT="${TYPESENSE_PORT:-8108}"
TYPESENSE_API_KEY="${TYPESENSE_API_KEY}"
TYPESENSE_PROTOCOL="${TYPESENSE_PROTOCOL:-http}"

if [ -z "$TYPESENSE_API_KEY" ]; then
    echo "Error: TYPESENSE_API_KEY environment variable is required"
    exit 1
fi

TYPESENSE_URL="${TYPESENSE_PROTOCOL}://${TYPESENSE_HOST}:${TYPESENSE_PORT}"

echo "Creating service_versions collection in Typesense at ${TYPESENSE_URL}"

# Create the collection
curl -X POST "${TYPESENSE_URL}/collections" \
  -H "X-TYPESENSE-API-KEY: ${TYPESENSE_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "service_versions",
    "fields": [
      {"name": "service_id", "type": "string", "facet": true},
      {"name": "version_number", "type": "int64", "facet": false},
      {"name": "created_at", "type": "int64", "facet": false, "sort": true},
      {"name": "created_by", "type": "string", "facet": false},
      {"name": "created_by_cpf", "type": "string", "facet": true},
      {"name": "change_type", "type": "string", "facet": true},
      {"name": "change_reason", "type": "string", "optional": true, "facet": false},
      {"name": "previous_version", "type": "int64", "optional": true, "facet": false},
      {"name": "is_rollback", "type": "bool", "facet": true},
      {"name": "rollback_to_version", "type": "int64", "optional": true, "facet": false},
      {"name": "nome_servico", "type": "string", "facet": false},
      {"name": "orgao_gestor", "type": "string[]", "facet": true},
      {"name": "resumo", "type": "string", "facet": false},
      {"name": "tempo_atendimento", "type": "string", "optional": true, "facet": false},
      {"name": "custo_servico", "type": "string", "optional": true, "facet": false},
      {"name": "resultado_solicitacao", "type": "string", "optional": true, "facet": false},
      {"name": "descricao_completa", "type": "string", "optional": true, "facet": false},
      {"name": "autor", "type": "string", "facet": false},
      {"name": "documentos_necessarios", "type": "string[]", "optional": true, "facet": false},
      {"name": "instrucoes_solicitante", "type": "string", "optional": true, "facet": false},
      {"name": "canais_digitais", "type": "string[]", "optional": true, "facet": false},
      {"name": "canais_presenciais", "type": "string[]", "optional": true, "facet": false},
      {"name": "servico_nao_cobre", "type": "string", "optional": true, "facet": false},
      {"name": "legislacao_relacionada", "type": "string[]", "optional": true, "facet": false},
      {"name": "tema_geral", "type": "string", "facet": true},
      {"name": "publico_especifico", "type": "string[]", "optional": true, "facet": true},
      {"name": "fixar_destaque", "type": "bool", "facet": true},
      {"name": "awaiting_approval", "type": "bool", "facet": true},
      {"name": "published_at", "type": "int64", "optional": true, "facet": false},
      {"name": "is_free", "type": "bool", "optional": true, "facet": true},
      {"name": "status", "type": "int32", "facet": true},
      {"name": "search_content", "type": "string", "facet": false},
      {"name": "embedding_hash", "type": "string", "optional": true, "facet": false},
      {"name": "changed_fields_json", "type": "string", "optional": true, "facet": false}
    ],
    "default_sorting_field": "created_at"
  }'

echo ""
echo "Collection service_versions created successfully!"
