package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	middlewares "github.com/prefeitura-rio/app-busca-search/internal/middleware"
	"github.com/prefeitura-rio/app-busca-search/internal/migration/schemas"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
)

// MigrationHandler gerencia operações de migração de schema
type MigrationHandler struct {
	migrationService *services.MigrationService
	schemaRegistry   *schemas.Registry
	validator        *validator.Validate
}

// NewMigrationHandler cria um novo handler de migração
func NewMigrationHandler(migrationService *services.MigrationService, schemaRegistry *schemas.Registry) *MigrationHandler {
	return &MigrationHandler{
		migrationService: migrationService,
		schemaRegistry:   schemaRegistry,
		validator:        validator.New(),
	}
}

// StartMigration godoc
// @Summary Inicia uma migração de schema
// @Description Inicia o processo de migração para uma nova versão de schema. O sistema será bloqueado para operações CUD durante a migração.
// @Tags migration
// @Accept json
// @Produce json
// @Param migration body models.MigrationStartRequest true "Dados da migração"
// @Success 200 {object} models.MigrationStatusResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/migration/start [post]
func (h *MigrationHandler) StartMigration(c *gin.Context) {
	var request models.MigrationStartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	if err := h.validator.Struct(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validação falhou: " + err.Error()})
		return
	}

	userName := middlewares.GetUserName(c)
	userCPF := middlewares.GetUserCPF(c)

	response, err := h.migrationService.StartMigration(c.Request.Context(), &request, userName, userCPF)
	if err != nil {
		if isConflictError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetStatus godoc
// @Summary Obtém o status atual da migração
// @Description Retorna o status da migração em andamento ou o último estado conhecido
// @Tags migration
// @Produce json
// @Success 200 {object} models.MigrationStatusResponse
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/migration/status [get]
func (h *MigrationHandler) GetStatus(c *gin.Context) {
	response, err := h.migrationService.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// Rollback godoc
// @Summary Executa rollback para a versão anterior
// @Description Restaura a collection para o backup da última migração
// @Tags migration
// @Accept json
// @Produce json
// @Param rollback body models.MigrationRollbackRequest false "Dados do rollback"
// @Success 200 {object} models.MigrationStatusResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/migration/rollback [post]
func (h *MigrationHandler) Rollback(c *gin.Context) {
	var request models.MigrationRollbackRequest
	c.ShouldBindJSON(&request)

	userName := middlewares.GetUserName(c)
	userCPF := middlewares.GetUserCPF(c)

	response, err := h.migrationService.RollbackMigration(c.Request.Context(), &request, userName, userCPF)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if isConflictError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetHistory godoc
// @Summary Lista o histórico de migrações
// @Description Retorna o histórico completo de migrações com paginação
// @Tags migration
// @Produce json
// @Param page query int false "Página" default(1)
// @Param per_page query int false "Resultados por página" default(10)
// @Success 200 {object} models.MigrationHistoryResponse
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/migration/history [get]
func (h *MigrationHandler) GetHistory(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	perPage, err := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if err != nil || perPage < 1 || perPage > 100 {
		perPage = 10
	}

	response, err := h.migrationService.GetHistory(c.Request.Context(), page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// ListSchemas godoc
// @Summary Lista os schemas disponíveis
// @Description Retorna a lista de versões de schema disponíveis para migração
// @Tags migration
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/admin/migration/schemas [get]
func (h *MigrationHandler) ListSchemas(c *gin.Context) {
	versions := h.schemaRegistry.ListVersions()
	currentVersion := h.schemaRegistry.GetCurrentVersion()

	c.JSON(http.StatusOK, gin.H{
		"current_version":    currentVersion,
		"available_versions": versions,
	})
}

// isConflictError verifica se o erro é de conflito (migração em andamento)
func isConflictError(err error) bool {
	msg := err.Error()
	return contains(msg, "em andamento") || contains(msg, "in progress")
}

// isNotFoundError verifica se o erro é de recurso não encontrado
func isNotFoundError(err error) bool {
	msg := err.Error()
	return contains(msg, "não encontrad") || contains(msg, "not found")
}

// contains verifica se a string contém o substring (case insensitive simplificado)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsAt(s, substr, 0)))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

