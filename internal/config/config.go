package config

import (
	"log"
	"os"

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
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Fatalln("Error loading .env file")
	}

	return &Config{
		TypesenseHost: getEnv("TYPESENSE_HOST", "localhost"),
		TypesensePort: getEnv("TYPESENSE_PORT", "8108"),
		TypesenseAPIKey: getEnv("TYPESENSE_API_KEY", ""),
		TypesenseProtocol: getEnv("TYPESENSE_PROTOCOL", "http"),

		ServerPort: getEnv("SERVER_PORT", "8080"),
		
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
		GeminiEmbeddingModel: getEnv("GEMINI_EMBEDDING_MODEL", "text-embedding-004"),
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}