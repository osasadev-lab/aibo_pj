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
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/logging"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
)

// ListMembers は GET /projects/:project_id/members。
// publicプロジェクトはworkspace全員が閲覧できてしまうため、参画者一覧自体は
// 責任者（manager）かworkspace Ownerにのみ公開する。privateはRequireProjectAccessの
// 時点で参画者以外弾かれているため無条件で許可する（8番の可視性ルール）。
func (h *ProjectHandler) ListMembers(c *gin.Context) {
	p := middleware.CurrentProject(c)
	u := middleware.CurrentUser(c)
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	if p.Visibility == project.VisibilityPublic && m.Role != workspacemember.RoleOwner {
		isManager, err := h.client.ProjectMember.Query().
			Where(
				projectmember.ProjectIDEQ(p.ID),
				projectmember.UserIDEQ(u.ID),
				projectmember.RoleEQ(projectmember.RoleManager),
			).
			Exist(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check permission"})
			return
		}
		if !isManager {
			c.JSON(http.StatusForbidden, gin.H{"error": "project manager required"})
			return
		}
	}

	members, err := h.client.ProjectMember.Query().
		Where(projectmember.ProjectIDEQ(p.ID)).
		WithUser().
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
		return
	}

	out := make([]gin.H, 0, len(members))
	for _, pm := range members {
		out = append(out, gin.H{
			"id":         pm.ID,
			"user_id":    pm.Edges.User.ID,
			"name":       pm.Edges.User.Name,
			"email":      pm.Edges.User.Email,
			"avatar_url": pm.Edges.User.AvatarURL,
			"role":       pm.Role,
		})
	}
	c.JSON(http.StatusOK, out)
}

type putMembersRequest struct {
	MemberIDs []uuid.UUID `json:"member_ids" binding:"required"`
}

var errNoManager = errors.New("resulting member set has no manager")

// PutMembers は PUT /projects/:project_id/members。参画メンバーを入れ替える。
// publicはワークスペース全員が閲覧できるため参画という概念自体が無く、
// 責任者の付与はPutManagersで行う（本エンドポイントはprivate専用）。
// 既存メンバーのroleは維持し（新規追加分はデフォルトのstaff）、置き換え後の集合に
// managerが1人もいなくなる場合は拒否する（プロジェクトが無責任者状態になるのを防ぐ、
// ワークスペースOwnerの「最後の1人保護」と同じ考え方）。
func (h *ProjectHandler) PutMembers(c *gin.Context) {
	p := middleware.CurrentProject(c)
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	if p.Visibility == project.VisibilityPublic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_applicable_for_public_project"})
		return
	}

	var req putMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "member_ids is required"})
		return
	}

	ctx := c.Request.Context()

	memberSet := map[uuid.UUID]struct{}{}
	for _, id := range req.MemberIDs {
		memberSet[id] = struct{}{}
	}
	ids := make([]uuid.UUID, 0, len(memberSet))
	for id := range memberSet {
		ids = append(ids, id)
	}
	validCount, err := h.client.WorkspaceMember.Query().
		Where(
			workspacemember.WorkspaceIDEQ(m.WorkspaceID),
			workspacemember.UserIDIn(ids...),
		).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
		return
	}
	if validCount != len(ids) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "member_ids must all be workspace members"})
		return
	}

	var pendingPush []pushdelivery.Item
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		existing, err := tx.ProjectMember.Query().Where(projectmember.ProjectIDEQ(p.ID)).All(ctx)
		if err != nil {
			return err
		}
		roleByUser := make(map[uuid.UUID]projectmember.Role, len(existing))
		wasMember := make(map[uuid.UUID]struct{}, len(existing))
		for _, pm := range existing {
			roleByUser[pm.UserID] = pm.Role
			wasMember[pm.UserID] = struct{}{}
		}
		isMember := make(map[uuid.UUID]struct{}, len(ids))
		for _, uid := range ids {
			isMember[uid] = struct{}{}
		}

		managerCount := 0
		for _, uid := range ids {
			if roleByUser[uid] == projectmember.RoleManager {
				managerCount++
			}
		}
		if managerCount == 0 {
			return errNoManager
		}

		if _, err := tx.ProjectMember.Delete().Where(projectmember.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		builders := make([]*ent.ProjectMemberCreate, 0, len(ids))
		for _, uid := range ids {
			b := tx.ProjectMember.Create().SetProjectID(p.ID).SetUserID(uid)
			if roleByUser[uid] == projectmember.RoleManager {
				b = b.SetRole(projectmember.RoleManager)
			}
			builders = append(builders, b)
		}
		if len(builders) > 0 {
			if _, err := tx.ProjectMember.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}

		// 参画・除外の通知（新規追加・除外されたユーザーのみ。既存参画者のroleのみの
		// 変更はnotifyProjectMembershipを呼ばない＝参画状態自体は変わっていないため）。
		for _, uid := range ids {
			if _, was := wasMember[uid]; was {
				continue
			}
			if err := notifyProjectMembership(ctx, tx, &pendingPush, h.frontendURL, m.WorkspaceID, p.ID, p.Name, m.UserID, u.Name, uid, true); err != nil {
				return err
			}
		}
		for uid := range wasMember {
			if _, still := isMember[uid]; still {
				continue
			}
			if err := notifyProjectMembership(ctx, tx, &pendingPush, h.frontendURL, m.WorkspaceID, p.ID, p.Name, m.UserID, u.Name, uid, false); err != nil {
				return err
			}
		}
		return nil
	})

	switch {
	case err == errNoManager:
		c.JSON(http.StatusBadRequest, gin.H{"error": "at_least_one_manager_required"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update members"})
	default:
		logging.Action(c, "project.members_updated", map[string]any{"project_id": p.ID, "member_ids": ids})
		pushdelivery.Async(h.client, h.pushCfg, pendingPush)
		c.Status(http.StatusNoContent)
	}
}

type changeProjectMemberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=manager staff"`
}

