package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

type BuscaHandler struct {
	typesenseClient *typesense.Client
}

func NewBuscaHandler(client *typesense.Client) *BuscaHandler {
	return &BuscaHandler{
		typesenseClient: client,
	}
}

// BuscaMultiColecao godoc
// @Summary Busca híbrida em várias coleções
// @Description Realiza uma busca híbrida em várias coleções combinando texto e embeddings
// @Tags busca
// @Accept json
// @Produce json
// @Param collections query string true "Lista de coleções separadas por vírgula"
// @Param q query string true "Termo de busca"
// @Param page query int false "Página" default(1)
// @Param per_page query int false "Resultados por página" default(10)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/busca-hibrida-multi [get]
func (h *BuscaHandler) BuscaMultiColecao(c *gin.Context) {
	collectionsParam := c.Query("collections")
	if collectionsParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Lista de coleções é obrigatória"})
		return
	}

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Termo de busca é obrigatório"})
		return
	}

	pagina, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || pagina < 1 {
		pagina = 1
	}

	porPagina, err := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if err != nil || porPagina < 1 || porPagina > 100 {
		porPagina = 10
	}

	// Parsing das coleções - dividindo a string por vírgulas
	colecoes := strings.Split(collectionsParam, ",")
	
	// Realiza a busca em múltiplas coleções
	ctx := context.Background()
	resultado, err := h.typesenseClient.BuscaMultiColecaoComTexto(ctx, colecoes, query, pagina, porPagina)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao realizar busca: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resultado)
}

// BuscaPorCategoria godoc
// @Summary Busca documentos por categoria
// @Description Busca documentos de uma categoria específica retornando apenas título e ID
// @Tags busca
// @Accept json
// @Produce json
// @Param collection path string true "Nome da coleção"
// @Param categoria query string true "Categoria dos documentos"
// @Param page query int false "Página" default(1)
// @Param per_page query int false "Resultados por página" default(10)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/categoria/{collection} [get]
func (h *BuscaHandler) BuscaPorCategoria(c *gin.Context) {
	colecao := c.Param("collection")
	if colecao == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nome da coleção é obrigatório"})
		return
	}

	categoria := c.Query("categoria")
	if categoria == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Categoria é obrigatória"})
		return
	}

	pagina, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || pagina < 1 {
		pagina = 1
	}

	porPagina, err := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if err != nil || porPagina < 1 || porPagina > 100 {
		porPagina = 10
	}

	resultado, err := h.typesenseClient.BuscaPorCategoria(colecao, categoria, pagina, porPagina)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao buscar por categoria: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resultado)
}

// BuscaPorID godoc
// @Summary Busca documento por ID
// @Description Busca um documento específico por ID retornando todos os campos exceto embedding e campos normalizados
// @Tags busca
// @Accept json
// @Produce json
// @Param collection path string true "Nome da coleção"
// @Param id path string true "ID do documento"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/documento/{collection}/{id} [get]
func (h *BuscaHandler) BuscaPorID(c *gin.Context) {
	colecao := c.Param("collection")
	if colecao == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nome da coleção é obrigatório"})
		return
	}

	documentoID := c.Param("id")
	if documentoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do documento é obrigatório"})
		return
	}

	resultado, err := h.typesenseClient.BuscaPorID(colecao, documentoID)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Documento não encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao buscar documento: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resultado)
}