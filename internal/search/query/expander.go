package query

import (
	"context"
	"strings"

	"github.com/prefeitura-rio/app-busca-search/internal/search/synonyms"
)

// ExpandedQuery representa uma query expandida
type ExpandedQuery struct {
	Original      string   // query original
	Normalized    string   // query normalizada
	Tokens        []string // tokens originais
	ExpandedTerms []string // termos após expansão
	QueryString   string   // query final para busca
}

// Expander expande queries com sinônimos
type Expander struct {
	synonymService *synonyms.Service
	maxExpansions  int
}

// NewExpander cria um novo expander
func NewExpander(synonymService *synonyms.Service, maxExpansions int) *Expander {
	if maxExpansions <= 0 {
		maxExpansions = 5
	}
	return &Expander{
		synonymService: synonymService,
		maxExpansions:  maxExpansions,
	}
}

// Expand expande a query com sinônimos
func (e *Expander) Expand(ctx context.Context, parsed *ParsedQuery) *ExpandedQuery {
	result := &ExpandedQuery{
		Original:   parsed.Original,
		Normalized: parsed.Normalized,
		Tokens:     parsed.Tokens,
	}

	// Mapa para evitar duplicatas
	seen := make(map[string]bool)
	expanded := make([]string, 0)

	// Adiciona tokens originais primeiro
	for _, token := range parsed.Tokens {
		if !seen[token] {
			seen[token] = true
			expanded = append(expanded, token)
		}
	}

	// Expande cada token com sinônimos
	expansionsAdded := 0
	for _, token := range parsed.Tokens {
		if expansionsAdded >= e.maxExpansions {
			break
		}

		// Busca sinônimos locais
		syns := synonyms.FindSynonyms(token)
		for _, syn := range syns {
			if !seen[syn] && expansionsAdded < e.maxExpansions {
				seen[syn] = true
				expanded = append(expanded, syn)
				expansionsAdded++
			}
		}
	}

	result.ExpandedTerms = expanded
	result.QueryString = e.buildQueryString(expanded)

	return result
}

// ExpandSimple expande sem usar serviço de sinônimos (usa dados locais)
func (e *Expander) ExpandSimple(parsed *ParsedQuery) *ExpandedQuery {
	result := &ExpandedQuery{
		Original:   parsed.Original,
		Normalized: parsed.Normalized,
		Tokens:     parsed.Tokens,
	}

	// Mapa para evitar duplicatas
	seen := make(map[string]bool)
	expanded := make([]string, 0)

	// Adiciona tokens originais
	for _, token := range parsed.Tokens {
		if !seen[token] {
			seen[token] = true
			expanded = append(expanded, token)
		}
	}

	// Expande com sinônimos locais
	expansionsAdded := 0
	for _, token := range parsed.Tokens {
		if expansionsAdded >= e.maxExpansions {
			break
		}

		syns := synonyms.FindSynonyms(token)
		for _, syn := range syns {
			if !seen[syn] && expansionsAdded < e.maxExpansions {
				seen[syn] = true
				expanded = append(expanded, syn)
				expansionsAdded++
			}
		}
	}

	result.ExpandedTerms = expanded
	result.QueryString = e.buildQueryString(expanded)

	return result
}

// buildQueryString constrói a string de busca
func (e *Expander) buildQueryString(terms []string) string {
	if len(terms) == 0 {
		return "*"
	}
	return strings.Join(terms, " ")
}
