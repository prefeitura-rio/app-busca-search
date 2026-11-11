package config

import "testing"

// DEV-511: text-embedding-004 foi desligado pelo Google (404 em toda chamada); o modelo
// configurado, venha do default ou de GEMINI_EMBEDDING_MODEL, tem de ser um vigente.
func TestResolveGeminiEmbeddingModel(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "vazio usa o default", configured: "", want: DefaultGeminiEmbeddingModel},
		{name: "só espaços usa o default", configured: "   ", want: DefaultGeminiEmbeddingModel},
		{name: "text-embedding-004 desligado", configured: "text-embedding-004", want: DefaultGeminiEmbeddingModel},
		{name: "text-embedding-004 com prefixo models/", configured: "models/text-embedding-004", want: DefaultGeminiEmbeddingModel},
		{name: "embedding-001 desligado", configured: "embedding-001", want: DefaultGeminiEmbeddingModel},
		{name: "gemini-embedding-exp desligado", configured: "gemini-embedding-exp-03-07", want: DefaultGeminiEmbeddingModel},
		{name: "modelo vigente é mantido", configured: "gemini-embedding-001", want: "gemini-embedding-001"},
		{name: "outro modelo vigente é mantido", configured: "gemini-embedding-2", want: "gemini-embedding-2"},
		{name: "espaços em volta são removidos", configured: " gemini-embedding-001 ", want: "gemini-embedding-001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveGeminiEmbeddingModel(tt.configured); got != tt.want {
				t.Fatalf("ResolveGeminiEmbeddingModel(%q) = %q, want %q", tt.configured, got, tt.want)
			}
		})
	}
}

func TestDefaultGeminiEmbeddingModelIsNotRetired(t *testing.T) {
	if retiredGeminiEmbeddingModels[DefaultGeminiEmbeddingModel] {
		t.Fatalf("o default %q está na lista de modelos desligados", DefaultGeminiEmbeddingModel)
	}
}

func TestLoadConfig_GeminiEmbeddingModel(t *testing.T) {
	t.Setenv("SEARCHABLE_COLLECTIONS", "prefrio_services_base")
	t.Setenv("COLLECTION_CONFIGS", `{"prefrio_services_base":{"type":"service"}}`)

	t.Run("variável vazia usa o modelo vigente", func(t *testing.T) {
		t.Setenv("GEMINI_EMBEDDING_MODEL", "")
		if got := LoadConfig().GeminiEmbeddingModel; got != DefaultGeminiEmbeddingModel {
			t.Fatalf("GeminiEmbeddingModel = %q, want %q", got, DefaultGeminiEmbeddingModel)
		}
	})

	t.Run("variável com modelo desligado é corrigida", func(t *testing.T) {
		t.Setenv("GEMINI_EMBEDDING_MODEL", "text-embedding-004")
		if got := LoadConfig().GeminiEmbeddingModel; got != DefaultGeminiEmbeddingModel {
			t.Fatalf("GeminiEmbeddingModel = %q, want %q", got, DefaultGeminiEmbeddingModel)
		}
	})

	t.Run("variável com modelo vigente é respeitada", func(t *testing.T) {
		t.Setenv("GEMINI_EMBEDDING_MODEL", "gemini-embedding-2")
		if got := LoadConfig().GeminiEmbeddingModel; got != "gemini-embedding-2" {
			t.Fatalf("GeminiEmbeddingModel = %q, want %q", got, "gemini-embedding-2")
		}
	})
}
