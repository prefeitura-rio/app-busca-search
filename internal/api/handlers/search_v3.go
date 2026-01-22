package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search"
)

// SearchHandlerV3 gerencia endpoints de busca v3
type SearchHandlerV3 struct {
	engine *search.Engine
}

// NewSearchHandlerV3 cria um novo handler de busca v3
func NewSearchHandlerV3(engine *search.Engine) *SearchHandlerV3 {
	return &SearchHandlerV3{
		engine: engine,
	}
}

// Search godoc
// @Summary Busca unificada v3
// @Description Busca multi-collection com suporte a keyword, semantic, hybrid e AI. Inclui sinônimos, query expansion e modos human/agent.
// @Tags search-v3
// @Accept json
// @Produce json
// @Param q query string true "Query de busca"
// @Param type query string true "Tipo: keyword, semantic, hybrid, ai"
// @Param page query int false "Página (default: 1)"
// @Param per_page query int false "Resultados por página (default: 10, max: 100)"
// @Param collections query string false "Collections (comma-separated)"
// @Param mode query string false "Modo: human ou agent (default: human)"
// @Param alpha query number false "Peso texto vs vetor (0-1, default: 0.3)"
// @Param threshold query number false "Score mínimo (0-1)"
// @Param expand query bool false "Expandir query com sinônimos"
// @Param recency query bool false "Boost por recência"
// @Param typos query int false "Tolerância a typos (0-2)"
// @Param status query int false "Filtrar por status"
// @Param category query string false "Filtrar por categoria"
// @Success 200 {object} v3.SearchResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v3/search [get]
func (h *SearchHandlerV3) Search(c *gin.Context) {
	var req v3.SearchRequest

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetros inválidos",
			"details": err.Error(),
		})
		return
	}

	// Parse collections
	if req.Collections != "" {
		req.ParsedCollections = parseCollections(req.Collections)
	}

	// Executa busca
	result, err := h.engine.Search(c.Request.Context(), &req)
	if err != nil {
		status := http.StatusInternalServerError
		if err == v3.ErrQueryRequired || err == v3.ErrInvalidSearchType {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetDocument godoc
// @Summary Busca documento por ID
// @Description Retorna um documento específico pelo ID
// @Tags search-v3
// @Accept json
// @Produce json
// @Param id path string true "ID do documento"
// @Param collection query string false "Collection do documento"
// @Success 200 {object} v3.Document
// @Failure 404 {object} map[string]string
// @Router /api/v3/search/{id} [get]
func (h *SearchHandlerV3) GetDocument(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID é obrigatório"})
		return
	}

	// Por enquanto, retorna não implementado
	// TODO: implementar busca por ID
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Endpoint em desenvolvimento"})
}

func parseCollections(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
