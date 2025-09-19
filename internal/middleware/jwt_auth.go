package middlewares

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTClaims representa os claims do JWT
type JWTClaims struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	EmailVerified     bool   `json:"email_verified"`
	ResourceAccess    struct {
		Superapp struct {
			Roles []string `json:"roles"`
		} `json:"superapp"`
	} `json:"resource_access"`
}

// JWTAuthMiddleware extrai informações do usuário do JWT
// Não valida assinatura nem roles - apenas extrai dados para uso interno
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token não fornecido"})
			c.Abort()
			return
		}

		// Remove "Bearer " do header
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		
		// Parse do JWT (sem validação de assinatura)
		claims, err := parseJWTClaims(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token inválido: " + err.Error()})
			c.Abort()
			return
		}

		// Extrai dados do usuário para o contexto
		c.Set(UserCPFKey, claims.PreferredUsername)
		c.Set(UserIDKey, claims.Sub)
		c.Set(UserNameKey, claims.Name)
		c.Set(UserEmailKey, claims.Email)
		
		// Extrai role principal (para logs/auditoria, não para autorização)
		role := extractPrimaryRole(claims)
		c.Set(UserRoleKey, role)
		
		c.Next()
	}
}

// parseJWTClaims decodifica o payload do JWT sem validar assinatura
func parseJWTClaims(tokenString string) (*JWTClaims, error) {
	// JWT tem 3 partes: header.payload.signature
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, gin.Error{Err: gin.Error{}, Type: gin.ErrorTypePublic}
	}

	// Decodifica o payload (parte do meio)
	payload := parts[1]
	
	// Adiciona padding se necessário
	if len(payload)%4 != 0 {
		payload += strings.Repeat("=", 4-len(payload)%4)
	}
	
	// Decodifica base64
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}

	// Parse JSON
	var claims JWTClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, err
	}

	return &claims, nil
}

// extractPrimaryRole extrai a role principal para logs/auditoria
func extractPrimaryRole(claims *JWTClaims) string {
	// Verifica se tem go:admin
	for _, role := range claims.ResourceAccess.Superapp.Roles {
		if role == "go:admin" {
			return "ADMIN"
		}
		if role == "admin:login" {
			return "USER"
		}
	}
	return "USER"
}

// RequireJWTAuth middleware simples que apenas verifica se há JWT válido
// Não faz validação de roles - isso será feito externamente
func RequireJWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userCPF := GetUserCPF(c)
		if userCPF == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não autenticado"})
			c.Abort()
			return
		}
		c.Next()
	}
}
