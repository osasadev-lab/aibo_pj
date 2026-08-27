package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// GetGitHubSettings は GET /workspaces/:workspace_id/github-settings（M8.5後追加）。
// connectedはgithub_tokenを保持しているかどうかのみを表す（生の値はフロントへ絶対に
// 返さない。GoogleカレンダーのGetCalendarSettingsと同じ考え方）。ワークスペースの
// 全メンバーが閲覧可（タスク詳細でIssueコメントボタンを出し分けるために必要なため）。
func (h *WorkspaceHandler) GetGitHubSettings(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	w, err := h.client.Workspace.Get(c.Request.Context(), m.WorkspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load github settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": w.GithubToken != nil && *w.GithubToken != ""})
}

type updateGitHubSettingsRequest struct {
	Token string `json:"token"`
}

// UpdateGitHubSettings は PATCH /workspaces/:workspace_id/github-settings（M8.5後追加）。
// ワークスペースメンバーなら誰でも変更可（当初Owner限定としていたが、設定できる人が
// 限られすぎるとのユーザーフィードバックにより2026-08-27に変更）。tokenを空文字にすると
// 連携解除（github_tokenをクリア）、非空ならAES-256-GCMで暗号化して保存する
// （users.google_refresh_tokenと同じ暗号化方式、TOKEN_ENCRYPTION_KEYを再利用）。
// 発行されたトークン自体が有効かどうかはここでは検証しない（コメント投稿時に初めて
// GitHub API呼び出しで判明する、Googleカレンダー連携と異なりOAuth同意フローを
// 経由しないため事前検証の手段が無い）。
func (h *WorkspaceHandler) UpdateGitHubSettings(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	var req updateGitHubSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	token := strings.TrimSpace(req.Token)
	builder := h.client.Workspace.UpdateOneID(m.WorkspaceID)
	if token == "" {
		builder = builder.ClearGithubToken()
	} else {
		encrypted, err := internalauth.EncryptToken(h.encKey, token)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt token"})
			return
		}
		builder = builder.SetGithubToken(encrypted)
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update github settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": updated.GithubToken != nil && *updated.GithubToken != ""})
}
