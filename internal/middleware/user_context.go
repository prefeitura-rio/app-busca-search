package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	UserCPFKey = "user_cpf"
	UserRoleKey = "user_role"
	UserIDKey = "user_id"
	UserNameKey = "user_name"
	UserEmailKey = "user_email"
)

// ExtractUserContext extrai informações do usuário dos headers injetados pelo Istio
// O Istio deve injetar os seguintes headers após validar o JWT:
// - X-User-CPF: CPF do usuário (extraído de preferred_username)
// - X-User-Role: Role do usuário (ADMIN se tem go:admin em resource_access.superapp.roles)
// - X-User-ID: ID do usuário (extraído de sub)
// - X-User-Name: Nome completo (extraído de name)
// - X-User-Email: Email do usuário (extraído de email)
func ExtractUserContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		// CPF do usuário (preferred_username no JWT)
		cpf := c.GetHeader("X-User-CPF")
		if cpf != "" {
			c.Set(UserCPFKey, cpf)
		}

		// ID do usuário (sub no JWT)
		userID := c.GetHeader("X-User-ID")
		if userID != "" {
			c.Set(UserIDKey, userID)
		}

		// Role do usuário (baseado em resource_access.superapp.roles)
		// Istio deve enviar "ADMIN" se go:admin está presente, senão "USER"
		role := c.GetHeader("X-User-Role")
		if role != "" {
			c.Set(UserRoleKey, strings.ToUpper(role))
		}

		// Nome completo do usuário
		userName := c.GetHeader("X-User-Name")
		if userName != "" {
			c.Set(UserNameKey, userName)
		}

		// Email do usuário
		userEmail := c.GetHeader("X-User-Email")
		if userEmail != "" {
			c.Set(UserEmailKey, userEmail)
		}

		c.Next()
	}
}

// GetUserCPF retorna o CPF do usuário autenticado
func GetUserCPF(c *gin.Context) string {
	if cpf, exists := c.Get(UserCPFKey); exists {
		if cpfStr, ok := cpf.(string); ok {
			return cpfStr
		}
	}
	return ""
}

// GetUserRole retorna o role do usuário (ADMIN ou USER)
func GetUserRole(c *gin.Context) string {
	if role, exists := c.Get(UserRoleKey); exists {
		if roleStr, ok := role.(string); ok {
			return roleStr
		}
	}
	return ""
}

// GetUserID retorna o ID único do usuário
func GetUserID(c *gin.Context) string {
	if userID, exists := c.Get(UserIDKey); exists {
		if userIDStr, ok := userID.(string); ok {
			return userIDStr
		}
	}
	return ""
}

// GetUserName retorna o nome completo do usuário
func GetUserName(c *gin.Context) string {
	if userName, exists := c.Get(UserNameKey); exists {
		if userNameStr, ok := userName.(string); ok {
			return userNameStr
		}
	}
	return ""
}

// GetUserEmail retorna o email do usuário
func GetUserEmail(c *gin.Context) string {
	if userEmail, exists := c.Get(UserEmailKey); exists {
		if userEmailStr, ok := userEmail.(string); ok {
			return userEmailStr
		}
	}
	return ""
}

// IsAdmin verifica se o usuário tem role ADMIN
func IsAdmin(c *gin.Context) bool {
	role := GetUserRole(c)
	return role == "ADMIN"
}

// IsEditor verifica se o usuário tem role EDITOR
func IsEditor(c *gin.Context) bool {
	role := GetUserRole(c)
	return role == "EDITOR"
}

// IsGeral verifica se o usuário tem role GERAL
func IsGeral(c *gin.Context) bool {
	role := GetUserRole(c)
	return role == "GERAL"
}

// HasRole verifica se o usuário tem uma das roles especificadas
func HasRole(c *gin.Context, roles ...string) bool {
	userRole := GetUserRole(c)
	for _, role := range roles {
		if userRole == role {
			return true
		}
	}
	return false
}

// RequireRole middleware que verifica se o usuário tem uma das roles necessárias
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := GetUserRole(c)
		
		// Verifica se tem uma das roles necessárias
		hasRole := false
		for _, role := range roles {
			if userRole == role {
				hasRole = true
				break
			}
		}
		
		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Acesso negado: permissão insuficiente",
				"roles_required": roles,
				"user_role": userRole,
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// RequireAuthentication middleware que verifica se o usuário está autenticado
func RequireAuthentication() gin.HandlerFunc {
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

// RequireOwnershipOrAdmin middleware que verifica se o usuário é owner ou admin
func RequireOwnershipOrAdmin(ownerCPF string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userCPF := GetUserCPF(c)
		userRole := GetUserRole(c)

		// Admin tem acesso total
		if userRole == "ADMIN" {
			c.Next()
			return
		}

		// Se não tem CPF, não está autenticado
		if userCPF == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuário não identificado"})
			c.Abort()
			return
		}

		// Verifica se é o próprio usuário
		if userCPF != ownerCPF {
			c.JSON(http.StatusForbidden, gin.H{"error": "Acesso negado: você só pode acessar seus próprios dados"})
			c.Abort()
			return
		}

		c.Next()
	}
}