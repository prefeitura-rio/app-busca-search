package routes

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/api/handlers"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	middlewares "github.com/prefeitura-rio/app-busca-search/internal/middleware"
	"github.com/prefeitura-rio/app-busca-search/internal/migration/schemas"
	v3 "github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search"
	"github.com/prefeitura-rio/app-busca-search/internal/search/adapter"
	"github.com/prefeitura-rio/app-busca-search/internal/search/synonyms"
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

	// Inicializa defaults da v3 a partir das variáveis de ambiente
	v3.SetDefaults(v3.SearchConfigDefaults{
		Alpha:                  cfg.SearchV3.DefaultAlpha,
		TyposHuman:             cfg.SearchV3.DefaultTyposHuman,
		TyposAgent:             cfg.SearchV3.DefaultTyposAgent,
		EnableQueryExpansion:   cfg.SearchV3.EnableQueryExpansion,
		MaxQueryExpansionTerms: cfg.SearchV3.MaxQueryExpansionTerms,
		EnableRecencyBoost:     cfg.SearchV3.EnableRecencyBoost,
		RecencyGracePeriodDays: cfg.SearchV3.RecencyGracePeriodDays,
		RecencyDecayRate:       cfg.SearchV3.RecencyDecayRate,
	})

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
		cfg.GeminiChatModel,
		cache,
		typesenseURL,
		cfg.TypesenseAPIKey,
	)
	searchHandler := handlers.NewSearchHandler(searchService, typesenseClient)

	// Initialize category services
	popularityService := services.NewPopularityService()
	categoryService := services.NewCategoryService(typesenseClient.GetClient(), popularityService)
	categoryHandler := handlers.NewCategoryHandler(categoryService)

	// Initialize subcategory services
	subcategoryService := services.NewSubcategoryService(typesenseClient.GetClient(), popularityService)
	subcategoryHandler := handlers.NewSubcategoryHandler(subcategoryService)

	// Initialize v2 search service (multi-collection)
	var embeddingService services.EmbeddingProvider
	if geminiClient != nil {
		embeddingService = services.NewGeminiEmbeddingProvider(geminiClient, cfg.GeminiEmbeddingModel, cache)
	}
	searchServiceV2 := services.NewSearchServiceV2(
		typesenseClient.GetClient(),
		embeddingService,
		cfg,
	)
	searchHandlerV2 := handlers.NewSearchHandlerV2(searchServiceV2)

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

	// v1 API (services only - backward compatibility)
	api := r.Group("/api/v1")
	{
		// Unified search endpoints
		api.GET("/search", searchHandler.Search)
		api.GET("/search/:id", searchHandler.GetDocumentByID)

		// SEO-friendly service endpoint (by slug)
		api.GET("/services/:slug", searchHandler.GetServiceBySlug)

		// Category endpoints
		api.GET("/categories", categoryHandler.GetCategories)

		// Subcategory endpoints
		api.GET("/categories/:category/subcategories", subcategoryHandler.GetSubcategories)
		api.GET("/subcategories/:subcategory/services", subcategoryHandler.GetServicesBySubcategory)
	}

	// v2 API (multi-collection search)
	apiV2 := r.Group("/api/v2")
	{
		// Multi-collection search endpoints
		apiV2.GET("/search", searchHandlerV2.Search)
		apiV2.GET("/search/:id", searchHandlerV2.GetDocumentByID)
	}

	// v3 API (new architecture with synonyms, query expansion, etc.)
	searchEngineV3 := initializeSearchEngineV3(cfg, typesenseClient, geminiClient, typesenseURL, popularityService)
	searchHandlerV3 := handlers.NewSearchHandlerV3(searchEngineV3)

	apiV3 := r.Group("/api/v3")
	{
		apiV3.GET("/search", searchHandlerV3.Search)
		apiV3.GET("/search/:id", searchHandlerV3.GetDocument)
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

// initializeSearchEngineV3 inicializa o engine de busca v3
func initializeSearchEngineV3(
	cfg *config.Config,
	typesenseClient *typesense.Client,
	geminiClient *genai.Client,
	typesenseURL string,
	popularityService *services.PopularityService,
) *search.Engine {
	// Adapter Typesense
	typesenseAdapter := adapter.NewTypesenseAdapter(
		typesenseClient.GetClient(),
		typesenseURL,
		cfg.TypesenseAPIKey,
	)

	// Adapter Gemini
	var geminiAdapter *adapter.GeminiAdapter
	if geminiClient != nil {
		geminiConfig := adapter.GeminiConfig{
			EmbeddingModel:      cfg.GeminiEmbeddingModel,
			ChatModel:           cfg.GeminiChatModel,
			EmbeddingDimensions: cfg.SearchV3.EmbeddingDimensions,
			MaxTextLength:       cfg.SearchV3.MaxEmbeddingTextLength,
			CacheTTLMinutes:     cfg.SearchV3.EmbeddingCacheTTLMinutes,
			CacheMaxSize:        cfg.SearchV3.EmbeddingCacheMaxSize,
		}
		geminiAdapter = adapter.NewGeminiAdapter(geminiClient, geminiConfig)
	}

	// Serviço de sinônimos
	synonymService := synonyms.NewService(typesenseClient.GetClient(), "prefrio_services_base")

	// Configurações de collections
	collectionConfigs := make(map[string]*v3.CollectionConfig)
	collectionConfigs["prefrio_services_base"] = v3.DefaultPrefRioConfig()

	// Adiciona outras collections configuradas
	for name, collCfg := range cfg.CollectionConfigs {
		if _, exists := collectionConfigs[name]; !exists {
			collectionConfigs[name] = &v3.CollectionConfig{
				Name:           name,
				Type:           collCfg.Type,
				TitleField:     collCfg.TitleField,
				DescField:      collCfg.DescField,
				StatusField:    collCfg.FilterField,
				StatusValue:    collCfg.FilterValue,
				EmbeddingField: "embedding",
				SearchFields:   collCfg.SearchFields,
				SearchWeights:  collCfg.SearchWeights,
			}
		}
	}

	// Cria engine com popularity service
	engine := search.NewEngine(
		typesenseAdapter,
		geminiAdapter,
		synonymService,
		popularityService,
		collectionConfigs,
		"prefrio_services_base",
	)

	// Carrega sinônimos padrão (em background)
	go func() {
		ctx := context.Background()
		if err := engine.LoadSynonyms(ctx); err != nil {
			log.Printf("Aviso: erro ao carregar sinônimos: %v", err)
		}
	}()

	log.Println("Search Engine v3 inicializado com sucesso")
	return engine
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
