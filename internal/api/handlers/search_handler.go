package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

// SearchHandler gerencia endpoints de busca
type SearchHandler struct {
	searchService   *services.SearchService
	typesenseClient *typesense.Client
}

// NewSearchHandler cria um novo handler de busca
func NewSearchHandler(searchService *services.SearchService, typesenseClient *typesense.Client) *SearchHandler {
	return &SearchHandler{
		searchService:   searchService,
		typesenseClient: typesenseClient,
	}
}

// Search godoc
// @Summary Busca unificada de serviços públicos
// @Description Executa busca com 4 estratégias: keyword (textual), semantic (vetorial), hybrid (combinada) ou ai (agente inteligente). Resposta inclui total_count (total do Typesense) e filtered_count (após aplicar thresholds).
// @Tags search
// @Accept json
// @Produce json
// @Param q query string true "Texto da busca"
// @Param type query string true "Tipo de busca: keyword, semantic, hybrid ou ai"
// @Param page query int false "Número da página (mínimo: 1)" default(1)
// @Param per_page query int false "Resultados por página (máximo: 100)" default(10)
// @Param include_inactive query bool false "Incluir serviços inativos (status != 1)" default(false)
// @Param alpha query number false "Alpha para busca hybrid (0-1). Alpha=0.3 significa 30% texto + 70% vetor." default(0.3)
// @Param threshold_keyword query number false "Score mínimo para busca keyword (0-1, filtra text_match normalizado via log normalization)"
// @Param threshold_semantic query number false "Score mínimo para busca semantic (0-1, filtra por similaridade vetorial)"
// @Param threshold_hybrid query number false "Score mínimo para busca hybrid (0-1, filtra score híbrido combinado)"
// @Param threshold_ai query number false "Score mínimo para busca AI com generate_scores=true (0-1, filtra por ai_score.final_score)"
// @Param exclude_agent_exclusive query bool false "Se true, exclui serviços exclusivos para agentes IA (mostra apenas serviços para humanos)" default(false)
// @Param generate_scores query bool false "Gera scores detalhados via LLM para os resultados (apenas type=ai). ATENÇÃO: Consome créditos da API Gemini (1 chamada por resultado, max 20)." default(false)
// @Success 200 {object} models.SearchResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/search [get]
func (h *SearchHandler) Search(c *gin.Context) {
	var req models.SearchRequest

	// Bind e validação
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetros inválidos",
			"details": err.Error(),
		})
		return
	}

	// Parse manual de threshold parameters (struct aninhado)
	if c.Query("threshold_keyword") != "" || c.Query("threshold_semantic") != "" || c.Query("threshold_hybrid") != "" || c.Query("threshold_ai") != "" {
		req.ScoreThreshold = &models.ScoreThreshold{}

		if val := c.Query("threshold_keyword"); val != "" {
			var f float64
			if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
				req.ScoreThreshold.Keyword = &f
			}
		}

		if val := c.Query("threshold_semantic"); val != "" {
			var f float64
			if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
				req.ScoreThreshold.Semantic = &f
			}
		}

		if val := c.Query("threshold_hybrid"); val != "" {
			var f float64
			if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
				req.ScoreThreshold.Hybrid = &f
			}
		}

		if val := c.Query("threshold_ai"); val != "" {
			var f float64
			if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
				req.ScoreThreshold.AI = &f
			}
		}
	}

	// Validar tipo de busca
	validTypes := map[models.SearchType]bool{
		models.SearchTypeKeyword:  true,
		models.SearchTypeSemantic: true,
		models.SearchTypeHybrid:   true,
		models.SearchTypeAI:       true,
	}

	if !validTypes[req.Type] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Tipo de busca inválido",
			"details": "Tipos válidos: keyword, semantic, hybrid, ai",
		})
		return
	}

	// Executar busca
	result, err := h.searchService.Search(c.Request.Context(), &req)
	if err != nil {
		if err == services.ErrSearchCanceled {
			c.JSON(http.StatusRequestTimeout, gin.H{
				"error": "Busca cancelada ou timeout",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Erro ao executar busca",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetDocumentByID godoc
// @Summary Busca um serviço por ID (UUID)
// @Description Retorna os detalhes completos de um serviço específico através de busca direta por UUID no Typesense
// @Tags search
// @Accept json
// @Produce json
// @Param id path string true "UUID do serviço" example(cffe0736-80a6-46fe-ace6-3cebb4d262ea)
// @Success 200 {object} models.PrefRioService
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/search/{id} [get]
func (h *SearchHandler) GetDocumentByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "ID do serviço é obrigatório",
		})
		return
	}

	// Busca direta por ID no Typesense (retrieval por chave primária)
	doc, err := h.typesenseClient.GetPrefRioService(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Serviço não encontrado",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, doc)
}
