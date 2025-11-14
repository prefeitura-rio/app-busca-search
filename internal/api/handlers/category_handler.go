package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
)

// CategoryHandler gerencia endpoints de categorias
type CategoryHandler struct {
	categoryService *services.CategoryService
}

// NewCategoryHandler cria um novo handler de categorias
func NewCategoryHandler(categoryService *services.CategoryService) *CategoryHandler {
	return &CategoryHandler{
		categoryService: categoryService,
	}
}

// GetCategories godoc
// @Summary Lista categorias com contadores de serviços e scores de popularidade
// @Description Endpoint híbrido que retorna lista de categorias ordenadas por popularidade, quantidade de serviços ou ordem alfabética. Permite também filtrar serviços de uma categoria específica em uma única chamada. Scores de popularidade são baseados em dados hardcoded (futura integração com Google Analytics).
// @Description
// @Description **Casos de Uso:**
// @Description 1. Listar categorias: GET /api/v1/categories
// @Description 2. Ordenar por quantidade: GET /api/v1/categories?sort_by=count&order=desc
// @Description 3. Buscar serviços de categoria: GET /api/v1/categories?filter_category=Educação&page=1&per_page=10
// @Description
// @Description **Nota:** Quando filter_category é usado, o endpoint retorna tanto a lista de categorias quanto os serviços filtrados.
// @Tags categories
// @Accept json
// @Produce json
// @Param sort_by query string false "Critério de ordenação" Enums(popularity, count, alpha) default(popularity)
// @Param order query string false "Direção da ordenação" Enums(asc, desc) default(desc)
// @Param include_empty query bool false "Incluir categorias sem serviços publicados" default(false)
// @Param include_inactive query bool false "Incluir serviços inativos/rascunhos (status != 1) nas contagens e filtros" default(false)
// @Param filter_category query string false "Nome da categoria para filtrar serviços (ex: Educação, Saúde, Transporte)"
// @Param page query int false "Número da página para serviços filtrados (mínimo: 1)" minimum(1) default(1)
// @Param per_page query int false "Quantidade de serviços por página (máximo: 100)" minimum(1) maximum(100) default(10)
// @Success 200 {object} models.CategoryResponse "Lista de categorias com metadados. Se filter_category fornecido, inclui também os serviços filtrados"
// @Failure 400 {object} map[string]string "Parâmetros inválidos (sort_by, order, page ou per_page)"
// @Failure 500 {object} map[string]string "Erro interno ao buscar categorias ou serviços"
// @Router /api/v1/categories [get]
func (h *CategoryHandler) GetCategories(c *gin.Context) {
	// Parse query parameters
	req := &models.CategoryRequest{
		SortBy:          c.DefaultQuery("sort_by", "popularity"),
		Order:           c.DefaultQuery("order", "desc"),
		IncludeEmpty:    c.Query("include_empty") == "true",
		IncludeInactive: c.Query("include_inactive") == "true",
		FilterCategory:  c.Query("filter_category"),
		Page:            parseIntQuery(c, "page", 1),
		PerPage:         parseIntQuery(c, "per_page", 10),
	}

	// Validar sort_by
	validSortBy := map[string]bool{
		"popularity": true,
		"count":      true,
		"alpha":      true,
	}
	if !validSortBy[req.SortBy] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetro sort_by inválido",
			"details": "Valores válidos: popularity, count, alpha",
		})
		return
	}

	// Validar order
	if req.Order != "asc" && req.Order != "desc" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetro order inválido",
			"details": "Valores válidos: asc, desc",
		})
		return
	}

	// Validar page
	if req.Page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetro page inválido",
			"details": "Page deve ser maior ou igual a 1",
		})
		return
	}

	// Validar per_page
	if req.PerPage < 1 || req.PerPage > 100 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetro per_page inválido",
			"details": "PerPage deve estar entre 1 e 100",
		})
		return
	}

	// Executar busca de categorias
	result, err := h.categoryService.GetCategories(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Erro ao buscar categorias",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// parseIntQuery faz parse de query parameter inteiro com valor default
func parseIntQuery(c *gin.Context, param string, defaultValue int) int {
	valueStr := c.Query(param)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}
