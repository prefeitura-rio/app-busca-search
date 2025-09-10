package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/prefeitura-rio/app-busca-search/internal/middleware"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

type AdminHandler struct {
	typesenseClient *typesense.Client
	validator       *validator.Validate
}

func NewAdminHandler(client *typesense.Client) *AdminHandler {
	return &AdminHandler{
		typesenseClient: client,
		validator:       validator.New(),
	}
}

// CreateService godoc
// @Summary Cria um novo serviço
// @Description Cria um novo serviço na collection prefrio_services_base. Apenas GERAL e ADMIN podem criar.
// @Tags admin
// @Accept json
// @Produce json
// @Param service body models.PrefRioServiceRequest true "Dados do serviço"
// @Success 201 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services [post]
func (h *AdminHandler) CreateService(c *gin.Context) {
	var request models.PrefRioServiceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	// Valida os dados
	if err := h.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validação falhou: " + err.Error()})
		return
	}

	// Converte para modelo completo
	service := &models.PrefRioService{
		NomeServico:               request.NomeServico,
		OrgaoGestor:               request.OrgaoGestor,
		Resumo:                    request.Resumo,
		TempoAtendimento:          request.TempoAtendimento,
		CustoServico:              request.CustoServico,
		ResultadoSolicitacao:      request.ResultadoSolicitacao,
		DescricaoCompleta:         request.DescricaoCompleta,
		Autor:                     middlewares.GetUserName(c), // Preenchimento automático
		DocumentosNecessarios:     request.DocumentosNecessarios,
		InstrucoesSolicitante:     request.InstrucoesSolicitante,
		CanaisDigitais:            request.CanaisDigitais,
		CanaisPresenciais:         request.CanaisPresenciais,
		ServicoNaoCobre:           request.ServicoNaoCobre,
		LegislacaoRelacionada:     request.LegislacaoRelacionada,
		TemaGeral:                 request.TemaGeral,
		PublicoEspecifico:         request.PublicoEspecifico,
		ObjetivoCidadao:           request.ObjetivoCidadao,
		FixarDestaque:             request.FixarDestaque,
		Status:                    request.Status,
	}

	// Cria o serviço
	ctx := context.Background()
	createdService, err := h.typesenseClient.CreatePrefRioService(ctx, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao criar serviço: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, createdService)
}

// UpdateService godoc
// @Summary Atualiza um serviço existente
// @Description Atualiza um serviço existente. EDITOR pode atualizar, GERAL e ADMIN podem atualizar e publicar.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Param service body models.PrefRioServiceRequest true "Dados atualizados do serviço"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id} [put]
func (h *AdminHandler) UpdateService(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	var request models.PrefRioServiceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	// Valida os dados
	if err := h.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validação falhou: " + err.Error()})
		return
	}

	// Verifica permissões específicas para status
	userRole := middlewares.GetUserRole(c)
	if request.Status == 1 { // Publicar
		if userRole != "GERAL" && userRole != "ADMIN" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Apenas GERAL e ADMIN podem publicar serviços"})
			return
		}
	}

	// Busca o serviço existente para preservar created_at
	ctx := context.Background()
	existingService, err := h.typesenseClient.GetPrefRioService(ctx, serviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Serviço não encontrado"})
		return
	}

	// Converte para modelo completo preservando dados existentes
	service := &models.PrefRioService{
		ID:                        serviceID,
		NomeServico:               request.NomeServico,
		OrgaoGestor:               request.OrgaoGestor,
		Resumo:                    request.Resumo,
		TempoAtendimento:          request.TempoAtendimento,
		CustoServico:              request.CustoServico,
		ResultadoSolicitacao:      request.ResultadoSolicitacao,
		DescricaoCompleta:         request.DescricaoCompleta,
		Autor:                     existingService.Autor, // Preserva autor original
		DocumentosNecessarios:     request.DocumentosNecessarios,
		InstrucoesSolicitante:     request.InstrucoesSolicitante,
		CanaisDigitais:            request.CanaisDigitais,
		CanaisPresenciais:         request.CanaisPresenciais,
		ServicoNaoCobre:           request.ServicoNaoCobre,
		LegislacaoRelacionada:     request.LegislacaoRelacionada,
		TemaGeral:                 request.TemaGeral,
		PublicoEspecifico:         request.PublicoEspecifico,
		ObjetivoCidadao:           request.ObjetivoCidadao,
		FixarDestaque:             request.FixarDestaque,
		Status:                    request.Status,
		CreatedAt:                 existingService.CreatedAt, // Preserva data de criação
	}

	// Atualiza o serviço
	updatedService, err := h.typesenseClient.UpdatePrefRioService(ctx, serviceID, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao atualizar serviço: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedService)
}

