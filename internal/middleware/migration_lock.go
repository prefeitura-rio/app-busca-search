package middlewares

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
)

// MigrationLockMiddleware bloqueia operações CUD durante migrações de schema
type MigrationLockMiddleware struct {
	migrationService *services.MigrationService
	cacheLock        sync.RWMutex
	cachedLocked     bool
	cacheExpiry      time.Time
	cacheTTL         time.Duration
}

// NewMigrationLockMiddleware cria um novo middleware de bloqueio de migração
func NewMigrationLockMiddleware(migrationService *services.MigrationService) *MigrationLockMiddleware {
	return &MigrationLockMiddleware{
		migrationService: migrationService,
		cacheTTL:         5 * time.Second,
	}
}

// BlockCUD retorna um handler Gin que bloqueia operações CUD durante migrações
func (m *MigrationLockMiddleware) BlockCUD() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if !isCUDMethod(method) {
			c.Next()
			return
		}

		locked, err := m.isLocked(c)
		if err != nil {
			c.Next()
			return
		}

		if locked {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Sistema em manutenção",
				"message": "Uma migração de schema está em andamento. Operações de criação, atualização e exclusão estão temporariamente bloqueadas. Tente novamente em alguns minutos.",
				"code":    "MIGRATION_IN_PROGRESS",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// isLocked verifica se o sistema está bloqueado (com cache)
func (m *MigrationLockMiddleware) isLocked(c *gin.Context) (bool, error) {
	m.cacheLock.RLock()
	if time.Now().Before(m.cacheExpiry) {
		locked := m.cachedLocked
		m.cacheLock.RUnlock()
		return locked, nil
	}
	m.cacheLock.RUnlock()

	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()

	if time.Now().Before(m.cacheExpiry) {
		return m.cachedLocked, nil
	}

	ctx := c.Request.Context()
	locked, err := m.migrationService.IsMigrationLocked(ctx)
	if err != nil {
		return false, err
	}

	m.cachedLocked = locked
	m.cacheExpiry = time.Now().Add(m.cacheTTL)

	return locked, nil
}

// isCUDMethod verifica se o método HTTP é uma operação CUD
func isCUDMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

