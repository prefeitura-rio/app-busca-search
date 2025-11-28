package routes

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/api/handlers"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	middlewares "github.com/prefeitura-rio/app-busca-search/internal/middleware"
	"github.com/prefeitura-rio/app-busca-search/internal/migration/schemas"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"google.golang.org/genai"
)

func SetupRouter(cfg *config.Config) *gin.Engine {
	r := gin.Default()

	r.Use(corsMiddleware())
	r.Use(middlewares.RequestTiming()) // Add OpenTelemetry tracing

	typesenseClient := typesense.NewClient(cfg)

	// Initialize Gemini client
	ctx := context.Background()
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.GeminiAPIKey,
	})
	if err != nil {
		println("Aviso: Gemini client não inicializado, busca vetorial desabilitada:", err.Error())
		geminiClient = nil
	}

	// Initialize cache service (500 entries, cleanup a cada 5min)
	cache := services.NewLRUCache(500)
	cache.StartCleanupRoutine(5 * time.Minute)

	// Initialize handlers
	adminHandler := handlers.NewAdminHandler(typesenseClient)
	tombamentoHandler := handlers.NewTombamentoHandler(typesenseClient)
	versionHandler := handlers.NewVersionHandler(typesenseClient)

	// Initialize search service (direct search)
	typesenseURL := fmt.Sprintf("%s://%s:%s", cfg.TypesenseProtocol, cfg.TypesenseHost, cfg.TypesensePort)
	searchService := services.NewSearchService(
		typesenseClient.GetClient(),
		geminiClient,
		cfg.GeminiEmbeddingModel,
		cache,
		typesenseURL,
		cfg.TypesenseAPIKey,
	)
	searchHandler := handlers.NewSearchHandler(searchService, typesenseClient)

	// Initialize category services
	popularityService := services.NewPopularityService()
	categoryService := services.NewCategoryService(typesenseClient.GetClient(), popularityService)
	categoryHandler := handlers.NewCategoryHandler(categoryService)

	// Initialize migration services
	schemaRegistry := schemas.NewRegistry()
	migrationService := services.NewMigrationService(typesenseClient.GetClient(), schemaRegistry)
	migrationHandler := handlers.NewMigrationHandler(migrationService, schemaRegistry)
	migrationLockMiddleware := middlewares.NewMigrationLockMiddleware(migrationService)

	// Initialize health handler
	healthHandler := handlers.NewHealthHandler(typesenseClient)

	// Health check endpoints (no /api/v1 prefix for K8s probes and uptime monitoring)
	r.GET("/liveness", healthHandler.Liveness)   // K8s liveness probe
	r.GET("/readiness", healthHandler.Readiness) // K8s readiness probe
	r.GET("/health", healthHandler.Health)       // Uptime monitoring (comprehensive)

	api := r.Group("/api/v1")
	{
		// Unified search endpoints
		api.GET("/search", searchHandler.Search)
		api.GET("/search/:id", searchHandler.GetDocumentByID)

		// Category endpoints
		api.GET("/categories", categoryHandler.GetCategories)
	}

	// Rotas administrativas com autenticação JWT
	admin := api.Group("/admin")
	admin.Use(middlewares.JWTAuthMiddleware()) // Extrai dados do JWT
	admin.Use(middlewares.RequireJWTAuth())    // Verifica apenas se está autenticado
	{
		// Rotas de serviços com bloqueio de CUD durante migrações
		servicesGroup := admin.Group("/services")
		servicesGroup.Use(migrationLockMiddleware.BlockCUD()) // Bloqueia CUD durante migrações
		{
			// Criar serviço
			servicesGroup.POST("", adminHandler.CreateService)

			// Listar serviços (GET não é bloqueado)
			servicesGroup.GET("", adminHandler.ListServices)

			// Buscar serviço por ID (GET não é bloqueado)
			servicesGroup.GET("/:id", adminHandler.GetService)

			// Atualizar serviço
			servicesGroup.PUT("/:id", adminHandler.UpdateService)

			// Deletar serviço
			servicesGroup.DELETE("/:id", adminHandler.DeleteService)

			// Publicar serviço
			servicesGroup.PATCH("/:id/publish", adminHandler.PublishService)

			// Despublicar serviço
			servicesGroup.PATCH("/:id/unpublish", adminHandler.UnpublishService)

			// Rotas de versionamento (GET não é bloqueado)
			servicesGroup.GET("/:id/versions", versionHandler.ListServiceVersions)
			servicesGroup.GET("/:id/versions/:version", versionHandler.GetServiceVersion)
			servicesGroup.GET("/:id/versions/compare", versionHandler.CompareServiceVersions)
			servicesGroup.POST("/:id/rollback", versionHandler.RollbackService)
		}

		// Rotas de tombamentos com bloqueio de CUD durante migrações
		tombamentos := admin.Group("/tombamentos")
		tombamentos.Use(migrationLockMiddleware.BlockCUD()) // Bloqueia CUD durante migrações
		{
			// Criar tombamento
			tombamentos.POST("", tombamentoHandler.CreateTombamento)

			// Listar tombamentos
			tombamentos.GET("", tombamentoHandler.ListTombamentos)

			// Buscar tombamento por serviço antigo
			tombamentos.GET("/by-old-service", tombamentoHandler.GetTombamentoByOldService)

			// Buscar tombamento por ID
			tombamentos.GET("/:id", tombamentoHandler.GetTombamento)

			// Atualizar tombamento
			tombamentos.PUT("/:id", tombamentoHandler.UpdateTombamento)

			// Deletar tombamento
			tombamentos.DELETE("/:id", tombamentoHandler.DeleteTombamento)
		}

		// Rotas de migração de schema (não bloqueadas)
		migration := admin.Group("/migration")
		{
			// Iniciar migração
			migration.POST("/start", migrationHandler.StartMigration)

			// Verificar status
			migration.GET("/status", migrationHandler.GetStatus)

			// Executar rollback
			migration.POST("/rollback", migrationHandler.Rollback)

			// Histórico de migrações
			migration.GET("/history", migrationHandler.GetHistory)

			// Listar schemas disponíveis
			migration.GET("/schemas", migrationHandler.ListSchemas)
		}
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