// DeleteService godoc
// @Summary Deleta um serviço
// @Description Deleta um serviço. Apenas GERAL e ADMIN podem deletar.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id} [delete]
func (h *AdminHandler) DeleteService(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	// Deleta o serviço
	ctx := context.Background()
	err := h.typesenseClient.DeletePrefRioService(ctx, serviceID)
	if err != nil {
		if err.Error() == "serviço não encontrado" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Serviço não encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao deletar serviço: " + err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetService godoc
// @Summary Busca um serviço por ID
// @Description Busca um serviço específico por ID
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id} [get]
func (h *AdminHandler) GetService(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	// Busca o serviço
	ctx := context.Background()
	service, err := h.typesenseClient.GetPrefRioService(ctx, serviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Serviço não encontrado"})
		return
	}

	c.JSON(http.StatusOK, service)
}

// ListServices godoc
// @Summary Lista serviços com paginação e filtros
// @Description Lista serviços com paginação e filtros opcionais
// @Tags admin
// @Accept json
// @Produce json
// @Param page query int false "Página" default(1)
// @Param per_page query int false "Resultados por página" default(10)
// @Param status query int false "Status do serviço (0=Draft, 1=Published)"
// @Param author query string false "Filtrar por autor"
// @Param tema_geral query string false "Filtrar por tema geral"
// @Success 200 {object} models.PrefRioServiceResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services [get]
func (h *AdminHandler) ListServices(c *gin.Context) {
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
	filters := make(map[string]interface{})
	
	if status := c.Query("status"); status != "" {
		if statusInt, err := strconv.Atoi(status); err == nil && (statusInt == 0 || statusInt == 1) {
			filters["status"] = statusInt
		}
	}
	
	if author := c.Query("author"); author != "" {
		filters["autor"] = author
	}
	
	if tema := c.Query("tema_geral"); tema != "" {
		filters["tema_geral"] = tema
	}

	// Lista os serviços
	ctx := context.Background()
	response, err := h.typesenseClient.ListPrefRioServices(ctx, page, perPage, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao listar serviços: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// PublishService godoc
// @Summary Publica um serviço (altera status para 1)
// @Description Publica um serviço alterando seu status para 1. Apenas GERAL e ADMIN podem publicar.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id}/publish [patch]
func (h *AdminHandler) PublishService(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	// Busca o serviço existente
	ctx := context.Background()
	service, err := h.typesenseClient.GetPrefRioService(ctx, serviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Serviço não encontrado"})
		return
	}

	// Atualiza apenas o status para publicado
	service.Status = 1
	
	// Atualiza o serviço
	updatedService, err := h.typesenseClient.UpdatePrefRioService(ctx, serviceID, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao publicar serviço: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedService)
}

// UnpublishService godoc
// @Summary Despublica um serviço (altera status para 0)
// @Description Despublica um serviço alterando seu status para 0. Apenas GERAL e ADMIN podem despublicar.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id}/unpublish [patch]
func (h *AdminHandler) UnpublishService(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	// Busca o serviço existente
	ctx := context.Background()
	service, err := h.typesenseClient.GetPrefRioService(ctx, serviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Serviço não encontrado"})
		return
	}

	// Atualiza apenas o status para rascunho
	service.Status = 0
	
	// Atualiza o serviço
	updatedService, err := h.typesenseClient.UpdatePrefRioService(ctx, serviceID, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao despublicar serviço: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedService)
}