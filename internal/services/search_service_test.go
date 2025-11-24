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

	// Calcular min/max para normalização min-max
	minDist := testCases[0].vectorDistance
	maxDist := testCases[len(testCases)-1].vectorDistance

	fmt.Println("\n=== Comparação de Normalização de Scores ===")
	fmt.Printf("\nMin distance: %.2f | Max distance: %.2f | Range: %.2f\n\n", minDist, maxDist, maxDist-minDist)
	fmt.Println("Documento                      | Vector Dist | Antiga (fixa) | Nova (min-max) | Diferença")
	fmt.Println("-------------------------------|-------------|---------------|----------------|----------")

	for _, tc := range testCases {
		// Normalização antiga: 1.0 - (vd / 2.0)
		oldSimilarity := 1.0 - (tc.vectorDistance / 2.0)

		// Normalização nova: min-max
		newSimilarity := 1.0 - ((tc.vectorDistance - minDist) / (maxDist - minDist))

		diff := newSimilarity - oldSimilarity

		fmt.Printf("%-30s | %.2f      | %.3f         | %.3f          | %+.3f\n",
			tc.name, tc.vectorDistance, oldSimilarity, newSimilarity, diff)
	}

	// Calcular variação (melhor - pior)
	oldRange := (1.0 - (minDist / 2.0)) - (1.0 - (maxDist / 2.0))
	newRange := 1.0 - 0.0

	fmt.Println("\n=== Análise de Variação ===")
	fmt.Printf("Antiga - Range de scores: %.3f (%.1f%% do total possível)\n", oldRange, oldRange*100)
	fmt.Printf("Nova   - Range de scores: %.3f (%.1f%% do total possível)\n", newRange, newRange*100)
	fmt.Printf("Melhoria: %.1fx mais variação\n", newRange/oldRange)
}

// TestMinMaxEdgeCases testa casos extremos da normalização min-max
func TestMinMaxEdgeCases(t *testing.T) {
	t.Run("Todos os resultados com mesma distance", func(t *testing.T) {
		// Se todos os docs têm a mesma distance, similarity deve ser 1.0
		minDist := 0.5
		maxDist := 0.5
		vd := 0.5

		var similarity float64
		if maxDist > minDist {
			similarity = 1.0 - ((vd - minDist) / (maxDist - minDist))
		} else {
			similarity = 1.0
		}

		if similarity != 1.0 {
			t.Errorf("Expected 1.0 for identical distances, got %.2f", similarity)
		}
	})

	t.Run("Single resultado", func(t *testing.T) {
		// Com um único resultado, min == max, similarity deve ser 1.0
		minDist := 0.3
		maxDist := 0.3
		vd := 0.3

		var similarity float64
		if maxDist > minDist {
			similarity = 1.0 - ((vd - minDist) / (maxDist - minDist))
		} else {
			similarity = 1.0
		}

		if similarity != 1.0 {
			t.Errorf("Expected 1.0 for single result, got %.2f", similarity)
		}
	})

	t.Run("Melhor resultado (min distance)", func(t *testing.T) {
		minDist := 0.2
		maxDist := 0.8
		vd := minDist // melhor resultado

		similarity := 1.0 - ((vd - minDist) / (maxDist - minDist))

		if similarity != 1.0 {
			t.Errorf("Expected 1.0 for best result, got %.2f", similarity)
		}
	})

	t.Run("Pior resultado (max distance)", func(t *testing.T) {
		minDist := 0.2
		maxDist := 0.8
		vd := maxDist // pior resultado

		similarity := 1.0 - ((vd - minDist) / (maxDist - minDist))

		if similarity != 0.0 {
			t.Errorf("Expected 0.0 for worst result, got %.2f", similarity)
		}
	})

	t.Run("Resultado médio", func(t *testing.T) {
		minDist := 0.2
		maxDist := 0.8
		vd := 0.5 // meio do range

		similarity := 1.0 - ((vd - minDist) / (maxDist - minDist))

		expected := 0.5
		tolerance := 0.001
		if similarity < expected-tolerance || similarity > expected+tolerance {
			t.Errorf("Expected %.2f for middle result, got %.2f", expected, similarity)
		}
	})
}
