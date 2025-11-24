package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	middlewares "github.com/prefeitura-rio/app-busca-search/internal/middleware"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

type TombamentoHandler struct {
	typesenseClient *typesense.Client
	validator       *validator.Validate
}

func NewTombamentoHandler(client *typesense.Client) *TombamentoHandler {
	return &TombamentoHandler{
		typesenseClient: client,
		validator:       validator.New(),
	}
}

// CreateTombamento godoc
// @Summary Cria um novo tombamento
// @Description Cria um mapeamento de serviço antigo para serviço novo na collection tombamentos_overlay
// @Tags tombamentos
// @Accept json
// @Produce json
// @Param tombamento body models.TombamentoRequest true "Dados do tombamento"
// @Success 201 {object} models.Tombamento
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/tombamentos [post]
func (h *TombamentoHandler) CreateTombamento(c *gin.Context) {
	var request models.TombamentoRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	// Valida os dados
	if err := h.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validação falhou: " + err.Error()})
		return
	}

	ctx := context.Background()

	// Verifica se o serviço novo existe na prefrio_services_base
	_, err := h.typesenseClient.GetPrefRioService(ctx, request.IDServicoNovo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Serviço novo não encontrado na collection prefrio_services_base"})
		return
	}

	// Verifica se já existe um tombamento para este serviço antigo
	existingTombamento, _ := h.typesenseClient.GetTombamentoByOldServiceID(ctx, request.Origem, request.IDServicoAntigo)
	if existingTombamento != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":                "Já existe um tombamento para este serviço antigo",
			"tombamento_existente": existingTombamento,
		})
		return
	}

	// Converte para modelo completo
	tombamento := &models.Tombamento{
		Origem:          request.Origem,
		IDServicoAntigo: request.IDServicoAntigo,
		IDServicoNovo:   request.IDServicoNovo,
		CriadoPor:       middlewares.GetUserName(c),
		Observacoes:     request.Observacoes,
	}

	// Cria o tombamento
	createdTombamento, err := h.typesenseClient.CreateTombamento(ctx, tombamento)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar tombamento: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, createdTombamento)
}

// GetTombamento godoc
// @Summary Busca um tombamento por ID
// @Description Busca um tombamento específico por ID
// @Tags tombamentos
// @Accept json
// @Produce json
// @Param id path string true "ID do tombamento"
// @Success 200 {object} models.Tombamento
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/tombamentos/{id} [get]
func (h *TombamentoHandler) GetTombamento(c *gin.Context) {
	tombamentoID := c.Param("id")
	if tombamentoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do tombamento é obrigatório"})
		return
	}

	// Busca o tombamento
	ctx := context.Background()
	tombamento, err := h.typesenseClient.GetTombamento(ctx, tombamentoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tombamento não encontrado"})
		return
	}

	c.JSON(http.StatusOK, tombamento)
}

