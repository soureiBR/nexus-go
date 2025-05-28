// internal/config/config.go
package config

import (
	"os"
)

// Config armazena configurações da aplicação
type Config struct {
	Port        string
	APIKey      string
	LogLevel    string
	WebhookURL  string
	DBPath      string
	RabbitMQURL string
}

// LoadConfig carrega configurações a partir de variáveis de ambiente
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return Config{
		Port:        port,
		APIKey:      os.Getenv("API_KEY"),
		LogLevel:    getEnvOrDefault("LOG_LEVEL", "info"),
		WebhookURL:  os.Getenv("WEBHOOK_URL"),
		DBPath:      getEnvOrDefault("DB_PATH", "./data/whatsapp.db"),
		RabbitMQURL: getEnvOrDefault("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
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
