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
// @Description Cria um novo serviço na collection prefrio_services_base
// @Tags admin
// @Accept json
// @Produce json
// @Param service body models.PrefRioServiceRequest true "Dados do serviço"
// @Success 201 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
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
		FixarDestaque:             request.FixarDestaque,
		AwaitingApproval:          request.AwaitingApproval,
		PublishedAt:               request.PublishedAt,
		IsFree:                    request.IsFree,
		Agents:                    request.Agents,
		ExtraFields:               request.ExtraFields,
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
// @Description Atualiza um serviço existente
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Param service body models.PrefRioServiceRequest true "Dados atualizados do serviço"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
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

	// Nota: Validação de permissões será feita externamente à API

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
		FixarDestaque:             request.FixarDestaque,
		AwaitingApproval:          request.AwaitingApproval,
		PublishedAt:               request.PublishedAt,
		IsFree:                    request.IsFree,
		Agents:                    request.Agents,
		ExtraFields:               request.ExtraFields,
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
// @Description Deleta um serviço
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
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
// @Param awaiting_approval query bool false "Filtrar por aguardando aprovação"
// @Param is_free query bool false "Filtrar por serviços gratuitos"
// @Param published_at query int false "Filtrar por data de publicação (timestamp)"
// @Param field query string false "Campo para filtro dinâmico"
// @Param value query string false "Valor para filtro dinâmico (usado com field)"
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

	if awaitingApproval := c.Query("awaiting_approval"); awaitingApproval != "" {
		if approvalBool, err := strconv.ParseBool(awaitingApproval); err == nil {
			filters["awaiting_approval"] = approvalBool
		}
	}

	if isFree := c.Query("is_free"); isFree != "" {
		if freeBool, err := strconv.ParseBool(isFree); err == nil {
			filters["is_free"] = freeBool
		}
	}

	if publishedAt := c.Query("published_at"); publishedAt != "" {
		if publishedAtInt, err := strconv.ParseInt(publishedAt, 10, 64); err == nil {
			filters["published_at"] = publishedAtInt
		}
	}

	// Filtro dinâmico por campo e valor
	if field := c.Query("field"); field != "" {
		if value := c.Query("value"); value != "" {
			filters[field] = value
		}
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
// @Summary Publica um serviço (altera status para 1 e marca como aprovado)
// @Description Publica um serviço alterando seu status para 1 e awaiting_approval para false
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
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

	// Atualiza status para publicado e marca como aprovado
	service.Status = 1
	service.AwaitingApproval = false
	
	// Atualiza o serviço
	updatedService, err := h.typesenseClient.UpdatePrefRioService(ctx, serviceID, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao publicar serviço: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedService)
}

// UnpublishService godoc
// @Summary Despublica um serviço (altera status para 0 e marca como aguardando aprovação)
// @Description Despublica um serviço alterando seu status para 0 e awaiting_approval para true
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
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

	// Atualiza status para rascunho e marca como aguardando aprovação
	service.Status = 0
	service.AwaitingApproval = true
	
	// Atualiza o serviço
	updatedService, err := h.typesenseClient.UpdatePrefRioService(ctx, serviceID, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao despublicar serviço: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedService)
}