var errLastManager = errors.New("last manager cannot be demoted")

// ChangeMemberRole は PATCH /projects/:project_id/members/:member_id。
// 責任者(manager)⇄スタッフ(staff)を切り替える。最後の1人のmanagerは降格できない。
// publicはPutManagers専用（staffという参画概念自体が無いため）。
func (h *ProjectHandler) ChangeMemberRole(c *gin.Context) {
	p := middleware.CurrentProject(c)

	if p.Visibility == project.VisibilityPublic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_applicable_for_public_project"})
		return
	}

	memberID, err := uuid.Parse(c.Param("member_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	var req changeProjectMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid role is required"})
		return
	}
	newRole := projectmember.Role(req.Role)

	ctx := c.Request.Context()
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		target, err := tx.ProjectMember.Query().
			Where(projectmember.IDEQ(memberID), projectmember.ProjectIDEQ(p.ID)).
			Only(ctx)
		if err != nil {
			return err
		}

		if target.Role == projectmember.RoleManager && newRole != projectmember.RoleManager {
			if err := ensureNotLastManager(ctx, tx, p.ID, target.ID); err != nil {
				return err
			}
		}

		_, err = tx.ProjectMember.UpdateOneID(target.ID).SetRole(newRole).Save(ctx)
		return err
	})

	switch {
	case err == errLastManager:
		c.JSON(http.StatusBadRequest, gin.H{"error": "last_manager"})
	case ent.IsNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change role"})
	default:
		logging.Action(c, "project.member_role_changed", map[string]any{
			"project_id": p.ID, "member_id": memberID, "new_role": req.Role,
		})
		c.Status(http.StatusNoContent)
	}
}

type putManagersRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" binding:"required"`
}

