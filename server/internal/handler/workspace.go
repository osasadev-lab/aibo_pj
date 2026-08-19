package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/activitylog"
	"github.com/osasadev-lab/aibo_pj/server/ent/attachment"
	"github.com/osasadev-lab/aibo_pj/server/ent/comment"
	"github.com/osasadev-lab/aibo_pj/server/ent/commentmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/project"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectstatuscolumn"
	"github.com/osasadev-lab/aibo_pj/server/ent/section"
	"github.com/osasadev-lab/aibo_pj/server/ent/tag"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskdependency"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspaceinvitation"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"
)

// WorkspaceHandler は /workspaces 配下のエンドポイントを扱う。
type WorkspaceHandler struct {
	client *ent.Client
	r2     *storage.R2Client
}

func NewWorkspaceHandler(client *ent.Client, r2 *storage.R2Client) *WorkspaceHandler {
	return &WorkspaceHandler{client: client, r2: r2}
}

type createWorkspaceRequest struct {
	Name string `json:"name" binding:"required"`
}

// List は GET /workspaces。自分が所属するワークスペース一覧（自分のroleを含む）を返す。
func (h *WorkspaceHandler) List(c *gin.Context) {
	u := middleware.CurrentUser(c)

	memberships, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.UserIDEQ(u.ID)).
		WithWorkspace().
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workspaces"})
		return
	}

	out := make([]gin.H, 0, len(memberships))
	for _, m := range memberships {
		out = append(out, gin.H{
			"id":   m.Edges.Workspace.ID,
			"name": m.Edges.Workspace.Name,
			"role": m.Role,
		})
	}
	c.JSON(http.StatusOK, out)
}

// Create は POST /workspaces。Workspace作成と同時に作成者をOwnerとして登録する。
func (h *WorkspaceHandler) Create(c *gin.Context) {
	u := middleware.CurrentUser(c)

	var req createWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	var ws *ent.Workspace
	err := withTx(c.Request.Context(), h.client, func(tx *ent.Tx) error {
		var err error
		ws, err = tx.Workspace.Create().SetName(req.Name).Save(c.Request.Context())
		if err != nil {
			return err
		}
		_, err = tx.WorkspaceMember.Create().
			SetWorkspaceID(ws.ID).
			SetUserID(u.ID).
			SetRole(workspacemember.RoleOwner).
			Save(c.Request.Context())
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create workspace"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": ws.ID, "name": ws.Name, "role": workspacemember.RoleOwner})
}

// Get は GET /workspaces/:workspace_id。RequireWorkspaceMemberで存在・所属確認済み。
func (h *WorkspaceHandler) Get(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ws, err := h.client.Workspace.Get(c.Request.Context(), m.WorkspaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": ws.ID, "name": ws.Name, "role": m.Role})
}

type updateWorkspaceRequest struct {
	Name string `json:"name" binding:"required"`
}

// Update は PATCH /workspaces/:workspace_id。RequireOwner限定。
func (h *WorkspaceHandler) Update(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	var req updateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	ws, err := h.client.Workspace.UpdateOneID(m.WorkspaceID).SetName(req.Name).Save(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update workspace"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": ws.ID, "name": ws.Name})
}

// Delete は DELETE /workspaces/:workspace_id。RequireOwner限定（Ownerなら誰でも単独で
// 実行可能、他Ownerの同意は不要。ワークスペース配下の全データを削除する不可逆操作）。
// entにOnDeleteカスケードが無いため、FK制約に違反しない順序でアプリ層から手動削除する
// （project.go/task.goのDeleteと同じ考え方で、その全プロジェクト・全タスク分をまとめて行う）。
func (h *WorkspaceHandler) Delete(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	var attachmentKeys []string
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		taskIDs, err := tx.Task.Query().Where(task.WorkspaceIDEQ(m.WorkspaceID)).IDs(ctx)
		if err != nil {
			return err
		}
		projectIDs, err := tx.Project.Query().Where(project.WorkspaceIDEQ(m.WorkspaceID)).IDs(ctx)
		if err != nil {
			return err
		}

		if len(taskIDs) > 0 {
			if _, err := tx.TaskDependency.Delete().
				Where(taskdependency.Or(
					taskdependency.TaskIDIn(taskIDs...),
					taskdependency.DependsOnTaskIDIn(taskIDs...),
				)).
				Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.TaskTag.Delete().Where(tasktag.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.TaskAssignee.Delete().Where(taskassignee.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.TaskMention.Delete().Where(taskmention.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.CommentMention.Delete().
				Where(commentmention.HasCommentWith(comment.TaskIDIn(taskIDs...))).
				Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.Comment.Delete().Where(comment.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
			// R2オブジェクトはDBコミット確定後に削除する（キーだけここで収集）。
			attachments, err := tx.Attachment.Query().Where(attachment.TaskIDIn(taskIDs...)).All(ctx)
			if err != nil {
				return err
			}
			for _, a := range attachments {
				attachmentKeys = append(attachmentKeys, a.StorageKey)
			}
			if _, err := tx.Attachment.Delete().Where(attachment.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
		}

		// activity_logsは全行がworkspace_idを持つため、これ一発で足りる。
		if _, err := tx.ActivityLog.Delete().Where(activitylog.WorkspaceIDEQ(m.WorkspaceID)).Exec(ctx); err != nil {
			return err
		}
		if len(taskIDs) > 0 {
			if _, err := tx.Task.Delete().Where(task.IDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
		}

		if len(projectIDs) > 0 {
			if _, err := tx.Section.Delete().Where(section.ProjectIDIn(projectIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.ProjectStatusColumn.Delete().Where(projectstatuscolumn.ProjectIDIn(projectIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.ProjectMember.Delete().Where(projectmember.ProjectIDIn(projectIDs...)).Exec(ctx); err != nil {
				return err
			}
		}
		// tagsは共通タグ（project_id nil）・プロジェクト専用タグどちらもworkspace_idを
		// 持つため、これ一発で両方消える。task_tagsは既にtask_id経由で消去済み。
		if _, err := tx.Tag.Delete().Where(tag.WorkspaceIDEQ(m.WorkspaceID)).Exec(ctx); err != nil {
			return err
		}
		if len(projectIDs) > 0 {
			if _, err := tx.Project.Delete().Where(project.IDIn(projectIDs...)).Exec(ctx); err != nil {
				return err
			}
		}

		if _, err := tx.WorkspaceInvitation.Delete().Where(workspaceinvitation.WorkspaceIDEQ(m.WorkspaceID)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.WorkspaceMember.Delete().Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID)).Exec(ctx); err != nil {
			return err
		}
		return tx.Workspace.DeleteOneID(m.WorkspaceID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete workspace"})
		return
	}
	deleteR2Objects(ctx, h.r2, attachmentKeys)
	c.Status(http.StatusNoContent)
}
