# Migração de Schema - Guia Rápido

## 1. Criar o Novo Schema

Crie um arquivo em `internal/migration/schemas/`:

```go
// internal/migration/schemas/v2.go
package schemas

import "github.com/typesense/typesense-go/v3/typesense/api"

func SchemaV2() *SchemaDefinition {
    return &SchemaDefinition{
        Version:      "v2",
        Name:         "prefrio_services_base",
        SortingField: "last_update",
        NestedFields: true,
        Fields: []api.Field{
            // Copie todos os campos do schema anterior (v1.go)
            // e adicione/modifique os novos campos
            {Name: "novo_campo", Type: "string", Optional: BoolPtr(true)},
        },
        Transform: nil, // ou função de transformação (ver abaixo)
    }
}
```

## 2. Registrar o Schema

Edite `internal/migration/schemas/schema.go`:

```go
func (r *Registry) registerBuiltinSchemas() {
    r.Register(SchemaV1())
    r.Register(SchemaV2()) // ← adicione aqui
}
```

## 3. Transformações (Opcional)

Se precisar transformar dados durante a migração:

```go
Transform: func(doc map[string]interface{}) (map[string]interface{}, error) {
    // Valor padrão para novo campo
    if doc["novo_campo"] == nil {
        doc["novo_campo"] = "valor_padrao"
    }
    
    // Renomear campo
    if old, ok := doc["campo_antigo"]; ok {
        doc["campo_novo"] = old
        delete(doc, "campo_antigo")
    }
    
    // Converter tipo
    if val, ok := doc["preco"].(string); ok {
        doc["preco"] = parseFloat(val)
    }
    
    return doc, nil
},
```

## 4. Executar a Migração

### Via CLI (acesso ao servidor)
```bash
# Verificar schemas disponíveis
go run ./cmd/migrate schemas

# Testar sem modificar (dry-run)
go run ./cmd/migrate start --schema=v2 --dry-run

# Executar migração
go run ./cmd/migrate start --schema=v2 --user="Seu Nome"

# Acompanhar
go run ./cmd/migrate status
```

### Via API (remoto)
```bash
curl -X POST -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{"schema_version":"v2"}' \
  http://localhost:8080/api/v1/admin/migration/start
```

## 5. Rollback (se necessário)

```bash
go run ./cmd/migrate rollback
```

## Fluxo Visual

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  1. Criar v2.go │ ──► │  2. Registrar   │ ──► │  3. Rebuild     │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  6. Validar     │ ◄── │  5. Migrar      │ ◄── │  4. Disparar    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## Importante

- **CUD bloqueado** durante a migração (retorna HTTP 503)
- **GET continua funcionando** normalmente
- **Backup automático** criado antes de migrar
- **Alias** atualizado automaticamente - código existente não precisa mudar

