package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"magnetm3u8-gateway/internal/user"
)

// AdminHandler serves admin-only APIs.
type AdminHandler struct {
	users *user.Repository
}

func NewAdminHandler(repo *user.Repository) *AdminHandler {
	return &AdminHandler{users: repo}
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	accounts, err := h.users.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "无法加载用户列表"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": accounts})
}

func (h *AdminHandler) UpdateBanState(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "用户ID无效"})
		return
	}

	var payload struct {
		Banned bool `json:"banned"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请求格式不正确"})
		return
	}

	if err := h.users.SetBanState(c.Request.Context(), userID, payload.Banned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "更新状态失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
