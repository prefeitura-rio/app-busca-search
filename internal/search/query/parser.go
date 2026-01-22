package query

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ParsedQuery representa uma query processada
type ParsedQuery struct {
	Original   string   // query original
	Normalized string   // query normalizada
	Tokens     []string // tokens extraídos
	HasSigla   bool     // contém sigla (IPTU, CPF, etc)
}

// Parser processa e normaliza queries de busca
type Parser struct{}

// NewParser cria um novo parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse processa a query e retorna estrutura normalizada
func (p *Parser) Parse(query string) *ParsedQuery {
	result := &ParsedQuery{
		Original: query,
	}

	// Normaliza
	normalized := p.normalize(query)
	result.Normalized = normalized

	// Tokeniza
	result.Tokens = p.tokenize(normalized)

	// Detecta siglas
	result.HasSigla = p.detectSigla(query)

	return result
}

// normalize limpa e normaliza a query
func (p *Parser) normalize(query string) string {
	// Remove espaços extras
	query = strings.TrimSpace(query)
	query = regexp.MustCompile(`\s+`).ReplaceAllString(query, " ")

	// Lowercase
	query = strings.ToLower(query)

	// Remove pontuação desnecessária mas mantém hífens e apóstrofos
	query = regexp.MustCompile(`[^\w\s\-'áàãâéêíóõôúüç]`).ReplaceAllString(query, " ")

	// Remove espaços extras novamente
	query = regexp.MustCompile(`\s+`).ReplaceAllString(query, " ")
	query = strings.TrimSpace(query)

	return query
}

// tokenize quebra a query em tokens
func (p *Parser) tokenize(query string) []string {
	// Split por espaços
	parts := strings.Fields(query)

	// Remove stopwords
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if !isStopword(part) && len(part) > 1 {
			tokens = append(tokens, part)
		}
	}

	return tokens
}

// detectSigla verifica se a query contém siglas conhecidas
func (p *Parser) detectSigla(query string) bool {
	siglas := []string{
		"iptu", "iss", "itbi", "cnh", "rg", "cpf", "cnpj",
		"inss", "fgts", "ir", "icms", "pis", "pasep",
		"bu", "brt", "vlt", "metrô", "metro",
	}

	queryLower := strings.ToLower(query)
	for _, sigla := range siglas {
		if strings.Contains(queryLower, sigla) {
			return true
		}
	}

	return false
}

// RemoveAccents remove acentos de uma string
func RemoveAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

// stopwords em português
var stopwords = map[string]bool{
	"a": true, "o": true, "e": true, "de": true, "da": true, "do": true,
	"em": true, "na": true, "no": true, "para": true, "por": true,
	"com": true, "um": true, "uma": true, "os": true, "as": true,
	"ao": true, "aos": true, "às": true, "que": true, "qual": true,
	"se": true, "ou": true, "mas": true, "como": true, "quero": true,
	"preciso": true, "gostaria": true, "fazer": true, "meu": true, "minha": true,
	"onde": true, "quando": true, "eu": true, "me": true, "mim": true,
	"é": true, "são": true, "foi": true, "será": true, "seria": true,
	"ter": true, "tenho": true, "tem": true, "posso": true, "pode": true,
}

func isStopword(word string) bool {
	return stopwords[strings.ToLower(word)]
}
