package handler

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/githubissue"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

var taskStatusLabels = map[task.Status]string{
	task.StatusNotStarted: "未対応",
	task.StatusInProgress: "対応中",
	task.StatusDone:       "対応済",
	task.StatusOnHold:     "保留",
}

// resolveGitHubConfig はworkspaces.github_token（AES-256-GCM暗号化）を復号し、
// GitHub APIクライアント用のConfigを組み立てる。未設定ならConfigured()=falseの
// ゼロ値を返す（エラーにはしない、呼び出し元でgithub_not_configuredとして扱う）。
func (h *TaskHandler) resolveGitHubConfig(ctx context.Context, workspaceID uuid.UUID) (githubissue.Config, error) {
	w, err := h.client.Workspace.Get(ctx, workspaceID)
	if err != nil {
		return githubissue.Config{}, err
	}
	if w.GithubToken == nil || *w.GithubToken == "" {
		return githubissue.Config{}, nil
	}
	token, err := internalauth.DecryptToken(h.encKey, *w.GithubToken)
	if err != nil {
		return githubissue.Config{}, err
	}
	return githubissue.Config{Token: token}, nil
}

// PostGitHubIssueComment は POST /tasks/:task_id/github-issue/comment。
// タスクの現在の内容（タイトル・説明・ステータス・担当者・期限）を要約し、
// github_issue_urlにリンク済みのGitHub Issueへコメントとして投稿する。
// 既に一度投稿済み（github_issue_comment_idが設定済み）なら、新規コメントを
// 都度作らず同じコメントを更新する（ユーザー要望、2026-08-27追加。編集・削除した
// 内容も反映したいという要望に対し、投稿を都度上書きする方式で応える）。
// 認証にはワークスペースOwnerが設定したPersonal Access Token（workspaces.github_token、
// AES-256-GCM暗号化）を使う。ユーザーが明示的に押すボタン起点の同期呼び出しで、
// 結果（成功/失敗・コメントURL）をそのままレスポンスで返す必要があるため
// バックグラウンド化はしない（api-spec.md「手動モード」のGoogleカレンダー連携と同じ判断）。
func (h *TaskHandler) PostGitHubIssueComment(c *gin.Context) {
	t := middleware.CurrentTask(c)
	ctx := c.Request.Context()

	if t.GithubIssueURL == nil || strings.TrimSpace(*t.GithubIssueURL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_issue_url_not_set"})
		return
	}
	cfg, err := h.resolveGitHubConfig(ctx, t.WorkspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load github settings"})
		return
	}
	if !cfg.Configured() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_not_configured"})
		return
	}
	ref, err := githubissue.ParseIssueURL(*t.GithubIssueURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_github_issue_url"})
		return
	}

	full, err := h.client.Task.Query().
		Where(task.IDEQ(t.ID)).
		WithAssignees(func(q *ent.TaskAssigneeQuery) { q.WithUser() }).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task"})
		return
	}

	body := buildGitHubIssueCommentBody(full, h.frontendURL)

	var commentID int64
	var htmlURL string
	if full.GithubIssueCommentID != nil {
		commentID, htmlURL, err = githubissue.UpdateComment(ctx, cfg, ref, *full.GithubIssueCommentID, body)
		if err != nil {
			// 更新対象のコメントがGitHub側で削除されている等の場合は新規作成にフォールバックする。
			commentID, htmlURL, err = githubissue.CreateComment(ctx, cfg, ref, body)
		}
	} else {
		commentID, htmlURL, err = githubissue.CreateComment(ctx, cfg, ref, body)
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to post github comment"})
		return
	}

	if _, err := h.client.Task.UpdateOneID(t.ID).SetGithubIssueCommentID(commentID).Save(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save comment reference"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"comment_id": commentID, "html_url": htmlURL})
}

// buildGitHubIssueCommentBody はタイトル・説明・ステータス・担当者・期限を
// Markdownで要約する（ユーザー確認済みの内容範囲）。
func buildGitHubIssueCommentBody(t *ent.Task, frontendURL string) string {
	var sb strings.Builder
	sb.WriteString("### " + t.Title + "\n\n")
	if t.Description != nil && strings.TrimSpace(*t.Description) != "" {
		sb.WriteString(*t.Description + "\n\n")
	}
	sb.WriteString("---\n")
	sb.WriteString("- ステータス: " + taskStatusLabels[t.Status] + "\n")
	if len(t.Edges.Assignees) > 0 {
		names := make([]string, 0, len(t.Edges.Assignees))
		for _, a := range t.Edges.Assignees {
			if a.Edges.User != nil {
				names = append(names, a.Edges.User.Name)
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			sb.WriteString("- 担当者: " + strings.Join(names, ", ") + "\n")
		}
	}
	if t.DueDate != nil {
		sb.WriteString("- 期限: " + t.DueDate.In(jst).Format("2006-01-02 15:04") + "\n")
	}
	sb.WriteString("\n[aisuでタスクを開く](" + frontendURL + "/w/" + t.WorkspaceID.String() + "/my-tasks?task=" + t.ID.String() + ")\n\n")
	sb.WriteString("_このコメントはaisuのタスク内容から自動更新されています。_")
	return sb.String()
}
