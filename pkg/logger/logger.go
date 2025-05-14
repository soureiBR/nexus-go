// pkg/logger/logger.go
package logger

import (
	"log/slog"
	"os"
)

var (
	// Logger global
	logger *slog.Logger
)

// Níveis de log
const (
	DEBUG = slog.LevelDebug
	INFO  = slog.LevelInfo
	WARN  = slog.LevelWarn
	ERROR = slog.LevelError
)

// Setup configura o logger global
func Setup(logLevel string) {
	// Determinar nível de log
	var level slog.Level
	switch logLevel {
	case "debug":
		level = DEBUG
	case "info":
		level = INFO
	case "warn":
		level = WARN
	case "error":
		level = ERROR
	default:
		level = INFO
	}
	
	// Configurar handler com nível
	opts := &slog.HandlerOptions{
		Level: level,
	}
	
	// Criar handler JSON
	handler := slog.NewJSONHandler(os.Stdout, opts)
	
	// Configurar logger global
	logger = slog.New(handler)
	
	// Substituir logger global
	slog.SetDefault(logger)
	
	Info("Logger configurado", "level", logLevel)
}

// Debug registra uma mensagem de debug
func Debug(msg string, args ...interface{}) {
	if logger == nil {
		// Se o logger não foi configurado, usar o padrão
		slog.Debug(msg, args...)
		return
	}
	
	logger.Debug(msg, args...)
}

// Info registra uma mensagem informativa
func Info(msg string, args ...interface{}) {
	if logger == nil {
		// Se o logger não foi configurado, usar o padrão
		slog.Info(msg, args...)
		return
	}
	
	logger.Info(msg, args...)
}

// Warn registra uma mensagem de aviso
func Warn(msg string, args ...interface{}) {
	if logger == nil {
		// Se o logger não foi configurado, usar o padrão
		slog.Warn(msg, args...)
		return
	}
	
	logger.Warn(msg, args...)
}

// Error registra uma mensagem de erro
func Error(msg string, args ...interface{}) {
	if logger == nil {
		// Se o logger não foi configurado, usar o padrão
		slog.Error(msg, args...)
		return
	}
	
	logger.Error(msg, args...)
}

// With adiciona campos contextuais ao logger
func With(args ...interface{}) *slog.Logger {
	if logger == nil {
		// Se o logger não foi configurado, usar o padrão
		return slog.With(args...)
	}
	
	return logger.With(args...)
}

// GetLogger retorna o logger global
func GetLogger() *slog.Logger {
	if logger == nil {
		// Se o logger não foi configurado, usar o padrão
		return slog.Default()
	}
	
	return logger
}