package ranking

import "math"

// Normalizer normaliza scores para escala 0-1
type Normalizer struct {
	// Para normalização min-max de vetores
	minVectorDist float64
	maxVectorDist float64
	maxSimilarity float64
}

// NewNormalizer cria um novo normalizer
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// SetVectorBounds define os limites para normalização de distância vetorial
func (n *Normalizer) SetVectorBounds(minDist, maxDist float64) {
	n.minVectorDist = minDist
	n.maxVectorDist = maxDist
	// Similaridade absoluta do melhor resultado
	n.maxSimilarity = 1.0 - (minDist / 2.0)
}

// LogNormalize normaliza text_match usando log normalization
func (n *Normalizer) LogNormalize(score float64) float64 {
	const maxObserved = 100000.0
	if score <= 0 {
		return 0.0
	}
	normalized := math.Log1p(score) / math.Log1p(maxObserved)
	return math.Min(1.0, normalized)
}

// MinMaxNormalizeVector normaliza vector_distance usando min-max
// Retorna similaridade onde: melhor resultado = maxSimilarity, pior = 0
func (n *Normalizer) MinMaxNormalizeVector(distance float64) float64 {
	if n.maxVectorDist <= n.minVectorDist {
		// Edge case: todos iguais
		return n.maxSimilarity
	}

	// Proporção inversa (menor distância = maior similaridade)
	proportion := 1.0 - ((distance - n.minVectorDist) / (n.maxVectorDist - n.minVectorDist))
	similarity := proportion * n.maxSimilarity

	return math.Max(0.0, math.Min(n.maxSimilarity, similarity))
}

// NormalizeVectorSimple normaliza distância vetorial de forma simples
// similarity = 1 - (distance / 2)
func (n *Normalizer) NormalizeVectorSimple(distance float64) float64 {
	similarity := 1.0 - (distance / 2.0)
	return math.Max(0.0, math.Min(1.0, similarity))
}
