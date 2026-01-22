package search

import (
	"testing"

	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search/query"
)

func TestParser(t *testing.T) {
	parser := query.NewParser()

	tests := []struct {
		name     string
		input    string
		wantNorm string
		wantSigla bool
	}{
		{
			name:     "Query simples",
			input:    "IPTU",
			wantNorm: "iptu",
			wantSigla: true,
		},
		{
			name:     "Query com espaços extras",
			input:    "  pagar   iptu  ",
			wantNorm: "pagar iptu",
			wantSigla: true,
		},
		{
			name:     "Query com acentos",
			input:    "certidão de nascimento",
			wantNorm: "certidão de nascimento",
			wantSigla: false,
		},
		{
			name:     "Query em linguagem natural",
			input:    "Como faço para tirar meu RG?",
			wantNorm: "como faço para tirar meu rg",
			wantSigla: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.Parse(tt.input)
			
			if result.Normalized != tt.wantNorm {
				t.Errorf("Normalized = %q, want %q", result.Normalized, tt.wantNorm)
			}
			
			if result.HasSigla != tt.wantSigla {
				t.Errorf("HasSigla = %v, want %v", result.HasSigla, tt.wantSigla)
			}
		})
	}
}

func TestExpander(t *testing.T) {
	expander := query.NewExpander(nil, 5)
	parser := query.NewParser()

	tests := []struct {
		name          string
		input         string
		wantExpanded  bool // se deve ter termos expandidos além dos originais
	}{
		{
			name:         "Query com sinônimo conhecido (iptu)",
			input:        "iptu",
			wantExpanded: true,
		},
		{
			name:         "Query com sinônimo conhecido (pagar)",
			input:        "pagar conta",
			wantExpanded: true,
		},
		{
			name:         "Query sem sinônimo conhecido",
			input:        "xyz123",
			wantExpanded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parser.Parse(tt.input)
			result := expander.ExpandSimple(parsed)
			
			hasExpansion := len(result.ExpandedTerms) > len(parsed.Tokens)
			
			if hasExpansion != tt.wantExpanded {
				t.Errorf("HasExpansion = %v, want %v. Tokens: %v, Expanded: %v", 
					hasExpansion, tt.wantExpanded, parsed.Tokens, result.ExpandedTerms)
			}
		})
	}
}

func TestSearchConfig(t *testing.T) {
	t.Run("DefaultHumanConfig", func(t *testing.T) {
		config := v3.DefaultHumanConfig()
		
		if config.NumTypos != 2 {
			t.Errorf("NumTypos = %d, want 2", config.NumTypos)
		}
		if config.Alpha != 0.3 {
			t.Errorf("Alpha = %f, want 0.3", config.Alpha)
		}
		if !config.EnableExpansion {
			t.Error("EnableExpansion should be true for human mode")
		}
		if !config.RecencyBoost {
			t.Error("RecencyBoost should be true for human mode")
		}
	})

	t.Run("DefaultAgentConfig", func(t *testing.T) {
		config := v3.DefaultAgentConfig()
		
		if config.NumTypos != 1 {
			t.Errorf("NumTypos = %d, want 1", config.NumTypos)
		}
		if config.Alpha != 0.5 {
			t.Errorf("Alpha = %f, want 0.5", config.Alpha)
		}
		if config.EnableExpansion {
			t.Error("EnableExpansion should be false for agent mode")
		}
		if config.RecencyBoost {
			t.Error("RecencyBoost should be false for agent mode")
		}
	})
}

func TestSearchRequest(t *testing.T) {
	t.Run("Validate - missing query", func(t *testing.T) {
		req := &v3.SearchRequest{
			Type: v3.SearchTypeHybrid,
		}
		err := req.Validate()
		if err != v3.ErrQueryRequired {
			t.Errorf("Expected ErrQueryRequired, got %v", err)
		}
	})

	t.Run("Validate - invalid type", func(t *testing.T) {
		req := &v3.SearchRequest{
			Query: "teste",
			Type:  "invalid",
		}
		err := req.Validate()
		if err != v3.ErrInvalidSearchType {
			t.Errorf("Expected ErrInvalidSearchType, got %v", err)
		}
	})

	t.Run("Validate - valid request", func(t *testing.T) {
		req := &v3.SearchRequest{
			Query: "iptu",
			Type:  v3.SearchTypeHybrid,
		}
		err := req.Validate()
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		
		// Check defaults applied
		if req.Page != 1 {
			t.Errorf("Page = %d, want 1", req.Page)
		}
		if req.PerPage != 10 {
			t.Errorf("PerPage = %d, want 10", req.PerPage)
		}
		if req.Mode != v3.SearchModeHuman {
			t.Errorf("Mode = %s, want human", req.Mode)
		}
	})

	t.Run("GetTypos - human mode", func(t *testing.T) {
		req := &v3.SearchRequest{
			Query: "teste",
			Type:  v3.SearchTypeHybrid,
			Mode:  v3.SearchModeHuman,
		}
		if req.GetTypos() != 2 {
			t.Errorf("GetTypos() = %d, want 2", req.GetTypos())
		}
	})

	t.Run("GetTypos - agent mode", func(t *testing.T) {
		req := &v3.SearchRequest{
			Query: "teste",
			Type:  v3.SearchTypeHybrid,
			Mode:  v3.SearchModeAgent,
		}
		if req.GetTypos() != 1 {
			t.Errorf("GetTypos() = %d, want 1", req.GetTypos())
		}
	})
}
