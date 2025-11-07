package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"magnetm3u8-gateway/internal/auth"
	"magnetm3u8-gateway/internal/http/middleware"
	"magnetm3u8-gateway/internal/user"
)

// AuthHandler exposes HTTP handlers for authentication flows.
type AuthHandler struct {
	service    *auth.Service
	cookieName string
	sessionTTL time.Duration
}

func NewAuthHandler(service *auth.Service, cookieName string, ttl time.Duration) *AuthHandler {
	return &AuthHandler{
		service:    service,
		cookieName: cookieName,
		sessionTTL: ttl,
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请求格式不正确"})
		return
	}

	user, err := h.service.Register(c.Request.Context(), payload.Username, payload.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": sanitizeUser(user)})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请求格式不正确"})
		return
	}

	token, user, err := h.service.Login(c.Request.Context(), payload.Username, payload.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": err.Error()})
		return
	}

	h.setSessionCookie(c, token)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": sanitizeUser(user)})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token, err := c.Cookie(h.cookieName)
	if err == nil && token != "" {
		_ = h.service.Logout(c.Request.Context(), token)
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *AuthHandler) Profile(c *gin.Context) {
	if user, ok := middleware.CurrentUser(c); ok && user != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": sanitizeUser(user)})
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "未登录"})
}

func (h *AuthHandler) setSessionCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int(h.sessionTTL.Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
}

type userDTO struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	IsBanned  bool      `json:"is_banned"`
	CreatedAt time.Time `json:"created_at"`
}

func sanitizeUser(u *user.User) userDTO {
	return userDTO{
		ID:        u.ID,
		Username:  u.Username,
		Role:      u.Role,
		IsBanned:  u.IsBanned,
		CreatedAt: u.CreatedAt,
	}
}
