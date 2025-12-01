package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
)

// SubcategoryHandler gerencia endpoints de subcategorias
type SubcategoryHandler struct {
	subcategoryService *services.SubcategoryService
}

// NewSubcategoryHandler cria um novo handler de subcategorias
func NewSubcategoryHandler(subcategoryService *services.SubcategoryService) *SubcategoryHandler {
	return &SubcategoryHandler{
		subcategoryService: subcategoryService,
	}
}

// GetSubcategories godoc
// @Summary Lista subcategorias de uma categoria específica
// @Description Retorna lista de subcategorias dentro de uma categoria pai, ordenadas por popularidade, quantidade de serviços ou ordem alfabética. Subcategorias são extraídas dinamicamente dos serviços via facet search no Typesense.
// @Description
// @Description **Casos de Uso:**
// @Description 1. Listar subcategorias: GET /api/v1/categories/Educação/subcategories
// @Description 2. Ordenar por quantidade: GET /api/v1/categories/Educação/subcategories?sort_by=count&order=desc
// @Description 3. Incluir vazias: GET /api/v1/categories/Educação/subcategories?include_empty=true
// @Tags subcategories
// @Accept json
// @Produce json
// @Param category path string true "Nome da categoria pai (ex: Educação, Saúde, Transporte)"
// @Param sort_by query string false "Critério de ordenação" Enums(popularity, count, alpha) default(popularity)
// @Param order query string false "Direção da ordenação" Enums(asc, desc) default(desc)
// @Param include_empty query bool false "Incluir subcategorias sem serviços publicados" default(false)
// @Param include_inactive query bool false "Incluir serviços inativos/rascunhos (status != 1) nas contagens" default(false)
// @Success 200 {object} models.SubcategoryResponse "Lista de subcategorias com metadados"
// @Failure 400 {object} map[string]string "Parâmetros inválidos (sort_by ou order)"
// @Failure 500 {object} map[string]string "Erro interno ao buscar subcategorias"
// @Router /api/v1/categories/{category}/subcategories [get]
func (h *SubcategoryHandler) GetSubcategories(c *gin.Context) {
	// Parse path parameter
	category := c.Param("category")
	if category == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetro category obrigatório",
			"details": "Categoria pai deve ser fornecida no path",
		})
		return
	}

	// Parse query parameters
	req := &models.SubcategoryRequest{
		Category:        category,
		SortBy:          c.DefaultQuery("sort_by", "popularity"),
		Order:           c.DefaultQuery("order", "desc"),
		IncludeEmpty:    c.Query("include_empty") == "true",
		IncludeInactive: c.Query("include_inactive") == "true",
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

	// Executar busca de subcategorias
	result, err := h.subcategoryService.GetSubcategories(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Erro ao buscar subcategorias",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetServicesBySubcategory godoc
// @Summary Lista serviços de uma subcategoria específica
// @Description Retorna serviços filtrados por subcategoria, com paginação. Serviços são ordenados por última atualização (mais recentes primeiro).
// @Description
// @Description **Casos de Uso:**
// @Description 1. Listar serviços: GET /api/v1/subcategories/Ensino%20Fundamental/services
// @Description 2. Paginar resultados: GET /api/v1/subcategories/Ensino%20Fundamental/services?page=2&per_page=20
// @Description 3. Incluir inativos: GET /api/v1/subcategories/Ensino%20Fundamental/services?include_inactive=true
// @Tags subcategories
// @Accept json
// @Produce json
// @Param subcategory path string true "Nome da subcategoria (ex: Ensino Fundamental, Vacinação)"
// @Param page query int false "Número da página (mínimo: 1)" minimum(1) default(1)
// @Param per_page query int false "Quantidade de serviços por página (máximo: 100)" minimum(1) maximum(100) default(10)
// @Param include_inactive query bool false "Incluir serviços inativos/rascunhos (status != 1)" default(false)
// @Success 200 {object} models.SubcategoryServicesResponse "Lista de serviços da subcategoria com metadados"
// @Failure 400 {object} map[string]string "Parâmetros inválidos (page ou per_page)"
// @Failure 500 {object} map[string]string "Erro interno ao buscar serviços"
// @Router /api/v1/subcategories/{subcategory}/services [get]
func (h *SubcategoryHandler) GetServicesBySubcategory(c *gin.Context) {
	// Parse path parameter
	subcategory := c.Param("subcategory")
	if subcategory == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetro subcategory obrigatório",
			"details": "Subcategoria deve ser fornecida no path",
		})
		return
	}

	// Parse query parameters
	req := &models.SubcategoryServicesRequest{
		Subcategory:     subcategory,
		Page:            parseIntQuery(c, "page", 1),
		PerPage:         parseIntQuery(c, "per_page", 10),
		IncludeInactive: c.Query("include_inactive") == "true",
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

	// Executar busca de serviços
	result, err := h.subcategoryService.GetServicesBySubcategory(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Erro ao buscar serviços da subcategoria",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}
