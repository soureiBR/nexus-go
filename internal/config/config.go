// internal/config/config.go
package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config armazena configurações da aplicação
type Config struct {
	Port        string
	APIKey      string
	AdminAPIKey string
	LogLevel    string
	WebhookURL  string
	DBPath      string
	RabbitMQURL string
	PrintQR     bool
}

// LoadEnv loads environment variables from .env file
func LoadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
		// Don't fail if .env file doesn't exist, just log a warning
	}
}

// LoadConfig carrega configurações a partir de variáveis de ambiente
func LoadConfig() Config {
	// Load .env file first
	LoadEnv()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return Config{
		Port:        port,
		APIKey:      os.Getenv("API_KEY"),
		AdminAPIKey: os.Getenv("ADMIN_API_KEY"),
		LogLevel:    getEnvOrDefault("LOG_LEVEL", "debug"),
		WebhookURL:  os.Getenv("WEBHOOK_URL"),
		DBPath:      getEnvOrDefault("DB_PATH", "./data/whatsapp.db"),
		RabbitMQURL: getEnvOrDefault("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		PrintQR:     getBoolEnvOrDefault("PRINT_QR", false),
	}
}

// getEnvOrDefault obtém uma variável de ambiente ou retorna valor padrão
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getBoolEnvOrDefault obtém uma variável de ambiente boolean ou retorna valor padrão
func getBoolEnvOrDefault(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Printf("Warning: Invalid boolean value for %s: %s, using default: %v", key, value, defaultValue)
		return defaultValue
	}
	
	return parsed
}
