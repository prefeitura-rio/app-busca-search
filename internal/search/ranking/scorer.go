package ranking

import (
	"math"
	"sort"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models/v3"
)

// PopularityProvider interface para obter popularidade de categorias
type PopularityProvider interface {
	GetCategoryPopularity(category string) int
}

// ScoreResult contém os scores calculados
type ScoreResult struct {
	TextScore       float64
	VectorScore     float64
	HybridScore     float64
	RecencyScore    float64
	PopularityScore float64
	FinalScore      float64
}

// Hit representa um resultado do Typesense
type Hit struct {
	Document       map[string]interface{}
	TextMatch      *int64
	VectorDistance *float32
}

// Scorer calcula scores para resultados de busca
type Scorer struct {
	normalizer  *Normalizer
	config      *v3.SearchConfig
	popularity  PopularityProvider
	maxPopularity float64
}

// NewScorer cria um novo scorer
func NewScorer(config *v3.SearchConfig) *Scorer {
	return &Scorer{
		normalizer:    NewNormalizer(),
		config:        config,
		maxPopularity: 5000.0, // Maior valor de popularidade
	}
}

// NewScorerWithPopularity cria um scorer com serviço de popularidade
func NewScorerWithPopularity(config *v3.SearchConfig, popularity PopularityProvider) *Scorer {
	return &Scorer{
		normalizer:    NewNormalizer(),
		config:        config,
		popularity:    popularity,
		maxPopularity: 5000.0,
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

	// Hybrid score com fallback para documentos sem embedding
	switch searchType {
	case v3.SearchTypeKeyword:
		result.HybridScore = result.TextScore
	case v3.SearchTypeSemantic:
		if result.VectorScore == 0 {
			if result.TextScore > 0 {
				// Fallback: usa text score se não há embedding
				result.HybridScore = result.TextScore * 0.5
			} else {
				// Sem embedding e sem text match = score mínimo
				result.HybridScore = 0.01
			}
		} else {
			result.HybridScore = result.VectorScore
		}
	case v3.SearchTypeHybrid, v3.SearchTypeAI:
		alpha := s.config.Alpha
		if result.VectorScore == 0 && result.TextScore == 0 {
			// Sem embedding e sem text match = score mínimo
			result.HybridScore = 0.01
		} else if result.VectorScore == 0 && result.TextScore > 0 {
			// Sem embedding mas tem text match: usa text score com penalidade
			result.HybridScore = result.TextScore * 0.7
		} else if result.TextScore == 0 && result.VectorScore > 0 {
			// Sem text match mas tem embedding: usa vector score com penalidade
			result.HybridScore = result.VectorScore * 0.8
		} else {
			result.HybridScore = alpha*result.TextScore + (1-alpha)*result.VectorScore
		}
	}

	// Recency score
	if s.config.RecencyBoost {
		result.RecencyScore = s.calculateRecency(hit.Document)
	} else {
		result.RecencyScore = 1.0
	}

	// Popularity score
	result.PopularityScore = s.calculatePopularity(hit.Document)

	// Final score: hybrid * recency * popularity
	result.FinalScore = result.HybridScore * result.RecencyScore * result.PopularityScore

	return result
}

// calculatePopularity calcula fator de popularidade (1.0 - 1.1)
func (s *Scorer) calculatePopularity(doc map[string]interface{}) float64 {
	if s.popularity == nil {
		return 1.0
	}

	category := ""
	if v, ok := doc["tema_geral"].(string); ok {
		category = v
	} else if v, ok := doc["category"].(string); ok {
		category = v
	}

	if category == "" {
		return 1.0
	}

	pop := float64(s.popularity.GetCategoryPopularity(category))
	if pop <= 0 {
		return 1.0
	}

	// Normaliza para 0-1 e aplica boost máximo de 10%
	normalized := pop / s.maxPopularity
	boost := 1.0 + (normalized * 0.1)

	return math.Min(1.1, boost)
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
