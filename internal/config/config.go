// internal/config/config.go
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
	
	"github.com/joho/godotenv"
	"yourproject/pkg/logger"
)

// Config contém as configurações da aplicação
type Config struct {
	// Configurações do servidor
	Port            string
	Host            string
	Environment     string
	APIKey          string
	
	// Configurações do WhatsApp
	SessionDir      string
	WebhookURL      string
	WebhookSecret   string
	
	// Configurações de logging
	LogLevel        string
	LogFormat       string
	
	// Configurações de limpeza de sessões
	CleanupInterval  time.Duration
	MaxInactiveTime  time.Duration
	
	// Configurações de timeout
	RequestTimeout  time.Duration
	WebhookTimeout  time.Duration
	
	// Configurações de upload
	MaxUploadSize   int64
	TempDir         string
}

// LoadConfig carrega as configurações a partir de variáveis de ambiente
func LoadConfig() *Config {
	// Carregar .env se existir (ignorar erro se não existir)
	_ = godotenv.Load()
	
	config := &Config{
		// Configurações do servidor
		Port:            getEnv("PORT", "8080"),
		Host:            getEnv("HOST", "0.0.0.0"),
		Environment:     getEnv("ENVIRONMENT", "production"),
		APIKey:          getEnv("API_KEY", ""),
		
		// Configurações do WhatsApp
		SessionDir:      getEnv("SESSION_DIR", "./sessions"),
		WebhookURL:      getEnv("WEBHOOK_URL", ""),
		WebhookSecret:   getEnv("WEBHOOK_SECRET", ""),
		
		// Configurações de logging
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		LogFormat:       getEnv("LOG_FORMAT", "json"),
		
		// Configurações de limpeza de sessões
		CleanupInterval: getDurationEnv("CLEANUP_INTERVAL", 24*time.Hour),
		MaxInactiveTime: getDurationEnv("MAX_INACTIVE_TIME", 72*time.Hour),
		
		// Configurações de timeout
		RequestTimeout:  getDurationEnv("REQUEST_TIMEOUT", 30*time.Second),
		WebhookTimeout:  getDurationEnv("WEBHOOK_TIMEOUT", 10*time.Second),
		
		// Configurações de upload
		MaxUploadSize:   getInt64Env("MAX_UPLOAD_SIZE", 10*1024*1024), // 10 MB
		TempDir:         getEnv("TEMP_DIR", os.TempDir()),
	}
	
	// Validar configurações críticas
	validateConfig(config)
	
	return config
}

// getEnv obtém uma variável de ambiente ou retorna um valor padrão
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getBoolEnv obtém uma variável de ambiente como boolean
func getBoolEnv(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	b, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	
	return b
}

// getIntEnv obtém uma variável de ambiente como inteiro
func getIntEnv(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	i, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	
	return i
}

// getInt64Env obtém uma variável de ambiente como int64
func getInt64Env(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	i, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	
	return i
}

// getDurationEnv obtém uma variável de ambiente como Duration
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	d, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	
	return d
}

// getSliceEnv obtém uma variável de ambiente como slice de strings
func getSliceEnv(key, sep string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	return strings.Split(value, sep)
}

// validateConfig valida configurações críticas
func validateConfig(config *Config) {
	// Verificar se o diretório de sessões existe
	if _, err := os.Stat(config.SessionDir); os.IsNotExist(err) {
		logger.Info("Diretório de sessões não existe, criando...", "dir", config.SessionDir)
		if err := os.MkdirAll(config.SessionDir, 0755); err != nil {
			logger.Error("Falha ao criar diretório de sessões", "error", err)
		}
	}
	
	// Alertar se não há API key em produção
	if config.Environment == "production" && config.APIKey == "" {
		logger.Warn("API_KEY não configurada em ambiente de produção!")
	}
	
	// Verificar tempos de inatividade e limpeza
	if config.MaxInactiveTime < time.Hour {
		logger.Warn("MAX_INACTIVE_TIME muito baixo, ajustando para 1 hora")
		config.MaxInactiveTime = time.Hour
	}
	
	// Verificar diretório temporário
	if _, err := os.Stat(config.TempDir); os.IsNotExist(err) {
		logger.Warn("Diretório temporário não existe, usando padrão do sistema", "temp_dir", os.TempDir())
		config.TempDir = os.TempDir()
	}
}