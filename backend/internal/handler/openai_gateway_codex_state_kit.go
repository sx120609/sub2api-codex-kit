package handler

import (
	"strconv"
	"strings"

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

// IngestCodexStateKit accepts locally harvested Turn-State tokens.
// POST /v1/codex-state-kit/tokens  (gateway API key)
// POST /api/v1/codex-state-kit/tokens  (user JWT)
// POST /api/v1/admin/openai/accounts/:id/codex-state-kit/tokens
// POST /api/v1/admin/openai/codex-state-kit/tokens
func (h *OpenAIGatewayHandler) IngestCodexStateKit(c *gin.Context) {
	var accountID int64
	if raw := strings.TrimSpace(c.Param("id")); raw != "" {
		parsed, ok := parseAdminAccountID(c)
		if !ok {
			return
		}
		accountID = parsed
	}
	var req service.CodexStateKitIngest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if accountID <= 0 && req.AccountID > 0 {
		accountID = req.AccountID
	}
	result, err := h.gatewayService.IngestCodexStateKitTokens(c.Request.Context(), accountID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseAdminAccountID(c *gin.Context) (int64, bool) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return accountID, true
}
