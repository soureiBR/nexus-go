// internal/api/middlewares/auth.go
package middlewares

import (
	"net/http"
	"strings"
	
	"github.com/gin-gonic/gin"
	"yourproject/pkg/logger"
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