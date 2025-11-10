package utils

import (
	"testing"
)

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text without markdown",
			input:    "This is plain text",
			expected: "This is plain text",
		},
		{
			name:     "bold text",
			input:    "This is **bold** text",
			expected: "This is bold text",
		},
		{
			name:     "italic text",
			input:    "This is *italic* text",
			expected: "This is italic text",
		},
		{
			name:     "escaped asterisk",
			input:    "This is a \\*literal asterisk\\* not emphasis",
			expected: "This is a *literal asterisk* not emphasis",
		},
		{
			name:     "escaped underscore",
			input:    "This is a \\_literal underscore\\_",
			expected: "This is a _literal underscore_",
		},
		{
			name:     "link",
			input:    "Visit [Google](https://google.com) for search",
			expected: "Visit Google for search",
		},
		{
			name:     "heading",
			input:    "# Main Title\n\nSome content",
			expected: "Main Title\n\nSome content",
		},
		{
			name:     "code inline",
			input:    "Use the `StripMarkdown` function",
			expected: "Use the StripMarkdown function",
		},
		{
			name:     "code block",
			input:    "```go\nfunc main() {}\n```",
			expected: "func main() {}",
		},
		{
			name:     "unordered list",
			input:    "- Item 1\n- Item 2\n- Item 3",
			expected: "• Item 1\n\n• Item 2\n\n• Item 3",
		},
		{
			name:     "mixed formatting",
			input:    "This has **bold**, *italic*, and [a link](http://example.com)",
			expected: "This has bold, italic, and a link",
		},
		{
			name:     "blockquote",
			input:    "> This is a quote\n> With multiple lines",
			expected: "This is a quote\nWith multiple lines",
		},
		{
			name:     "complex markdown with escaped chars",
			input:    "# Title\n\nThis is a \\*service\\* with **formatting** and a [link](http://example.com).\n\n- Item 1\n- Item 2",
			expected: "Title\n\nThis is a *service* with formatting and a link.\n\n• Item 1\n\n• Item 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("StripMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStripMarkdownArray(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil array",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty array",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single element",
			input:    []string{"**bold** text"},
			expected: []string{"bold text"},
		},
		{
			name: "multiple elements with markdown",
			input: []string{
				"**RG** (Registro Geral)",
				"*CPF* (Cadastro de Pessoa Física)",
				"Comprovante de [residência](http://example.com)",
			},
			expected: []string{
				"RG (Registro Geral)",
				"CPF (Cadastro de Pessoa Física)",
				"Comprovante de residência",
			},
		},
		{
			name: "mixed plain and markdown",
			input: []string{
				"Plain text document",
				"Document with **bold**",
				"Another plain one",
			},
			expected: []string{
				"Plain text document",
				"Document with bold",
				"Another plain one",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripMarkdownArray(tt.input)

			if (result == nil) != (tt.expected == nil) {
				t.Errorf("StripMarkdownArray() nil mismatch: got %v, want %v", result == nil, tt.expected == nil)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("StripMarkdownArray() length = %d, want %d", len(result), len(tt.expected))
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("StripMarkdownArray()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func BenchmarkStripMarkdown(b *testing.B) {
	input := `# Serviço de Emissão de Documentos

Este é um serviço que permite a **emissão** de diversos *documentos* oficiais.

## Documentos Disponíveis

- RG (Registro Geral)
- CPF (Cadastro de Pessoa Física)
- Certidão de [Nascimento](http://example.com)

Para mais informações, acesse o \*portal oficial\*.`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StripMarkdown(input)
	}
}
