package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
)

const ctxTaskKey = "task"

// RequireTaskAccess はパスの :task_id を解決し、呼び出しユーザーがそのタスクを
// 閲覧できるか検証する。単体タスク（project_idがnil）はworkspace全体に公開、
// プロジェクト所属タスクはそのプロジェクトの可視性ルール（CheckProjectVisibility）
// に従う。RequireAuthの後段で使う。
func RequireTaskAccess(client *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		taskID, err := uuid.Parse(c.Param("task_id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}

		ctx := c.Request.Context()

		t, err := client.Task.Get(ctx, taskID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}

		membership, err := client.WorkspaceMember.Query().
			Where(
				workspacemember.WorkspaceIDEQ(t.WorkspaceID),
				workspacemember.UserIDEQ(u.ID),
			).
			Only(ctx)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a workspace member"})
			return
		}

		if t.ProjectID != nil {
			p, err := client.Project.Get(ctx, *t.ProjectID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
				return
			}
			if err := CheckProjectVisibility(ctx, client, p, u.ID); err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a project member"})
				return
			}
		}

		c.Set(ctxMembershipKey, membership)
		c.Set(ctxTaskKey, t)
		c.Next()
	}
}

// CurrentTask はRequireTaskAccessが格納した*ent.Taskを取り出す。
func CurrentTask(c *gin.Context) *ent.Task {
	v, ok := c.Get(ctxTaskKey)
	if !ok {
		return nil
	}
	t, _ := v.(*ent.Task)
	return t
}
