package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/api/handlers"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	middlewares "github.com/prefeitura-rio/app-busca-search/internal/middleware"
	"github.com/prefeitura-rio/app-busca-search/internal/typesense"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRouter(cfg *config.Config) *gin.Engine {
	r := gin.Default()

	r.Use(corsMiddleware())

	typesenseClient := typesense.NewClient(cfg)

	buscaHandler := handlers.NewBuscaHandler(typesenseClient)
	adminHandler := handlers.NewAdminHandler(typesenseClient)
	tombamentoHandler := handlers.NewTombamentoHandler(typesenseClient)
	versionHandler := handlers.NewVersionHandler(typesenseClient)

	api := r.Group("/api/v1")
	{
		api.GET("/busca-hibrida-multi", buscaHandler.BuscaMultiColecao)
		api.GET("/categoria/:collections", buscaHandler.BuscaPorCategoria)
		api.GET("/documento/:collection/:id", buscaHandler.BuscaPorID)
		api.GET("/categorias-relevancia", buscaHandler.CategoriasRelevancia)
	}

	// Rotas administrativas com autenticação JWT (sem validação de roles)
	admin := api.Group("/admin")
	admin.Use(middlewares.JWTAuthMiddleware()) // Extrai dados do JWT
	admin.Use(middlewares.RequireJWTAuth())    // Verifica apenas se está autenticado
	{
		services := admin.Group("/services")
		{
			// Criar serviço
			services.POST("", adminHandler.CreateService)

			// Listar serviços
			services.GET("", adminHandler.ListServices)

			// Buscar serviço por ID
			services.GET("/:id", adminHandler.GetService)

			// Atualizar serviço
			services.PUT("/:id", adminHandler.UpdateService)

			// Deletar serviço
			services.DELETE("/:id", adminHandler.DeleteService)

			// Publicar serviço
			services.PATCH("/:id/publish", adminHandler.PublishService)

			// Despublicar serviço
			services.PATCH("/:id/unpublish", adminHandler.UnpublishService)

			// Rotas de versionamento
			services.GET("/:id/versions", versionHandler.ListServiceVersions)
			services.GET("/:id/versions/:version", versionHandler.GetServiceVersion)
			services.GET("/:id/versions/compare", versionHandler.CompareServiceVersions)
			services.POST("/:id/rollback", versionHandler.RollbackService)
		}

		tombamentos := admin.Group("/tombamentos")
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