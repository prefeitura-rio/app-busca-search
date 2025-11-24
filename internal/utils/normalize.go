package utils

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// NormalizarCategoria remove acentos e caracteres especiais de uma categoria
// Exemplo: "Saúde" -> "saude", "Educação" -> "educacao"
func NormalizarCategoria(categoria string) string {
	if categoria == "" {
		return categoria
	}

	// Remove acentos e diacríticos
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, categoria)

	// Converte para minúsculas
	normalized = strings.ToLower(normalized)

	return normalized
}

// DesnormalizarCategoria tenta encontrar a categoria original com base na versão normalizada
// Recebe a categoria normalizada e uma lista de categorias válidas, retorna a categoria original
func DesnormalizarCategoria(categoriaNormalizada string, categoriasValidas []string) string {
	for _, categoria := range categoriasValidas {
		if NormalizarCategoria(categoria) == categoriaNormalizada {
			return categoria
		}
	}
	// Se não encontrar correspondência, retorna a categoria normalizada mesmo
	return categoriaNormalizada
}
