package utils

import (
	"testing"
)

func TestNormalizarCategoria(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Saúde", "saude"},
		{"Educação", "educacao"},
		{"Família", "familia"},
		{"Licenças", "licencas"},
		{"Emergência", "emergencia"},
		{"Segurança", "seguranca"},
		{"Cidade", "cidade"},
		{"Transporte", "transporte"},
		{"", ""},
	}

	for _, test := range tests {
		result := NormalizarCategoria(test.input)
		if result != test.expected {
			t.Errorf("NormalizarCategoria(%q) = %q; expected %q", test.input, result, test.expected)
		}
	}
}

func TestDesnormalizarCategoria(t *testing.T) {
	categoriasValidas := []string{
		"Saúde", "Educação", "Família", "Licenças", "Emergência", "Segurança",
		"Cidade", "Transporte", "Ambiente", "Taxas", "Cidadania", "Servidor",
		"Trabalho", "Cultura", "Esportes", "Animais",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"saude", "Saúde"},
		{"educacao", "Educação"},
		{"familia", "Família"},
		{"licencas", "Licenças"},
		{"emergencia", "Emergência"},
		{"seguranca", "Segurança"},
		{"cidade", "Cidade"},
		{"transporte", "Transporte"},
		{"categoria_inexistente", "categoria_inexistente"}, // Retorna o que foi passado se não encontrar
	}

	for _, test := range tests {
		result := DesnormalizarCategoria(test.input, categoriasValidas)
		if result != test.expected {
			t.Errorf("DesnormalizarCategoria(%q) = %q; expected %q", test.input, result, test.expected)
		}
	}
} 