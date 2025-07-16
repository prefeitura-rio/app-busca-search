package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	TypesenseHost string
	TypesensePort string
	TypesenseAPIKey string
	TypesenseProtocol string

	ServerPort string

	GeminiAPIKey string
	GeminiEmbeddingModel string

	// Configurações de relevância
	RelevanciaArquivo1746          string
	RelevanciaArquivoCariocaDigital string
	RelevanciaIntervaloAtualizacao  int // em minutos

	// Configuração de filtro
	FilterCSVPath string
}

func LoadConfig() *Config {
	_ = godotenv.Load()

	return &Config{
		TypesenseHost: getEnv("TYPESENSE_HOST", "localhost"),
		TypesensePort: getEnv("TYPESENSE_PORT", "8108"),
		TypesenseAPIKey: getEnv("TYPESENSE_API_KEY", ""),
		TypesenseProtocol: getEnv("TYPESENSE_PROTOCOL", "http"),

		ServerPort: getEnv("SERVER_PORT", "8080"),
		
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
		GeminiEmbeddingModel: getEnv("GEMINI_EMBEDDING_MODEL", "text-embedding-004"),
		
		// Configurações de relevância
		RelevanciaArquivo1746: getEnv("RELEVANCIA_ARQUIVO_1746", "data/volumetria_1746.csv"),
		RelevanciaArquivoCariocaDigital: getEnv("RELEVANCIA_ARQUIVO_CARIOCA_DIGITAL", "data/volumetria_carioca_digital.csv"),
		RelevanciaIntervaloAtualizacao: getEnvInt("RELEVANCIA_INTERVALO_ATUALIZACAO", 60),

		// Configuração de filtro
		FilterCSVPath: getEnv("FILTER_CSV_PATH", "data/servicos_similares_carioca_1746_20250702_095454.csv"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
} 