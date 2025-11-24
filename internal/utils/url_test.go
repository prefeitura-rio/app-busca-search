package utils

import (
	"testing"
)

func TestWrapURLIfNeeded(t *testing.T) {
	gatewayBase := "https://gateway-idrio.apps.rio.gov.br"

	tests := []struct {
		name     string
		input    string
		gateway  string
		expected string
	}{
		{
			name:     "services-carioca URL should be wrapped",
			input:    "https://services-carioca.rio.rj.gov.br/group/guest/agendamento-iss",
			gateway:  gatewayBase,
			expected: "https://gateway-idrio.apps.rio.gov.br/gateway?urlServico=https%3A%2F%2Fservices-carioca.rio.rj.gov.br%2Fgroup%2Fguest%2Fagendamento-iss",
		},
		{
			name:     "acesso.processo.rio URL should be wrapped",
			input:    "https://acesso.processo.rio/some/path",
			gateway:  gatewayBase,
			expected: "https://gateway-idrio.apps.rio.gov.br/gateway?urlServico=https%3A%2F%2Facesso.processo.rio%2Fsome%2Fpath",
		},
		{
			name:     "other domain should not be wrapped",
			input:    "https://example.com/path",
			gateway:  gatewayBase,
			expected: "https://example.com/path",
		},
		{
			name:     "empty URL should return empty",
			input:    "",
			gateway:  gatewayBase,
			expected: "",
		},
		{
			name:     "already wrapped URL should not be wrapped again",
			input:    "https://gateway-idrio.apps.rio.gov.br/gateway?urlServico=https%3A%2F%2Fservices-carioca.rio.rj.gov.br%2Ftest",
			gateway:  gatewayBase,
			expected: "https://gateway-idrio.apps.rio.gov.br/gateway?urlServico=https%3A%2F%2Fservices-carioca.rio.rj.gov.br%2Ftest",
		},
		{
			name:     "no gateway configured should return original",
			input:    "https://services-carioca.rio.rj.gov.br/test",
			gateway:  "",
			expected: "https://services-carioca.rio.rj.gov.br/test",
		},
		{
			name:     "HTTP protocol should also work",
			input:    "http://services-carioca.rio.rj.gov.br/test",
			gateway:  gatewayBase,
			expected: "https://gateway-idrio.apps.rio.gov.br/gateway?urlServico=http%3A%2F%2Fservices-carioca.rio.rj.gov.br%2Ftest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapURLIfNeeded(tt.input, tt.gateway)
			if result != tt.expected {
				t.Errorf("WrapURLIfNeeded() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWrapURLsInArray(t *testing.T) {
	gatewayBase := "https://gateway-idrio.apps.rio.gov.br"

	tests := []struct {
		name     string
		input    []string
		gateway  string
		expected []string
	}{
		{
			name: "mixed URLs should wrap only target domains",
			input: []string{
				"https://services-carioca.rio.rj.gov.br/test1",
				"https://example.com/test2",
				"https://acesso.processo.rio/test3",
			},
			gateway: gatewayBase,
			expected: []string{
				"https://gateway-idrio.apps.rio.gov.br/gateway?urlServico=https%3A%2F%2Fservices-carioca.rio.rj.gov.br%2Ftest1",
				"https://example.com/test2",
				"https://gateway-idrio.apps.rio.gov.br/gateway?urlServico=https%3A%2F%2Facesso.processo.rio%2Ftest3",
			},
		},
		{
			name:     "empty array should return empty",
			input:    []string{},
			gateway:  gatewayBase,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapURLsInArray(tt.input, tt.gateway)
			if len(result) != len(tt.expected) {
				t.Errorf("WrapURLsInArray() length = %v, want %v", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("WrapURLsInArray()[%d] = %v, want %v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}
