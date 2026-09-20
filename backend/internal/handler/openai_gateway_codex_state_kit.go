package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetCodexStateKit returns Turn-State kit status for an OpenAI OAuth account.
// GET /api/v1/admin/openai/accounts/:id/codex-state-kit
func (h *OpenAIGatewayHandler) GetCodexStateKit(c *gin.Context) {
	accountID, ok := parseAdminAccountID(c)
	if !ok {
		return
	}
	status, err := h.gatewayService.GetCodexStateKitStatus(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// RefreshCodexStateKit runs an on-demand harvest round for the account.
// POST /api/v1/admin/openai/accounts/:id/codex-state-kit/refresh
func (h *OpenAIGatewayHandler) RefreshCodexStateKit(c *gin.Context) {
	accountID, ok := parseAdminAccountID(c)
	if !ok {
		return
	}
	status, err := h.gatewayService.RefreshCodexStateKitAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// UpdateCodexStateKit updates per-account kit settings stored in extra.
// PUT /api/v1/admin/openai/accounts/:id/codex-state-kit
func (h *OpenAIGatewayHandler) UpdateCodexStateKit(c *gin.Context) {
	accountID, ok := parseAdminAccountID(c)
	if !ok {
		return
	}
	var patch service.CodexStateKitUpdate
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	status, err := h.gatewayService.UpdateCodexStateKitAccount(c.Request.Context(), accountID, patch)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func parseAdminAccountID(c *gin.Context) (int64, bool) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return accountID, true
}
