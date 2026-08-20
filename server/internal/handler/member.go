package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/project"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/user"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

var (
	errLastOwner = errors.New("last owner cannot be demoted or removed")
	errForbidden = errors.New("owner role required")
)

// MemberHandler は /workspaces/:workspace_id/members 配下のエンドポイントを扱う。
type MemberHandler struct {
	client *ent.Client
}

func NewMemberHandler(client *ent.Client) *MemberHandler {
	return &MemberHandler{client: client}
}

// List は GET .../members。メンバー一覧（氏名・アイコン・ロール）を返す。
func (h *MemberHandler) List(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	members, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID)).
		WithUser().
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
		return
	}

	out := make([]gin.H, 0, len(members))
	for _, mem := range members {
		out = append(out, gin.H{
			"id":         mem.ID,
			"user_id":    mem.Edges.User.ID,
			"name":       mem.Edges.User.Name,
			"email":      mem.Edges.User.Email,
			"avatar_url": mem.Edges.User.AvatarURL,
			"role":       mem.Role,
		})
	}
	c.JSON(http.StatusOK, out)
}

type inviteMemberRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// Invite は POST .../members/invite。RequireOwner限定。
// 既存ユーザーならその場でmember化、未登録ならworkspace_invitationsに保留する。
func (h *MemberHandler) Invite(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	inviter := middleware.CurrentUser(c)

	var req inviteMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid email is required"})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.client.User.Query().Where(user.EmailEQ(req.Email)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		_, err := h.client.WorkspaceInvitation.Create().
			SetWorkspaceID(m.WorkspaceID).
			SetEmail(req.Email).
			SetInvitedBy(inviter.ID).
			Save(ctx)
		if ent.IsConstraintError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "already_invited"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to invite member"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"status": "invited"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up user"})
		return
	}

	alreadyMember, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID), workspacemember.UserIDEQ(existing.ID)).
		Exist(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check membership"})
		return
	}
	if alreadyMember {
		c.JSON(http.StatusConflict, gin.H{"error": "already_member"})
		return
	}

	created, err := h.client.WorkspaceMember.Create().
		SetWorkspaceID(m.WorkspaceID).
		SetUserID(existing.ID).
		SetRole(workspacemember.RoleMember).
		Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add member"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": created.ID, "user_id": existing.ID, "role": created.Role})
}

type changeRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=owner member"`
}

// ChangeRole は PATCH .../members/:member_id。RequireOwner限定。
func (h *MemberHandler) ChangeRole(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	memberID, err := uuid.Parse(c.Param("member_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	var req changeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid role is required"})
		return
	}
	newRole := workspacemember.Role(req.Role)

	ctx := c.Request.Context()
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		target, err := tx.WorkspaceMember.Query().
			Where(workspacemember.IDEQ(memberID), workspacemember.WorkspaceIDEQ(m.WorkspaceID)).
			Only(ctx)
		if err != nil {
			return err
		}

		if target.Role == workspacemember.RoleOwner && newRole != workspacemember.RoleOwner {
			if err := ensureNotLastOwner(ctx, tx, m.WorkspaceID, target.ID); err != nil {
				return err
			}
		}

		_, err = tx.WorkspaceMember.UpdateOneID(target.ID).SetRole(newRole).Save(ctx)
		return err
	})

	switch {
	case err == errLastOwner:
		c.JSON(http.StatusBadRequest, gin.H{"error": "last_owner"})
	case ent.IsNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change role"})
	default:
		c.Status(http.StatusNoContent)
	}
}

// Remove は DELETE .../members/:member_id。
// Owner本人以外の削除はOwner限定、本人による脱退は常に可能（ただし最後のOwnerは不可）。
func (h *MemberHandler) Remove(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	caller := middleware.CurrentUser(c)

	memberID, err := uuid.Parse(c.Param("member_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	ctx := c.Request.Context()
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		target, err := tx.WorkspaceMember.Query().
			Where(workspacemember.IDEQ(memberID), workspacemember.WorkspaceIDEQ(m.WorkspaceID)).
			Only(ctx)
		if err != nil {
			return err
		}

		isSelf := target.UserID == caller.ID
		if !isSelf && m.Role != workspacemember.RoleOwner {
			return errForbidden
		}

		if target.Role == workspacemember.RoleOwner {
			if err := ensureNotLastOwner(ctx, tx, m.WorkspaceID, target.ID); err != nil {
				return err
			}
		}

		return tx.WorkspaceMember.DeleteOne(target).Exec(ctx)
	})

	switch {
	case err == errLastOwner:
		c.JSON(http.StatusBadRequest, gin.H{"error": "last_owner"})
	case err == errForbidden:
		c.JSON(http.StatusForbidden, gin.H{"error": "owner role required"})
	case ent.IsNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove member"})
	default:
		c.Status(http.StatusNoContent)
	}
}

// MemberTasks は GET /workspaces/:workspace_id/members/:member_id/tasks。
// 対象メンバーの担当タスク一覧・進捗状況（進捗画面の個人版、spec.md 4.7）を返す。
// :member_idはworkspace_members.id（ChangeRole/Removeと同じ解決方法）。
func (h *MemberHandler) MemberTasks(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	memberID, err := uuid.Parse(c.Param("member_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	target, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.IDEQ(memberID), workspacemember.WorkspaceIDEQ(m.WorkspaceID)).
		WithUser().
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	tasks, err := h.client.Task.Query().
		Where(
			task.WorkspaceIDEQ(m.WorkspaceID),
			task.HasAssigneesWith(taskassignee.UserIDEQ(target.UserID)),
			task.Or(
				task.ProjectIDIsNil(),
				task.HasProjectWith(project.VisibilityEQ(project.VisibilityPublic)),
				task.HasProjectWith(project.HasMembersWith(projectmember.UserIDEQ(m.UserID))),
			),
		).
		WithAssignees().
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load member tasks"})
		return
	}

	byStatus := map[string]int{"not_started": 0, "in_progress": 0, "done": 0, "on_hold": 0}
	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		byStatus[string(t.Status)]++
		out = append(out, taskJSON(t))
	}

	var memberName, memberEmail string
	if target.Edges.User != nil {
		memberName = target.Edges.User.Name
		memberEmail = target.Edges.User.Email
	}

	c.JSON(http.StatusOK, gin.H{
		"member": gin.H{
			"id":      target.ID,
			"user_id": target.UserID,
			"name":    memberName,
			"email":   memberEmail,
		},
		"tasks": out,
		"summary": gin.H{
			"total":     len(tasks),
			"by_status": byStatus,
		},
	})
}

// ensureNotLastOwner は、targetMemberIDを除いた残りOwner数が0なら
// errLastOwnerを返す（Owner保護ルール。spec.md 6章）。
func ensureNotLastOwner(ctx context.Context, tx *ent.Tx, workspaceID, targetMemberID uuid.UUID) error {
	remaining, err := tx.WorkspaceMember.Query().
		Where(
			workspacemember.WorkspaceIDEQ(workspaceID),
			workspacemember.RoleEQ(workspacemember.RoleOwner),
			workspacemember.IDNEQ(targetMemberID),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if remaining == 0 {
		return errLastOwner
	}
	return nil
}
