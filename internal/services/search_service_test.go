package services

import (
	"fmt"
	"testing"
)

// TestScoreNormalizationComparison demonstra a diferença entre normalização antiga e nova
func TestScoreNormalizationComparison(t *testing.T) {
	// Exemplo: resultados de uma busca por "IPTU" com diferentes vector distances
	testCases := []struct {
		name           string
		vectorDistance float64
	}{
		{"IPTU Certidão (melhor)", 0.25},
		{"Matrícula Escolar", 0.35},
		{"Notificações", 0.37},
		{"Cidade das Artes", 0.39},
		{"Painel Transparência (pior)", 0.40},
	}

	// Calcular min/max para normalização
	minDist := testCases[0].vectorDistance
	maxDist := testCases[len(testCases)-1].vectorDistance
	maxSimilarity := 1.0 - (minDist / 2.0) // similarity absoluta do melhor

	fmt.Println("\n=== Comparação de Normalização de Scores ===")
	fmt.Printf("\nMin distance: %.2f | Max distance: %.2f | MaxSimilarity: %.3f\n\n", minDist, maxDist, maxSimilarity)
	fmt.Println("Documento                      | Vector Dist | Antiga (fixa) | Nova (0 a max) | Diferença")
	fmt.Println("-------------------------------|-------------|---------------|----------------|----------")

	for _, tc := range testCases {
		// Normalização antiga: 1.0 - (vd / 2.0)
		oldSimilarity := 1.0 - (tc.vectorDistance / 2.0)

		// Normalização nova: pior = 0, melhor = maxSimilarity
		proportion := 1.0 - ((tc.vectorDistance - minDist) / (maxDist - minDist))
		newSimilarity := proportion * maxSimilarity

		diff := newSimilarity - oldSimilarity

		fmt.Printf("%-30s | %.2f      | %.3f         | %.3f          | %+.3f\n",
			tc.name, tc.vectorDistance, oldSimilarity, newSimilarity, diff)
	}

	// Calcular variação (melhor - pior)
	oldRange := (1.0 - (minDist / 2.0)) - (1.0 - (maxDist / 2.0))
	newRange := maxSimilarity - 0.0

	fmt.Println("\n=== Análise de Variação ===")
	fmt.Printf("Antiga - Range de scores: %.3f (%.1f%% do total possível)\n", oldRange, oldRange*100)
	fmt.Printf("Nova   - Range de scores: %.3f (%.1f%% do total possível)\n", newRange, newRange*100)
	fmt.Printf("Melhoria: %.1fx mais variação\n", newRange/oldRange)
}

// TestMinMaxEdgeCases testa casos extremos da normalização
func TestMinMaxEdgeCases(t *testing.T) {
	t.Run("Todos os resultados com mesma distance", func(t *testing.T) {
		// Se todos os docs têm a mesma distance, similarity deve ser maxSimilarity
		minDist := 0.5
		maxDist := 0.5
		vd := 0.5
		maxSimilarity := 1.0 - (minDist / 2.0)

		var similarity float64
		if maxDist > minDist {
			proportion := 1.0 - ((vd - minDist) / (maxDist - minDist))
			similarity = proportion * maxSimilarity
		} else {
			similarity = maxSimilarity
		}

		if similarity != maxSimilarity {
			t.Errorf("Expected %.3f for identical distances, got %.3f", maxSimilarity, similarity)
		}
	})

	t.Run("Single resultado", func(t *testing.T) {
		// Com um único resultado, min == max, similarity deve ser maxSimilarity
		minDist := 0.3
		maxDist := 0.3
		vd := 0.3
		maxSimilarity := 1.0 - (minDist / 2.0)

		var similarity float64
		if maxDist > minDist {
			proportion := 1.0 - ((vd - minDist) / (maxDist - minDist))
			similarity = proportion * maxSimilarity
		} else {
			similarity = maxSimilarity
		}

		if similarity != maxSimilarity {
			t.Errorf("Expected %.3f for single result, got %.3f", maxSimilarity, similarity)
		}
	})

	t.Run("Melhor resultado (min distance)", func(t *testing.T) {
		minDist := 0.2
		maxDist := 0.8
		vd := minDist // melhor resultado
		maxSimilarity := 1.0 - (minDist / 2.0)

		proportion := 1.0 - ((vd - minDist) / (maxDist - minDist))
		similarity := proportion * maxSimilarity

		if similarity != maxSimilarity {
			t.Errorf("Expected %.3f for best result, got %.3f", maxSimilarity, similarity)
		}
	})

	t.Run("Pior resultado (max distance)", func(t *testing.T) {
		minDist := 0.2
		maxDist := 0.8
		vd := maxDist // pior resultado
		maxSimilarity := 1.0 - (minDist / 2.0)

		proportion := 1.0 - ((vd - minDist) / (maxDist - minDist))
		similarity := proportion * maxSimilarity

		if similarity != 0.0 {
			t.Errorf("Expected 0.0 for worst result, got %.3f", similarity)
		}
	})

	t.Run("Resultado médio", func(t *testing.T) {
		minDist := 0.2
		maxDist := 0.8
		vd := 0.5 // meio do range
		maxSimilarity := 1.0 - (minDist / 2.0)

		proportion := 1.0 - ((vd - minDist) / (maxDist - minDist))
		similarity := proportion * maxSimilarity

		expected := 0.5 * maxSimilarity
		tolerance := 0.001
		if similarity < expected-tolerance || similarity > expected+tolerance {
			t.Errorf("Expected %.3f for middle result, got %.3f", expected, similarity)
		}
	})
}
