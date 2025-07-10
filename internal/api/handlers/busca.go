package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/constants"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
	"github.com/prefeitura-rio/app-busca-search/internal/utils"
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
// @Summary Busca hibrida em varias colecoes
// @Description Realiza uma busca hibrida em varias colecoes combinando texto e embeddings
// @Tags busca
// @Accept json
// @Produce json
// @Param collections query string true "Lista de colecoes separadas por virgula"
// @Param q query string true "Termo de busca"
// @Param page query int false "Pagina" default(1)
// @Param per_page query int false "Resultados por pagina" default(10)
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
// @Description Busca documentos de uma categoria especifica em uma ou multiplas colecoes retornando informacoes completas
// @Tags busca
// @Accept json
// @Produce json
// @Param collections path string true "Nome da colecao ou lista de colecoes separadas por virgula"
// @Param categoria query string true "Categoria dos documentos"
// @Param page query int false "Pagina" default(1)
// @Param per_page query int false "Resultados por pagina" default(10)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/categoria/{collections} [get]
func (h *BuscaHandler) BuscaPorCategoria(c *gin.Context) {
	collectionsParam := c.Param("collections")
	if collectionsParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nome da(s) coleção(ões) é obrigatório"})
		return
	}

	categoriaParam := c.Query("categoria")
	if categoriaParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Categoria é obrigatória"})
		return
	}

	// Normaliza o parâmetro categoria e encontra a categoria original correspondente
	categoriaNormalizada := utils.NormalizarCategoria(categoriaParam)
	categoria := utils.DesnormalizarCategoria(categoriaNormalizada, constants.CategoriasValidas)

	pagina, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || pagina < 1 {
		pagina = 1
	}

	porPagina, err := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if err != nil || porPagina < 1 || porPagina > 100 {
		porPagina = 10
	}

	// Verifica se são múltiplas coleções ou uma única
	colecoes := strings.Split(collectionsParam, ",")
	
	var resultado map[string]interface{}
	if len(colecoes) > 1 {
		// Múltiplas coleções - usa o método multi-coleção
		resultado, err = h.typesenseClient.BuscaPorCategoriaMultiColecao(colecoes, categoria, pagina, porPagina)
	} else {
		// Uma única coleção - usa o método original (mas agora retorna informações completas)
		resultado, err = h.typesenseClient.BuscaPorCategoria(colecoes[0], categoria, pagina, porPagina)
	}
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao buscar por categoria: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resultado)
}

// BuscaPorID godoc
// @Summary Busca documento por ID
// @Description Busca um documento especifico por ID retornando todos os campos exceto embedding e campos normalizados
// @Tags busca
// @Accept json
// @Produce json
// @Param collection path string true "Nome da colecao"
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

// CategoriasRelevancia godoc
// @Summary Busca categorias ordenadas por relevancia
// @Description Retorna todas as categorias ordenadas por relevancia baseada na volumetria dos servicos
// @Tags busca
// @Accept json
// @Produce json
// @Param collections query string true "Lista de colecoes separadas por virgula"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/categorias-relevancia [get]
func (h *BuscaHandler) CategoriasRelevancia(c *gin.Context) {
	collectionsParam := c.Query("collections")
	if collectionsParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Lista de coleções é obrigatória"})
		return
	}

	// Parsing das coleções - dividindo a string por vírgulas
	colecoes := strings.Split(collectionsParam, ",")
	
	// Busca categorias com relevância
	resultado, err := h.typesenseClient.BuscarCategoriasRelevancia(colecoes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao buscar categorias por relevância: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resultado)
}