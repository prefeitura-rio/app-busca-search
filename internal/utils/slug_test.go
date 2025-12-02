package utils

import (
	"strings"
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name        string
		nomeServico string
		serviceID   string
		expected    string
	}{
		{
			name:        "nome simples",
			nomeServico: "Matrícula Escolar",
			serviceID:   "abc123def456",
			expected:    "matricula-escolar-abc123de",
		},
		{
			name:        "nome com números e ordinal",
			nomeServico: "2ª Via de Certidão",
			serviceID:   "xyz789abc123",
			expected:    "2-via-de-certidao-xyz789ab",
		},
		{
			name:        "nome com segunda escrito por extenso",
			nomeServico: "Segunda Via de Certidão",
			serviceID:   "xyz789abc123",
			expected:    "segunda-via-de-certidao-xyz789ab",
		},
		{
			name:        "nome com parênteses",
			nomeServico: "Solicitação de Alvará (Comércio)",
			serviceID:   "def456ghi789",
			expected:    "solicitacao-de-alvara-comercio-def456gh",
		},
		{
			name:        "nome com acentos diversos",
			nomeServico: "Serviço de Atenção à Saúde",
			serviceID:   "aaa111bbb222",
			expected:    "servico-de-atencao-a-saude-aaa111bb",
		},
		{
			name:        "nome com cedilha",
			nomeServico: "Licença de Construção",
			serviceID:   "ccc333ddd444",
			expected:    "licenca-de-construcao-ccc333dd",
		},
		{
			name:        "ID curto (menos de 8 chars)",
			nomeServico: "Teste",
			serviceID:   "abc",
			expected:    "teste-abc",
		},
		{
			name:        "nome vazio",
			nomeServico: "",
			serviceID:   "abc123def456",
			expected:    "",
		},
		{
			name:        "ID vazio",
			nomeServico: "Teste",
			serviceID:   "",
			expected:    "",
		},
		{
			name:        "ambos vazios",
			nomeServico: "",
			serviceID:   "",
			expected:    "",
		},
		{
			name:        "nome com caracteres especiais apenas",
			nomeServico: "!@#$%^&*()",
			serviceID:   "abc123def456",
			expected:    "abc123de",
		},
		{
			name:        "nome com múltiplos espaços",
			nomeServico: "Serviço   de   Teste",
			serviceID:   "xyz789abc123",
			expected:    "servico-de-teste-xyz789ab",
		},
		{
			name:        "nome com hífen",
			nomeServico: "Auto-Escola",
			serviceID:   "aaa111bbb222",
			expected:    "auto-escola-aaa111bb",
		},
		{
			name:        "nome com underscores",
			nomeServico: "Serviço_de_Teste",
			serviceID:   "bbb222ccc333",
			expected:    "servico-de-teste-bbb222cc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSlug(tt.nomeServico, tt.serviceID)
			if result != tt.expected {
				t.Errorf("GenerateSlug(%q, %q) = %q; expected %q",
					tt.nomeServico, tt.serviceID, result, tt.expected)
			}
		})
	}
}

func TestGenerateSlug_Truncation(t *testing.T) {
	longName := strings.Repeat("Serviço de Atendimento ", 10) // ~230 chars
	serviceID := "abc123def456"

	result := GenerateSlug(longName, serviceID)

	// Verifica que o slug base (sem o ID) não excede MaxSlugBaseLength
	parts := strings.Split(result, "-")
	if len(parts) < 2 {
		t.Errorf("Slug deveria ter formato nome-id, got: %q", result)
		return
	}

	// Reconstrói o slug base (tudo exceto o último elemento que é o ID)
	slugBase := strings.Join(parts[:len(parts)-1], "-")
	if len(slugBase) > MaxSlugBaseLength {
		t.Errorf("Slug base deveria ter no máximo %d chars, got %d: %q",
			MaxSlugBaseLength, len(slugBase), slugBase)
	}

	// Verifica que termina com o short ID
	shortID := serviceID[:ShortIDLength]
	if !strings.HasSuffix(result, "-"+shortID) {
		t.Errorf("Slug deveria terminar com -%s, got: %q", shortID, result)
	}
}

func TestGenerateSlug_Consistency(t *testing.T) {
	// Mesmo input deve sempre gerar mesmo output
	nomeServico := "Matrícula Escolar"
	serviceID := "abc123def456"

	result1 := GenerateSlug(nomeServico, serviceID)
	result2 := GenerateSlug(nomeServico, serviceID)

	if result1 != result2 {
		t.Errorf("GenerateSlug não é consistente: %q != %q", result1, result2)
	}
}

func TestGenerateSlug_NoConsecutiveHyphens(t *testing.T) {
	tests := []string{
		"Serviço -- de -- Teste",
		"   Espaços   Múltiplos   ",
		"Caracteres!@#Especiais$%^",
	}

	for _, nomeServico := range tests {
		result := GenerateSlug(nomeServico, "abc123def456")
		if strings.Contains(result, "--") {
			t.Errorf("Slug contém hífens consecutivos: %q", result)
		}
	}
}

func TestGenerateSlug_NoLeadingTrailingHyphens(t *testing.T) {
	tests := []string{
		" Espaço no início",
		"Espaço no final ",
		" Espaços em ambos ",
		"-Hífen no início",
		"Hífen no final-",
	}

	for _, nomeServico := range tests {
		result := GenerateSlug(nomeServico, "abc123def456")
		// O slug começa com o nome, não com hífen
		parts := strings.SplitN(result, "-", 2)
		if len(parts) > 0 && parts[0] == "" {
			t.Errorf("Slug não deveria começar com hífen: %q", result)
		}
	}
}

