package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
)

// SearchHandlerV2 gerencia endpoints de busca v2 (multi-collection)
type SearchHandlerV2 struct {
	searchService *services.SearchServiceV2
}

// NewSearchHandlerV2 cria um novo handler de busca v2
func NewSearchHandlerV2(searchService *services.SearchServiceV2) *SearchHandlerV2 {
	return &SearchHandlerV2{
		searchService: searchService,
	}
}

// Search godoc
// @Summary Busca unificada multi-coleção (v2)
// @Description Executa busca em múltiplas coleções configuradas (services, courses, jobs). Suporta keyword, semantic e hybrid search. Retorna documentos com estrutura unificada incluindo campo 'collection' e 'type'.
// @Tags search-v2
// @Accept json
// @Produce json
// @Param q query string true "Texto da busca"
// @Param type query string true "Tipo de busca: keyword, semantic, hybrid"
// @Param page query int false "Número da página (mínimo: 1)" default(1)
// @Param per_page query int false "Resultados por página (máximo: 100)" default(10)
// @Param include_inactive query bool false "Incluir documentos inativos (aplica-se apenas a coleções com filtro de status)" default(false)
// @Param alpha query number false "Alpha para busca hybrid (0-1). Alpha=0.3 significa 30% texto + 70% vetor." default(0.3)
// @Param threshold_keyword query number false "Score mínimo para busca keyword (0-1, filtra text_match normalizado)"
// @Param threshold_semantic query number false "Score mínimo para busca semantic (0-1, filtra por similaridade vetorial)"
// @Param threshold_hybrid query number false "Score mínimo para busca hybrid (0-1, filtra score híbrido)"
// @Param search_fields query string false "Override dos campos de busca (comma-separated). Ex: titulo,descricao,conteudo"
// @Param search_weights query string false "Override dos pesos de busca (comma-separated). Ex: 4,2,1"
// @Success 200 {object} models.UnifiedSearchResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v2/search [get]
func (h *SearchHandlerV2) Search(c *gin.Context) {
	var req models.SearchRequest

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetros inválidos",
			"details": err.Error(),
		})
		return
	}

	// Parse manual de threshold parameters (struct aninhado)
	if c.Query("threshold_keyword") != "" || c.Query("threshold_semantic") != "" || c.Query("threshold_hybrid") != "" {
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
	}

	// Validar search_fields e search_weights quando ambos são passados
	if req.SearchFields != "" && req.SearchWeights != "" {
		fieldsCount := len(strings.Split(req.SearchFields, ","))
		weightsCount := len(strings.Split(req.SearchWeights, ","))
		if fieldsCount != weightsCount {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Parâmetros inválidos",
				"details": fmt.Sprintf("search_fields tem %d campos mas search_weights tem %d pesos", fieldsCount, weightsCount),
			})
			return
		}
	}

	// Validar tipo de busca (v2 não suporta AI search ainda)
	validTypes := map[models.SearchType]bool{
		models.SearchTypeKeyword:  true,
		models.SearchTypeSemantic: true,
		models.SearchTypeHybrid:   true,
	}

	if !validTypes[req.Type] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Tipo de busca inválido",
			"details": "Tipos válidos para v2: keyword, semantic, hybrid (AI search não suportado ainda)",
		})
		return
	}

	result, err := h.searchService.Search(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Erro ao executar busca",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetDocumentByID godoc
// @Summary Busca documento por ID em qualquer coleção configurada (v2)
// @Description Retorna documento de qualquer coleção configurada. Se 'collection' fornecido como query param, tenta buscar nessa coleção primeiro. Caso contrário, busca em todas as coleções configuradas.
// @Tags search-v2
// @Accept json
// @Produce json
// @Param id path string true "ID do documento (UUID)" example(cffe0736-80a6-46fe-ace6-3cebb4d262ea)
// @Param collection query string false "Collection hint para busca otimizada" example(go-cursos)
// @Success 200 {object} models.UnifiedDocument
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v2/search/{id} [get]
func (h *SearchHandlerV2) GetDocumentByID(c *gin.Context) {
	id := c.Param("id")
	collectionHint := c.Query("collection")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "ID do documento é obrigatório",
		})
		return
	}

	// Busca com hint opcional
	doc, err := h.searchService.GetDocumentByID(c.Request.Context(), id, collectionHint)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Documento não encontrado em nenhuma coleção",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, doc)
}
