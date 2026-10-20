package admin

import (
	"net/http"

	"github.com/MACOS-DO/sub4api/internal/pkg/response"
	"github.com/MACOS-DO/sub4api/internal/service"

	"github.com/gin-gonic/gin"
)

// GetCodexFingerprintVersion 返回当前 ModelTrace 指纹库 commit 与库内可归属的
// GPT 模型列表（降智检测的判定基准）。
func (h *AccountHandler) GetCodexFingerprintVersion(c *gin.Context) {
	models, err := service.ModelTraceGPTModels()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"commit": service.ModelTraceBankCommit(), "models": models})
}

// RefreshCodexFingerprint 从上游拉取最新 ModelTrace 指纹库。
func (h *AccountHandler) RefreshCodexFingerprint(c *gin.Context) {
	if h.codexDiagnosticSettings == nil {
		response.ErrorWithDetails(c, http.StatusUnprocessableEntity, "Codex diagnostic unavailable", "CODEX_DIAGNOSTIC_UNAVAILABLE", nil)
		return
	}
	commit, err := h.codexDiagnosticSettings.RefreshModelTraceBank(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	models, err := service.ModelTraceGPTModels()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"commit": commit, "models": models})
}

// GetCodexTicketHistory 返回目标账号的降智探测记录（手动诊断 + 自动检测），
// 最新在前。filter 支持 all/success/failed。
func (h *AccountHandler) GetCodexTicketHistory(c *gin.Context) {
	id, ok := codexDiagnosticAccountID(c)
	if !ok {
		return
	}
	model := c.Query("model")
	filter := c.DefaultQuery("filter", "all")
	if filter != "all" && filter != "success" && filter != "failed" {
		response.BadRequest(c, "filter must be all, success or failed")
		return
	}
	page, size := response.ParsePagination(c)
	if size > 100 {
		size = 100
	}
	if h.codexTicketAttempts == nil {
		response.ErrorWithDetails(c, http.StatusUnprocessableEntity, "Codex diagnostic unavailable", "CODEX_DIAGNOSTIC_UNAVAILABLE", nil)
		return
	}
	rows, total, err := h.codexTicketAttempts.List(c.Request.Context(), id, model, filter, page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": rows, "total": total, "page": page, "page_size": size})
}

// DeleteCodexTicketHistory 清空目标账号的降智探测记录（可按 model/filter 收窄）。
func (h *AccountHandler) DeleteCodexTicketHistory(c *gin.Context) {
	id, ok := codexDiagnosticAccountID(c)
	if !ok {
		return
	}
	model := c.Query("model")
	filter := c.DefaultQuery("filter", "all")
	if filter != "all" && filter != "success" && filter != "failed" {
		response.BadRequest(c, "filter must be all, success or failed")
		return
	}
	if h.codexTicketAttempts == nil {
		response.ErrorWithDetails(c, http.StatusUnprocessableEntity, "Codex diagnostic unavailable", "CODEX_DIAGNOSTIC_UNAVAILABLE", nil)
		return
	}
	deleted, err := h.codexTicketAttempts.Delete(c.Request.Context(), id, model, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": deleted})
}