// ListTombamentos godoc
// @Summary Lista tombamentos com paginação e filtros
// @Description Lista tombamentos com paginação e filtros opcionais
// @Tags tombamentos
// @Accept json
// @Produce json
// @Param page query int false "Página" default(1)
// @Param per_page query int false "Resultados por página" default(10)
// @Param origem query string false "Filtrar por origem (1746_v2_llm ou carioca-digital_v2_llm)"
// @Param criado_por query string false "Filtrar por criador"
// @Success 200 {object} models.TombamentoResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/tombamentos [get]
func (h *TombamentoHandler) ListTombamentos(c *gin.Context) {
	// Parse de parâmetros de paginação
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	perPage, err := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if err != nil || perPage < 1 || perPage > 100 {
		perPage = 10
	}

	// Parse de filtros
	filters := make(map[string]any)

	if origem := c.Query("origem"); origem != "" {
		// Valida origem
		if origem == "1746_v2_llm" || origem == "carioca-digital_v2_llm" {
			filters["origem"] = origem
		}
	}

	if criadoPor := c.Query("criado_por"); criadoPor != "" {
		filters["criado_por"] = criadoPor
	}

	// Lista os tombamentos
	ctx := context.Background()
	response, err := h.typesenseClient.ListTombamentos(ctx, page, perPage, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao listar tombamentos: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// UpdateTombamento godoc
// @Summary Atualiza um tombamento existente
// @Description Atualiza um tombamento existente
// @Tags tombamentos
// @Accept json
// @Produce json
// @Param id path string true "ID do tombamento"
// @Param tombamento body models.TombamentoRequest true "Dados atualizados do tombamento"
// @Success 200 {object} models.Tombamento
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/tombamentos/{id} [put]
func (h *TombamentoHandler) UpdateTombamento(c *gin.Context) {
	tombamentoID := c.Param("id")
	if tombamentoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do tombamento é obrigatório"})
		return
	}

	var request models.TombamentoRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	// Valida os dados
	if err := h.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validação falhou: " + err.Error()})
		return
	}

	ctx := context.Background()

	// Busca o tombamento existente para preservar dados
	existingTombamento, err := h.typesenseClient.GetTombamento(ctx, tombamentoID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tombamento não encontrado"})
		return
	}

	// Verifica se o serviço novo existe na prefrio_services_base
	_, err = h.typesenseClient.GetPrefRioService(ctx, request.IDServicoNovo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Serviço novo não encontrado na collection prefrio_services_base"})
		return
	}

	// Atualiza dados mantendo informações originais
	tombamento := &models.Tombamento{
		ID:              tombamentoID,
		Origem:          request.Origem,
		IDServicoAntigo: request.IDServicoAntigo,
		IDServicoNovo:   request.IDServicoNovo,
		CriadoEm:        existingTombamento.CriadoEm,
		CriadoPor:       existingTombamento.CriadoPor,
		Observacoes:     request.Observacoes,
	}

	// Atualiza o tombamento
	updatedTombamento, err := h.typesenseClient.UpdateTombamento(ctx, tombamentoID, tombamento)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar tombamento: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedTombamento)
}

// DeleteTombamento godoc
// @Summary Deleta um tombamento (reverte substituição)
// @Description Deleta um tombamento, fazendo com que o serviço antigo volte a aparecer normalmente
// @Tags tombamentos
// @Accept json
// @Produce json
// @Param id path string true "ID do tombamento"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/tombamentos/{id} [delete]
func (h *TombamentoHandler) DeleteTombamento(c *gin.Context) {
	tombamentoID := c.Param("id")
	if tombamentoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do tombamento é obrigatório"})
		return
	}

	// Deleta o tombamento
	ctx := context.Background()
	err := h.typesenseClient.DeleteTombamento(ctx, tombamentoID)
	if err != nil {
		if err.Error() == "tombamento não encontrado" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Tombamento não encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao deletar tombamento: " + err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetTombamentoByOldService godoc
// @Summary Busca tombamento por serviço antigo
// @Description Busca um tombamento pelo ID do serviço antigo e origem
// @Tags tombamentos
// @Accept json
// @Produce json
// @Param origem query string true "Origem (1746_v2_llm ou carioca-digital_v2_llm)"
// @Param id_servico_antigo query string true "ID do serviço antigo"
// @Success 200 {object} models.Tombamento
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/tombamentos/by-old-service [get]
func (h *TombamentoHandler) GetTombamentoByOldService(c *gin.Context) {
	origem := c.Query("origem")
	idServicoAntigo := c.Query("id_servico_antigo")

	if origem == "" || idServicoAntigo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Parâmetros 'origem' e 'id_servico_antigo' são obrigatórios"})
		return
	}

	// Valida origem
	if origem != "1746_v2_llm" && origem != "carioca-digital_v2_llm" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Origem deve ser '1746_v2_llm' ou 'carioca-digital_v2_llm'"})
		return
	}

	// Busca o tombamento
	ctx := context.Background()
	tombamento, err := h.typesenseClient.GetTombamentoByOldServiceID(ctx, origem, idServicoAntigo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tombamento não encontrado"})
		return
	}

	c.JSON(http.StatusOK, tombamento)
}
