// internal/api/middlewares/logger.go
package middlewares

import (
	"time"
	
	"github.com/gin-gonic/gin"
	"yourproject/pkg/logger"
)

// Logger middleware para logging de requisições HTTP
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Tempo de início
		startTime := time.Now()
		
		// Path da requisição
		path := c.Request.URL.Path
		
		// Query string
		query := c.Request.URL.RawQuery
		if query != "" {
			path = path + "?" + query
		}
		
		// IP do cliente
		clientIP := c.ClientIP()
		
		// Método HTTP
		method := c.Request.Method
		
		// User agent
		userAgent := c.Request.UserAgent()
		
		// Processar requisição
		c.Next()
		
		// Tempo de resposta
		latency := time.Since(startTime)
		
		// Status da resposta
		statusCode := c.Writer.Status()
		
		// Tamanho da resposta
		responseSize := c.Writer.Size()
		
		// Log da requisição
		logger.Info("HTTP Request",
			"status", statusCode,
			"method", method,
			"path", path,
			"ip", clientIP,
			"latency", latency,
			"size", responseSize,
			"user_agent", userAgent,
		)
	}
}

// RecoveryWithLogger middleware para recuperação de pânico com logging
func RecoveryWithLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log detalhado com stack trace
				logger.Error("Panic recuperado",
					"error", err,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"ip", c.ClientIP(),
				)
				
				// Responder com erro 500
				c.AbortWithStatusJSON(500, gin.H{
					"error": "Erro interno do servidor",
				})
			}
		}()
		
		c.Next()
	}
}