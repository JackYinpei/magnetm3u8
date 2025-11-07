package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"magnetm3u8-gateway/internal/auth"
	"magnetm3u8-gateway/internal/user"
)

const contextUserKey = "currentUser"

// Session attaches the authenticated user to the Gin context via cookie lookup.
func Session(authService *auth.Service, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(cookieName)
		if err == nil && token != "" {
			if account, fetchErr := authService.UserFromToken(c.Request.Context(), token); fetchErr == nil && account != nil {
				c.Set(contextUserKey, account)
			}
		}
		c.Next()
	}
}

func currentUser(c *gin.Context) (*user.User, bool) {
	val, exists := c.Get(contextUserKey)
	if !exists {
		return nil, false
	}
	account, ok := val.(*user.User)
	return account, ok
}

// CurrentUser exposes the authenticated user for handlers.
func CurrentUser(c *gin.Context) (*user.User, bool) {
	return currentUser(c)
}

// RequireAuth aborts with 401 when the user is not authenticated.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if account, ok := currentUser(c); !ok || account == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "需要登录后才能操作",
			})
			return
		}
		c.Next()
	}
}

// RequireAdmin aborts with 403 when the user is not an administrator.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		account, ok := currentUser(c)
		if !ok || account == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "未登录",
			})
			return
		}

		if account.Role != user.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "需要管理员权限",
			})
			return
		}

		c.Next()
	}
}
