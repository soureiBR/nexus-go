// internal/api/middlewares/auth.go
package middlewares

import (
	"net/http"
	"strings"

	"yourproject/pkg/logger"

	"github.com/gin-gonic/gin"
)

type AuthMiddleware struct {
	apiKey string
}

func NewAuthMiddleware(apiKey string) *AuthMiddleware {
	return &AuthMiddleware{
		apiKey: apiKey,
	}
}

// Authenticate verifica se a requisição contém um token de API válido
func (am *AuthMiddleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Se estamos em um ambiente de desenvolvimento e não há chave configurada, pular
		if am.apiKey == "" {
			logger.Warn("API key não configurada, autenticação desabilitada")
			c.Next()
			return
		}

		// Verificar Authorization header
		authHeader := c.GetHeader("Authorization")

		// Verificar formato do header
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key não fornecida"})
			return
		}

		// Verificar formato Bearer token
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Formato de API key inválido"})
			return
		}

		// Extrair token
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Verificar token
		if token != am.apiKey {
			logger.Warn("Tentativa de acesso com API key inválida", "ip", c.ClientIP())
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key inválida"})
			return
		}

		// Token válido, continuar
		c.Next()
	}
}

// ExtractUserID extrai o ID do usuário do cabeçalho X-User-ID e adiciona ao contexto
func (am *AuthMiddleware) ExtractUserID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Obter user_id do cabeçalho
		userID := c.GetHeader("X-User-ID")

		// Verificar se o user_id foi fornecido
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header é obrigatório"})
			return
		}

		// Adicionar o user_id ao contexto para uso nas handlers
		c.Set("userID", userID)
		logger.Debug("User ID extraído do cabeçalho", "user_id", userID)

		c.Next()
	}
}

// AuthenticateAndExtractUserID combina autenticação e extração de user_id
func (am *AuthMiddleware) AuthenticateAndExtractUserID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Authenticate API key if configured
		if am.apiKey != "" {
			authHeader := c.GetHeader("Authorization")

			if authHeader == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key não fornecida"})
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Formato de API key inválido"})
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")

			if token != am.apiKey {
				logger.Warn("Tentativa de acesso com API key inválida", "ip", c.ClientIP())
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key inválida"})
				return
			}
		} else {
			logger.Warn("API key não configurada, autenticação desabilitada")
		}

		// Extract user ID from header (mandatory regardless of API key config)
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "X-User-ID header é obrigatório"})
			return
		}

		// Set user ID in context
		c.Set("userID", userID)
		logger.Debug("User ID extraído do cabeçalho", "user_id", userID)

		c.Next()
	}
}
