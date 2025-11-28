package schemas

import (
	"fmt"
	"sync"

	"github.com/typesense/typesense-go/v3/typesense/api"
)

// SchemaDefinition define o schema de uma collection Typesense
type SchemaDefinition struct {
	Version      string
	Name         string
	Fields       []api.Field
	SortingField string
	NestedFields bool
	Transform    func(doc map[string]interface{}) (map[string]interface{}, error)
}

// Registry mantém o registro de schemas versionados
type Registry struct {
	mu             sync.RWMutex
	schemas        map[string]*SchemaDefinition
	currentVersion string
}

// NewRegistry cria um novo registro de schemas
func NewRegistry() *Registry {
	r := &Registry{
		schemas: make(map[string]*SchemaDefinition),
	}

	r.registerBuiltinSchemas()

	return r
}

// registerBuiltinSchemas registra todos os schemas disponíveis (REGISTRAR AQUI OS NOVOS SCHEMAS)
func (r *Registry) registerBuiltinSchemas() {
	r.Register(SchemaV1())
	r.Register(SchemaV2())
}

// Register registra um novo schema
func (r *Registry) Register(schema *SchemaDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.schemas[schema.Version] = schema

	if r.currentVersion == "" || schema.Version > r.currentVersion {
		r.currentVersion = schema.Version
	}
}

// GetSchema retorna um schema por versão
func (r *Registry) GetSchema(version string) (*SchemaDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schema, exists := r.schemas[version]
	if !exists {
		return nil, fmt.Errorf("schema versão '%s' não encontrado", version)
	}

	return schema, nil
}

// GetCurrentVersion retorna a versão atual do schema
func (r *Registry) GetCurrentVersion() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentVersion
}

// SetCurrentVersion define a versão atual do schema
func (r *Registry) SetCurrentVersion(version string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.schemas[version]; !exists {
		return fmt.Errorf("schema versão '%s' não encontrado", version)
	}

	r.currentVersion = version
	return nil
}

// ListVersions retorna todas as versões disponíveis
func (r *Registry) ListVersions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := make([]string, 0, len(r.schemas))
	for version := range r.schemas {
		versions = append(versions, version)
	}

	return versions
}

// Helper functions para criação de schemas

// StringPtr retorna um ponteiro para string
func StringPtr(s string) *string {
	return &s
}

// IntPtr retorna um ponteiro para int
func IntPtr(i int) *int {
	return &i
}

// BoolPtr retorna um ponteiro para bool
func BoolPtr(b bool) *bool {
	return &b
}
