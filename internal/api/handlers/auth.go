// internal/api/handlers/auth.go
package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"yourproject/pkg/logger"
)

// AuthHandler gerencia endpoints para operações de autenticação
type AuthHandler struct {
	encryptionKey []byte // 32 bytes para AES-256
}

// NewAuthHandler cria um novo handler para autenticação
func NewAuthHandler(secretKey string) *AuthHandler {
	// Garantir que a chave tenha 32 bytes para AES-256
	key := make([]byte, 32)
	copy(key, []byte(secretKey))
	
	return &AuthHandler{
		encryptionKey: key,
	}
}

// EncryptSessionRequest representa o request para criptografar uma sessão
type EncryptSessionRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// EncryptSessionResponse representa a resposta com a chave criptografada
type EncryptSessionResponse struct {
	UserSecret string    `json:"user_secret"`
	ExpiresAt  string    `json:"expires_at"`
	UserID     string    `json:"user_id"`
}

// EncryptSession gera uma chave criptografada para um usuário
func (h *AuthHandler) EncryptSession(c *gin.Context) {
	var req EncryptSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// Validar se o userID foi fornecido
	if req.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "user_id é obrigatório",
		})
		return
	}

	// Criar payload com userID e timestamp de expiração (24 horas)
	expiresAt := time.Now().Add(24 * time.Hour)
	payload := fmt.Sprintf("%s|%d", req.UserID, expiresAt.Unix())

	// Criptografar o payload
	encryptedSecret, err := h.encrypt(payload)
	if err != nil {
		logger.Error("Falha ao criptografar user secret", "error", err, "user_id", req.UserID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Falha ao gerar user secret",
		})
		return
	}

	response := EncryptSessionResponse{
		UserSecret: encryptedSecret,
		ExpiresAt:  expiresAt.Format(time.RFC3339),
		UserID:     req.UserID,
	}

	logger.Info("User secret gerado com sucesso", "user_id", req.UserID, "expires_at", expiresAt)

	c.JSON(http.StatusOK, response)
}

// encrypt criptografa um texto usando AES-256-GCM
func (h *AuthHandler) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(h.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt descriptografa um texto usando AES-256-GCM
func (h *AuthHandler) Decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(h.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext_bytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext_bytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