// PutManagers は PUT /projects/:project_id/managers。責任者(manager)の付与・剥奪。
// publicプロジェクトはワークスペース全員が閲覧できるため「参画」という概念自体が無く、
// 責任者を付与されたユーザーのみproject_membersにrole=managerとして記録する
// （剥奪時はその行ごと削除する）。privateは既存の参画メンバーのうち責任者権限だけを
// 入れ替える（剥奪してもstaffとして参画は維持する。参画自体の追加・削除はPutMembers）。
func (h *ProjectHandler) PutManagers(c *gin.Context) {
	p := middleware.CurrentProject(c)
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	var req putManagersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids is required"})
		return
	}

	ctx := c.Request.Context()
	ids := dedupUUIDs(req.UserIDs)
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at_least_one_manager_required"})
		return
	}

	validCount, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID), workspacemember.UserIDIn(ids...)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
		return
	}
	if validCount != len(ids) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must all be workspace members"})
		return
	}

	var pendingPush []pushdelivery.Item
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		existing, err := tx.ProjectMember.Query().Where(projectmember.ProjectIDEQ(p.ID)).All(ctx)
		if err != nil {
			return err
		}
		byUser := make(map[uuid.UUID]*ent.ProjectMember, len(existing))
		for _, pm := range existing {
			byUser[pm.UserID] = pm
		}
		newSet := make(map[uuid.UUID]struct{}, len(ids))
		for _, uid := range ids {
			newSet[uid] = struct{}{}
		}

		// 新たにmanagerにするユーザー：既存行があればrole更新、無ければ新規作成。
		// 新規作成の場合はproject_membersに初めて参画したことになるため通知する
		// （publicで新たに責任者を付与された、等）。
		for _, uid := range ids {
			if pm, ok := byUser[uid]; ok {
				if pm.Role != projectmember.RoleManager {
					if _, err := tx.ProjectMember.UpdateOneID(pm.ID).SetRole(projectmember.RoleManager).Save(ctx); err != nil {
						return err
					}
				}
				continue
			}
			if _, err := tx.ProjectMember.Create().
				SetProjectID(p.ID).
				SetUserID(uid).
				SetRole(projectmember.RoleManager).
				Save(ctx); err != nil {
				return err
			}
			if err := notifyProjectMembership(ctx, tx, &pendingPush, h.frontendURL, m.WorkspaceID, p.ID, p.Name, m.UserID, u.Name, uid, true); err != nil {
				return err
			}
		}

		// 外れたmanagerの後始末：publicは参画という概念自体が無いので行ごと削除
		// （＝参画から外れたことになるため通知する）、privateは参画は維持したまま
		// staffに降格するだけ（参画状態自体は変わらないため通知しない）。
		for _, pm := range existing {
			if pm.Role != projectmember.RoleManager {
				continue
			}
			if _, still := newSet[pm.UserID]; still {
				continue
			}
			if p.Visibility == project.VisibilityPublic {
				if err := tx.ProjectMember.DeleteOne(pm).Exec(ctx); err != nil {
					return err
				}
				if err := notifyProjectMembership(ctx, tx, &pendingPush, h.frontendURL, m.WorkspaceID, p.ID, p.Name, m.UserID, u.Name, pm.UserID, false); err != nil {
					return err
				}
			} else if _, err := tx.ProjectMember.UpdateOneID(pm.ID).SetRole(projectmember.RoleStaff).Save(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update managers"})
		return
	}
	logging.Action(c, "project.managers_updated", map[string]any{"project_id": p.ID, "manager_ids": ids})
	pushdelivery.Async(h.client, h.pushCfg, pendingPush)
	c.Status(http.StatusNoContent)
}

// ensureNotLastManager はmember.goのensureNotLastOwnerと同型のヘルパー。
// targetMemberIDを除いた残りmanager数が0ならerrLastManagerを返す。
func ensureNotLastManager(ctx context.Context, tx *ent.Tx, projectID, targetMemberID uuid.UUID) error {
	remaining, err := tx.ProjectMember.Query().
		Where(
			projectmember.ProjectIDEQ(projectID),
			projectmember.RoleEQ(projectmember.RoleManager),
			projectmember.IDNEQ(targetMemberID),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if remaining == 0 {
		return errLastManager
	}
	return nil
}
