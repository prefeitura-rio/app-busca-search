package utils

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxSlugBaseLength = 50
	ShortIDLength     = 8
)

// GenerateSlug cria um slug SEO-friendly a partir do nome do serviço e ID.
// Formato: {kebab-case-name}-{short-id}
// Exemplo: "Matrícula Escolar" + "abc123def456" -> "matricula-escolar-abc123de"
func GenerateSlug(nomeServico, serviceID string) string {
	if nomeServico == "" || serviceID == "" {
		return ""
	}

	slug := normalizeToSlug(nomeServico)
	shortID := truncateID(serviceID)

	if slug == "" {
		return shortID
	}

	return slug + "-" + shortID
}

// normalizeToSlug converte texto para formato slug kebab-case
func normalizeToSlug(text string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, text)
	normalized = strings.ToLower(normalized)

	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug := reg.ReplaceAllString(normalized, "-")
	slug = strings.Trim(slug, "-")

	if len(slug) > MaxSlugBaseLength {
		slug = slug[:MaxSlugBaseLength]
		if lastHyphen := strings.LastIndex(slug, "-"); lastHyphen > 0 {
			slug = slug[:lastHyphen]
		}
	}

	return slug
}

// truncateID retorna os primeiros 8 caracteres do ID
func truncateID(id string) string {
	if len(id) > ShortIDLength {
		return id[:ShortIDLength]
	}
	return id
}

