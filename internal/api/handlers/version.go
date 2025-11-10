package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	middlewares "github.com/prefeitura-rio/app-busca-search/internal/middleware"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

type VersionHandler struct {
	typesenseClient *typesense.Client
}

func NewVersionHandler(client *typesense.Client) *VersionHandler {
	return &VersionHandler{
		typesenseClient: client,
	}
}

// ListServiceVersions godoc
// @Summary Lista todas as versões de um serviço
// @Description Retorna o histórico completo de versões de um serviço com paginação
// @Tags versions
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Param page query int false "Página" default(1)
// @Param per_page query int false "Resultados por página" default(10)
// @Success 200 {object} models.VersionHistory
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id}/versions [get]
func (h *VersionHandler) ListServiceVersions(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))

	ctx := context.Background()
	history, err := h.typesenseClient.ListServiceVersions(ctx, serviceID, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao listar versões: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, history)
}

// GetServiceVersion godoc
// @Summary Busca uma versão específica de um serviço
// @Description Retorna os detalhes de uma versão específica
// @Tags versions
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Param version path int true "Número da versão"
// @Success 200 {object} models.ServiceVersion
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id}/versions/{version} [get]
func (h *VersionHandler) GetServiceVersion(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	versionStr := c.Param("version")
	versionNum, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Número da versão inválido"})
		return
	}

	ctx := context.Background()
	version, err := h.typesenseClient.GetServiceVersionByNumber(ctx, serviceID, versionNum)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Versão não encontrada: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, version)
}

// CompareServiceVersions godoc
// @Summary Compara duas versões de um serviço
// @Description Retorna as diferenças entre duas versões
// @Tags versions
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Param from_version query int true "Versão de origem"
// @Param to_version query int true "Versão de destino"
// @Success 200 {object} models.VersionDiff
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id}/versions/compare [get]
func (h *VersionHandler) CompareServiceVersions(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	fromVersionStr := c.Query("from_version")
	toVersionStr := c.Query("to_version")

	if fromVersionStr == "" || toVersionStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from_version e to_version são obrigatórios"})
		return
	}

	fromVersion, err := strconv.ParseInt(fromVersionStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from_version inválido"})
		return
	}

	toVersion, err := strconv.ParseInt(toVersionStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to_version inválido"})
		return
	}

	ctx := context.Background()
	diff, err := h.typesenseClient.CompareServiceVersions(ctx, serviceID, fromVersion, toVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao comparar versões: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, diff)
}

// RollbackService godoc
// @Summary Realiza rollback de um serviço para uma versão anterior
// @Description Cria uma nova versão que restaura o estado de uma versão anterior (git-revert style)
// @Tags versions
// @Accept json
// @Produce json
// @Param id path string true "ID do serviço"
// @Param rollback body models.RollbackRequest true "Dados do rollback"
// @Success 200 {object} models.PrefRioService
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/services/{id}/rollback [post]
func (h *VersionHandler) RollbackService(c *gin.Context) {
	serviceID := c.Param("id")
	if serviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do serviço é obrigatório"})
		return
	}

	var request models.RollbackRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	ctx := context.Background()

	// Busca a versão alvo do rollback
	targetVersion, err := h.typesenseClient.GetServiceVersionByNumber(ctx, serviceID, request.ToVersion)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Versão alvo não encontrada: " + err.Error()})
		return
	}

	// Busca a versão atual para diff
	currentVersion, err := h.typesenseClient.GetLatestServiceVersion(ctx, serviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao buscar versão atual: " + err.Error()})
		return
	}

	// Cria o serviço com os dados da versão alvo
	rolledBackService := &models.PrefRioService{
		ID:                        serviceID,
		NomeServico:               targetVersion.NomeServico,
		OrgaoGestor:               targetVersion.OrgaoGestor,
		Resumo:                    targetVersion.Resumo,
		TempoAtendimento:          targetVersion.TempoAtendimento,
		CustoServico:              targetVersion.CustoServico,
		ResultadoSolicitacao:      targetVersion.ResultadoSolicitacao,
		DescricaoCompleta:         targetVersion.DescricaoCompleta,
		Autor:                     targetVersion.Autor,
		DocumentosNecessarios:     targetVersion.DocumentosNecessarios,
		InstrucoesSolicitante:     targetVersion.InstrucoesSolicitante,
		CanaisDigitais:            targetVersion.CanaisDigitais,
		CanaisPresenciais:         targetVersion.CanaisPresenciais,
		ServicoNaoCobre:           targetVersion.ServicoNaoCobre,
		LegislacaoRelacionada:     targetVersion.LegislacaoRelacionada,
		TemaGeral:                 targetVersion.TemaGeral,
		PublicoEspecifico:         targetVersion.PublicoEspecifico,
		FixarDestaque:             targetVersion.FixarDestaque,
		AwaitingApproval:          targetVersion.AwaitingApproval,
		PublishedAt:               targetVersion.PublishedAt,
		IsFree:                    targetVersion.IsFree,
		Status:                    targetVersion.Status,
		SearchContent:             targetVersion.SearchContent,
	}

	// Atualiza o serviço com os dados do rollback
	changeReason := request.ChangeReason
	if changeReason == "" {
		changeReason = "Rollback para versão " + strconv.FormatInt(request.ToVersion, 10)
	}

	updatedService, err := h.typesenseClient.UpdatePrefRioServiceWithVersion(
		ctx,
		serviceID,
		rolledBackService,
		middlewares.GetUserName(c),
		middlewares.GetUserCPF(c),
		changeReason,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao realizar rollback: " + err.Error()})
		return
	}

	// Marca a nova versão como rollback
	// Nota: Isso seria feito no versionService.CaptureVersion, mas precisamos atualizar
	// para suportar o flag is_rollback. Por enquanto, retornamos sucesso.

	c.JSON(http.StatusOK, gin.H{
		"message":         "Rollback realizado com sucesso",
		"rolled_back_to":  request.ToVersion,
		"previous_version": currentVersion.VersionNumber,
		"service":         updatedService,
	})
}
