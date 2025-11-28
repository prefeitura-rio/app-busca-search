package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
)

// HealthHandler gerencia os endpoints de health check
type HealthHandler struct {
	typesenseClient *typesense.Client
}

// NewHealthHandler cria um novo handler de health check
func NewHealthHandler(client *typesense.Client) *HealthHandler {
	return &HealthHandler{
		typesenseClient: client,
	}
}

// HealthResponse representa a resposta do health check
type HealthResponse struct {
	Status    string            `json:"status"`
	Checks    map[string]string `json:"checks,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp int64             `json:"timestamp"`
}

// Liveness godoc
// @Summary Liveness probe endpoint
// @Description Verifica se a aplicação está viva (sem checagem de dependências externas)
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /liveness [get]
func (h *HealthHandler) Liveness(c *gin.Context) {
	// Liveness apenas confirma que o app está rodando
	// Sem checagens de dependências externas
	c.JSON(http.StatusOK, HealthResponse{
		Status:    "alive",
		Timestamp: time.Now().Unix(),
	})
}

// Readiness godoc
// @Summary Readiness probe endpoint
// @Description Verifica se a aplicação está pronta para receber tráfego (valida Typesense)
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Failure 503 {object} HealthResponse
// @Router /readiness [get]
func (h *HealthHandler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	response := HealthResponse{
		Status:    "ready",
		Checks:    make(map[string]string),
		Timestamp: time.Now().Unix(),
	}

	// Check Typesense connectivity (required for serving traffic)
	typesenseHealthy := h.checkTypesense(ctx)
	if typesenseHealthy {
		response.Checks["typesense"] = "ok"
	} else {
		response.Checks["typesense"] = "failed"
		response.Status = "not_ready"
		response.Error = "Typesense not available"
	}

	// Return appropriate status code
	statusCode := http.StatusOK
	if response.Status == "not_ready" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, response)
}

// Health godoc
// @Summary Comprehensive health check endpoint
// @Description Verifica a saúde completa da aplicação (para monitoramento externo de uptime)
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Failure 503 {object} HealthResponse
// @Router /health [get]
func (h *HealthHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	response := HealthResponse{
		Status:    "healthy",
		Checks:    make(map[string]string),
		Timestamp: time.Now().Unix(),
	}

	// Check Typesense connectivity
	typesenseHealthy := h.checkTypesense(ctx)
	if typesenseHealthy {
		response.Checks["typesense"] = "ok"
	} else {
		response.Checks["typesense"] = "failed"
		response.Status = "unhealthy"
		response.Error = "Typesense connectivity check failed"
	}

	// Future: Add more checks here (Gemini API, etc.)
	// response.Checks["gemini"] = "ok"

	// Return appropriate status code
	statusCode := http.StatusOK
	if response.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, response)
}

// checkTypesense verifica a conectividade com o Typesense
func (h *HealthHandler) checkTypesense(ctx context.Context) bool {
	// Tenta verificar a saúde do Typesense usando a API Health
	_, err := h.typesenseClient.GetClient().Health(ctx, 2*time.Second)
	return err == nil
}
