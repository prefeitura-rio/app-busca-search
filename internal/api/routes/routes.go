package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/api/handlers"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/prefeitura-rio/app-busca-search/internal/middleware"
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

	api := r.Group("/api/v1")
	{
		api.GET("/busca-hibrida-multi", buscaHandler.BuscaMultiColecao)
		api.GET("/categoria/:collections", buscaHandler.BuscaPorCategoria)
		api.GET("/documento/:collection/:id", buscaHandler.BuscaPorID)
		api.GET("/categorias-relevancia", buscaHandler.CategoriasRelevancia)
	}

	// Rotas administrativas com autenticação e autorização
	admin := api.Group("/admin")
	admin.Use(middlewares.ExtractUserContext())
	admin.Use(middlewares.RequireAuthentication())
	{
		services := admin.Group("/services")
		{
			// Criar serviço: apenas GERAL e ADMIN
			services.POST("", middlewares.RequireRole("GERAL", "ADMIN"), adminHandler.CreateService)
			
			// Listar serviços: todos os roles autenticados
			services.GET("", adminHandler.ListServices)
			
			// Buscar serviço por ID: todos os roles autenticados
			services.GET("/:id", adminHandler.GetService)
			
			// Atualizar serviço: EDITOR, GERAL e ADMIN
			services.PUT("/:id", middlewares.RequireRole("EDITOR", "GERAL", "ADMIN"), adminHandler.UpdateService)
			
			// Deletar serviço: apenas GERAL e ADMIN
			services.DELETE("/:id", middlewares.RequireRole("GERAL", "ADMIN"), adminHandler.DeleteService)
			
			// Publicar serviço: apenas GERAL e ADMIN
			services.PATCH("/:id/publish", middlewares.RequireRole("GERAL", "ADMIN"), adminHandler.PublishService)
			
			// Despublicar serviço: apenas GERAL e ADMIN
			services.PATCH("/:id/unpublish", middlewares.RequireRole("GERAL", "ADMIN"), adminHandler.UnpublishService)
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