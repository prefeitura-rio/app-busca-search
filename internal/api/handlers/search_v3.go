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
// @Description Busca multi-collection com suporte a keyword, semantic, hybrid e AI. Inclui sinônimos, query expansion, modos human/agent, cache e AI analysis.
// @Tags search-v3
// @Accept json
// @Produce json
// @Param q query string true "Query de busca"
// @Param type query string true "Tipo: keyword, semantic, hybrid, ai"
// @Param page query int false "Página (default: 1)"
// @Param per_page query int false "Resultados por página (default: 10, max: 100)"
// @Param collections query string false "Collections (comma-separated)"
// @Param mode query string false "Modo: human ou agent (default: human). Agent retorna resposta compacta."
// @Param alpha query number false "Peso texto vs vetor (0-1, default: 0.3)"
// @Param threshold query number false "Score mínimo (0-1)"
// @Param expand query bool false "Expandir query com sinônimos"
// @Param recency query bool false "Boost por recência"
// @Param typos query int false "Tolerância a typos (0-2)"
// @Param status query int false "Filtrar por status"
// @Param category query string false "Filtrar por categoria"
// @Param sub_category query string false "Filtrar por subcategoria"
// @Param orgao query string false "Filtrar por órgão gestor"
// @Param tempo_max query string false "Filtrar por tempo máximo (imediato, 1_dia, etc)"
// @Param is_free query bool false "Filtrar por gratuidade"
// @Param digital query bool false "Filtrar por canal digital disponível"
// @Param fields query string false "Campos a retornar (comma-separated): title,description,category,score,buttons,data"
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

	// Parse fields
	req.ParseFields()

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

	// Retorna resposta compacta para modo agent
	if req.Mode == v3.SearchModeAgent {
		c.JSON(http.StatusOK, result.ToAgentResponse())
		return
	}

	// Retorna campos filtrados se especificado
	if len(req.ParsedFields) > 0 {
		c.JSON(http.StatusOK, result.ToFilteredResponse(req.ParsedFields))
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID e obrigatorio"})
		return
	}

	// Collection hint opcional para otimizar a busca
	collection := c.Query("collection")

	doc, err := h.engine.GetDocument(c.Request.Context(), id, collection)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Documento nao encontrado"})
		return
	}

	c.JSON(http.StatusOK, doc)
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
