package content

import (
	"strings"

	"github.com/prefeitura-rio/app-busca-search/internal/models"
)

// GeneratorConfig define como gerar o search_content
type GeneratorConfig struct {
	IncludeCategory     bool // incluir categoria no content
	IncludePublico      bool // incluir público alvo
	IncludeDocumentos   bool // incluir docs necessários
	IncludeInstrucoes   bool // incluir instruções
	IncludeOrgao        bool // incluir órgão gestor
	MaxLength           int  // tamanho máximo (default: 8000)
	UseStructuredFormat bool // usar formato com labels
}

// DefaultConfig retorna configuração padrão
func DefaultConfig() *GeneratorConfig {
	return &GeneratorConfig{
		IncludeCategory:     true,
		IncludePublico:      true,
		IncludeDocumentos:   true,
		IncludeInstrucoes:   true,
		IncludeOrgao:        true,
		MaxLength:           8000,
		UseStructuredFormat: true,
	}
}

// Generator gera search_content otimizado para embeddings
type Generator struct {
	config *GeneratorConfig
}

// NewGenerator cria um novo gerador
func NewGenerator(config *GeneratorConfig) *Generator {
	if config == nil {
		config = DefaultConfig()
	}
	return &Generator{config: config}
}

// Generate gera o search_content a partir de um PrefRioService
func (g *Generator) Generate(service *models.PrefRioService) string {
	if g.config.UseStructuredFormat {
		return g.generateStructured(service)
	}
	return g.generateSimple(service)
}

// generateStructured gera conteúdo com formato estruturado
func (g *Generator) generateStructured(service *models.PrefRioService) string {
	var parts []string

	// Título do serviço (sempre incluído)
	if service.NomeServico != "" {
		parts = append(parts, "SERVIÇO: "+service.NomeServico)
	}

	// Categoria
	if g.config.IncludeCategory && service.TemaGeral != "" {
		parts = append(parts, "CATEGORIA: "+service.TemaGeral)
	}

	// Resumo
	if service.Resumo != "" {
		parts = append(parts, "RESUMO: "+service.Resumo)
	}

	// Descrição completa
	if service.DescricaoCompleta != "" {
		desc := service.DescricaoCompleta
		if len(desc) > 2000 {
			desc = desc[:2000] + "..."
		}
		parts = append(parts, "DESCRIÇÃO: "+desc)
	}

	// Órgão gestor
	if g.config.IncludeOrgao && len(service.OrgaoGestor) > 0 {
		parts = append(parts, "ÓRGÃO GESTOR: "+strings.Join(service.OrgaoGestor, ", "))
	}

	// Público específico
	if g.config.IncludePublico && len(service.PublicoEspecifico) > 0 {
		parts = append(parts, "PÚBLICO ALVO: "+strings.Join(service.PublicoEspecifico, ", "))
	}

	// Documentos necessários
	if g.config.IncludeDocumentos && len(service.DocumentosNecessarios) > 0 {
		docs := strings.Join(service.DocumentosNecessarios, "; ")
		if len(docs) > 1000 {
			docs = docs[:1000] + "..."
		}
		parts = append(parts, "DOCUMENTOS NECESSÁRIOS: "+docs)
	}

	// Instruções
	if g.config.IncludeInstrucoes && service.InstrucoesSolicitante != "" {
		instr := service.InstrucoesSolicitante
		if len(instr) > 1000 {
			instr = instr[:1000] + "..."
		}
		parts = append(parts, "COMO SOLICITAR: "+instr)
	}

	content := strings.Join(parts, "\n\n")

	// Limita tamanho total
	if g.config.MaxLength > 0 && len(content) > g.config.MaxLength {
		content = content[:g.config.MaxLength]
	}

	return content
}

// generateSimple gera conteúdo simples (concatenação)
func (g *Generator) generateSimple(service *models.PrefRioService) string {
	var content []string

	if service.NomeServico != "" {
		content = append(content, service.NomeServico)
	}
	if service.Resumo != "" {
		content = append(content, service.Resumo)
	}
	if service.DescricaoCompleta != "" {
		content = append(content, service.DescricaoCompleta)
	}
	if g.config.IncludeCategory && service.TemaGeral != "" {
		content = append(content, service.TemaGeral)
	}

	content = append(content, service.OrgaoGestor...)
	content = append(content, service.PublicoEspecifico...)
	content = append(content, service.DocumentosNecessarios...)

	result := strings.Join(content, " ")

	if g.config.MaxLength > 0 && len(result) > g.config.MaxLength {
		result = result[:g.config.MaxLength]
	}

	return result
}

// GenerateFromMap gera search_content a partir de um mapa (documento Typesense)
func (g *Generator) GenerateFromMap(doc map[string]interface{}) string {
	service := &models.PrefRioService{}

	if v, ok := doc["nome_servico"].(string); ok {
		service.NomeServico = v
	}
	if v, ok := doc["resumo"].(string); ok {
		service.Resumo = v
	}
	if v, ok := doc["descricao_completa"].(string); ok {
		service.DescricaoCompleta = v
	}
	if v, ok := doc["tema_geral"].(string); ok {
		service.TemaGeral = v
	}
	if v, ok := doc["orgao_gestor"].([]interface{}); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				service.OrgaoGestor = append(service.OrgaoGestor, s)
			}
		}
	}
	if v, ok := doc["publico_especifico"].([]interface{}); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				service.PublicoEspecifico = append(service.PublicoEspecifico, s)
			}
		}
	}
	if v, ok := doc["documentos_necessarios"].([]interface{}); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				service.DocumentosNecessarios = append(service.DocumentosNecessarios, s)
			}
		}
	}
	if v, ok := doc["instrucoes_solicitante"].(string); ok {
		service.InstrucoesSolicitante = v
	}

	return g.Generate(service)
}
