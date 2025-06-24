package main

import (
	"log"

	_ "github.com/prefeitura-rio/app-busca-search/docs"
	"github.com/prefeitura-rio/app-busca-search/internal/api/routes"
	"github.com/prefeitura-rio/app-busca-search/internal/config"
)

// @title           Mecanismo de Busca API
// @version         1.0
// @description     API para busca textual e vetorial usando Typesense e embeddings gerados via Google Gemini
// @termsOfService  http://swagger.io/terms/

// @contact.name   Prefeitura do Rio de Janeiro
// @contact.url    https://prefeitura.rio
// @contact.email  contato@prefeitura.rio

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      services.staging.app.dados.rio/app-busca-search

func main() {

	cfg := config.LoadConfig()

	r := routes.SetupRouter(cfg)

	log.Printf("Servidor iniciado na porta %s", cfg.ServerPort)
	err := r.Run(":" + cfg.ServerPort)
	if err != nil {
		log.Fatalf("Erro ao iniciar servidor: %v", err)
	}
}