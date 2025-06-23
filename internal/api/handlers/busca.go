package handlers

import (
	"context"
	"encoding/json"
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

type BuscaHibridaRequest struct {
	Texto     string    `json:"texto" binding:"required"`
	Embedding []float32 `json:"embedding" binding:"required"`
}

type BuscaHibridaMultiRequest struct {
	Texto     string    `json:"texto" binding:"required"`
	Embedding []float32 `json:"embedding" binding:"required"`
	Colecoes  []string  `json:"colecoes" binding:"required"`
}

func NewBuscaHandler(client *typesense.Client) *BuscaHandler {
	return &BuscaHandler{
		typesenseClient: client,
	}
}

// Busca godoc
// @Summary      Busca documentos
// @Description  Realiza uma busca híbrida (texto + vetor) na base do Typesense
// @Tags         busca
// @Accept       json
// @Produce      json
// @Param        colecao    path     string  true  "Nome da coleção"
// @Param        q          query    string  true  "Termo de busca"
// @Param        pagina     query    int     false "Número da página atual"
// @Param        por_pagina query    int     false "Itens por página"
// @Param        embedding  query    string  false "Vetor de embedding (no formato JSON: [0.1,0.2,...]))"
// @Success      200        {object} map[string]interface{}
// @Router       /busca/{colecao} [get]
func (h *BuscaHandler) Busca(c *gin.Context) {
	colecao := c.Param("colecao")
	query := c.Query("q")
	
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"erro": "O parâmetro de busca 'q' é obrigatório"})
		return
	}

	pagina, _ := strconv.Atoi(c.DefaultQuery("pagina", "1"))
	porPagina, _ := strconv.Atoi(c.DefaultQuery("por_pagina", "10"))

	var vetor []float32
	embeddingStr := c.Query("embedding")
	if embeddingStr != "" {
		if err := json.Unmarshal([]byte(embeddingStr), &vetor); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"erro": "Formato de vetor de embedding inválido. Deve ser um array JSON: [0.1,0.2,...]"})
			return
		}
	}

	resultado, err := h.typesenseClient.Busca(colecao, query, pagina, porPagina, vetor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"erro": "Erro ao realizar a busca: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, resultado)
}

// BuscaVetorial godoc
// @Summary      Busca vetorial
// @Description  Realiza uma busca puramente vetorial (por similaridade) na base do Typesense
// @Tags         busca
// @Accept       json
// @Produce      json
// @Param        colecao    path     string  true  "Nome da coleção"
// @Param        pagina     query    int     false "Número da página atual"
// @Param        por_pagina query    int     false "Itens por página"
// @Param        embedding  query    string  true  "Vetor de embedding (no formato JSON: [0.1,0.2,...]))"
// @Success      200        {object} map[string]interface{}
// @Router       /busca/{colecao}/vetorial [get]
func (h *BuscaHandler) BuscaVetorial(c *gin.Context) {
	colecao := c.Param("colecao")
	pagina, _ := strconv.Atoi(c.DefaultQuery("pagina", "1"))
	porPagina, _ := strconv.Atoi(c.DefaultQuery("por_pagina", "10"))

	embeddingStr := c.Query("embedding")
	if embeddingStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"erro": "O parâmetro 'embedding' é obrigatório para busca vetorial"})
		return
	}

	var vetor []float32
	if err := json.Unmarshal([]byte(embeddingStr), &vetor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"erro": "Formato de vetor de embedding inválido. Deve ser um array JSON: [0.1,0.2,...]"})
		return
	}

	if len(vetor) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"erro": "Vetor de embedding não pode estar vazio"})
		return
	}

	resultado, err := h.typesenseClient.BuscaVetorial(colecao, vetor, pagina, porPagina)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"erro": "Erro ao realizar a busca vetorial: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, resultado)
}

// BuscaHibrida godoc
// @Summary Busca híbrida no Typesense
// @Description Realiza uma busca híbrida combinando texto e embeddings no Typesense
// @Tags busca
// @Accept json
// @Produce json
// @Param collection path string true "Nome da coleção"
// @Param q query string true "Termo de busca"
// @Param page query int false "Página" default(1)
// @Param per_page query int false "Resultados por página" default(10)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/busca/{collection} [get]
func (h *BuscaHandler) BuscaHibrida(c *gin.Context) {
	colecao := c.Param("collection")
	if colecao == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nome da coleção é obrigatório"})
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

	// Realiza a busca
	ctx := context.Background()
	resultado, err := h.typesenseClient.BuscaComTexto(ctx, colecao, query, pagina, porPagina)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao realizar busca: %v", err)})
		return
	}

	c.JSON(http.StatusOK, resultado)
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
// @Router /api/v1/busca-auto-multi [get]
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

// BuscaMultiColecaoConteudos godoc
// @Summary Busca conteúdos em várias coleções (formato simplificado)
// @Description Realiza uma busca híbrida em várias coleções e retorna apenas os campos resumo_web (text) e url (cta_url)
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
// @Router /api/v1/busca-app [get]
func (h *BuscaHandler) BuscaMultiColecaoConteudos(c *gin.Context) {
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

	// Parâmetros opcionais
	pagina, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || pagina < 1 {
		pagina = 1
	}

	porPagina, err := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if err != nil || porPagina < 1 || porPagina > 100 {
		porPagina = 10
	}

	colecoes := strings.Split(collectionsParam, ",")

	ctx := context.Background()
	resultado, err := h.typesenseClient.BuscaMultiColecaoComTexto(ctx, colecoes, query, pagina, porPagina)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao realizar busca: %v", err)})
		return
	}

	contents := make([]map[string]interface{}, 0)

	switch hitsVal := resultado["hits"].(type) {
	case []interface{}:
		for _, hRaw := range hitsVal {
			hMap, ok := hRaw.(map[string]interface{})
			if !ok {
				continue
			}
			processHitForContent(&contents, hMap)
		}
	case []map[string]interface{}:
		for _, hMap := range hitsVal {
			processHitForContent(&contents, hMap)
		}
	}

	c.JSON(http.StatusOK, gin.H{"contents": contents})
}

func processHitForContent(contents *[]map[string]interface{}, hMap map[string]interface{}) {
	doc, ok := hMap["document"].(map[string]interface{})
	if !ok {
		return
	}

	resumo, _ := doc["resumo_web"].(string)
	urlStr, _ := doc["url"].(string)

	if resumo != "" && urlStr != "" {
		*contents = append(*contents, map[string]interface{}{
			"text":    resumo,
			"cta_url": urlStr,
		})
	}
}