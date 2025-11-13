package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
)

// SearchHandler gerencia endpoints de busca
type SearchHandler struct {
	searchService *services.SearchService
}

// NewSearchHandler cria um novo handler de busca
func NewSearchHandler(searchService *services.SearchService) *SearchHandler {
	return &SearchHandler{
		searchService: searchService,
	}
}

// Search godoc
// @Summary Busca unificada de serviços públicos
// @Description Executa busca com 4 estratégias: keyword (textual), semantic (vetorial), hybrid (combinada) ou ai (agente inteligente)
// @Tags search
// @Accept json
// @Produce json
// @Param q query string true "Texto da busca"
// @Param type query string true "Tipo de busca: keyword, semantic, hybrid ou ai"
// @Param page query int false "Número da página (mínimo: 1)" default(1)
// @Param per_page query int false "Resultados por página (máximo: 100)" default(10)
// @Param include_inactive query bool false "Incluir serviços inativos (status != 1)" default(false)
// @Param alpha query number false "Alpha para busca hybrid (0-1, default: 0.3)" default(0.3)
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
// @Summary Busca um serviço por ID
// @Description Retorna os detalhes completos de um serviço específico
// @Tags search
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 200 {object} models.ServiceDocument
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

	// Buscar por ID usando keyword search (mais rápido que Retrieve para este caso)
	req := &models.SearchRequest{
		Query:           id,
		Type:            models.SearchTypeKeyword,
		Page:            1,
		PerPage:         1,
		IncludeInactive: true, // Permitir buscar inativos por ID
	}

	result, err := h.searchService.Search(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Erro ao buscar serviço",
			"details": err.Error(),
		})
		return
	}

	if len(result.Results) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Serviço não encontrado",
		})
		return
	}

	c.JSON(http.StatusOK, result.Results[0])
}
