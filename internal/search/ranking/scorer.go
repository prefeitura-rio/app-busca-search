package ranking

import (
	"math"
	"sort"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models/v3"
)

// ScoreResult contém os scores calculados
type ScoreResult struct {
	TextScore    float64
	VectorScore  float64
	HybridScore  float64
	RecencyScore float64
	FinalScore   float64
}

// Hit representa um resultado do Typesense
type Hit struct {
	Document       map[string]interface{}
	TextMatch      *int64
	VectorDistance *float32
}

// Scorer calcula scores para resultados de busca
type Scorer struct {
	normalizer *Normalizer
	config     *v3.SearchConfig
}

// NewScorer cria um novo scorer
func NewScorer(config *v3.SearchConfig) *Scorer {
	return &Scorer{
		normalizer: NewNormalizer(),
		config:     config,
	}
}

// PrepareNormalization calcula bounds para normalização min-max
func (s *Scorer) PrepareNormalization(hits []Hit) {
	if len(hits) == 0 {
		return
	}

	minDist := math.MaxFloat64
	maxDist := -math.MaxFloat64

	for _, hit := range hits {
		if hit.VectorDistance != nil {
			dist := float64(*hit.VectorDistance)
			if dist < minDist {
				minDist = dist
			}
			if dist > maxDist {
				maxDist = dist
			}
		}
	}

	s.normalizer.SetVectorBounds(minDist, maxDist)
}

// Calculate calcula scores para um hit
func (s *Scorer) Calculate(hit Hit, searchType v3.SearchType) *ScoreResult {
	result := &ScoreResult{}

	// Text score (log normalization)
	if hit.TextMatch != nil {
		result.TextScore = s.normalizer.LogNormalize(float64(*hit.TextMatch))
	}

	// Vector score (min-max normalization)
	if hit.VectorDistance != nil {
		result.VectorScore = s.normalizer.MinMaxNormalizeVector(float64(*hit.VectorDistance))
	}

	// Hybrid score
	switch searchType {
	case v3.SearchTypeKeyword:
		result.HybridScore = result.TextScore
	case v3.SearchTypeSemantic:
		result.HybridScore = result.VectorScore
	case v3.SearchTypeHybrid, v3.SearchTypeAI:
		alpha := s.config.Alpha
		result.HybridScore = alpha*result.TextScore + (1-alpha)*result.VectorScore
	}

	// Recency score
	if s.config.RecencyBoost {
		result.RecencyScore = s.calculateRecency(hit.Document)
		result.FinalScore = result.HybridScore * result.RecencyScore
	} else {
		result.RecencyScore = 1.0
		result.FinalScore = result.HybridScore
	}

	return result
}

// RankDocuments ordena documentos por score
func (s *Scorer) RankDocuments(docs []v3.Document) {
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Score.Final > docs[j].Score.Final
	})
}

// FilterByThreshold filtra documentos abaixo do threshold
func (s *Scorer) FilterByThreshold(docs []v3.Document, threshold float64) []v3.Document {
	if threshold <= 0 {
		return docs
	}

	filtered := make([]v3.Document, 0, len(docs))
	for _, doc := range docs {
		if doc.Score.Final >= threshold {
			filtered = append(filtered, doc)
		}
	}
	return filtered
}

// calculateRecency calcula fator de recência
func (s *Scorer) calculateRecency(doc map[string]interface{}) float64 {
	var timestamp int64

	if v, ok := doc["last_update"].(float64); ok {
		timestamp = int64(v)
	} else if v, ok := doc["last_update"].(int64); ok {
		timestamp = v
	} else if v, ok := doc["updated_at"].(float64); ok {
		timestamp = int64(v)
	} else if v, ok := doc["updated_at"].(int64); ok {
		timestamp = v
	}

	if timestamp <= 0 {
		return 0.5 // Docs sem data recebem fator mínimo
	}

	now := time.Now().Unix()
	daysSinceUpdate := float64(now-timestamp) / 86400.0

	gracePeriodDays := float64(s.config.RecencyGracePeriodDays)
	if gracePeriodDays <= 0 {
		gracePeriodDays = 30.0
	}

	decay := s.config.RecencyDecay
	if decay <= 0 {
		decay = 0.00207 // ~0.5 em 1 ano
	}

	if daysSinceUpdate <= gracePeriodDays {
		return 1.0
	}

	daysAfterGrace := daysSinceUpdate - gracePeriodDays
	factor := math.Exp(-decay * daysAfterGrace)

	return math.Max(0.5, factor)
}
