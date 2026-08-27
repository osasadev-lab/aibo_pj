package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
	"github.com/osasadev-lab/aibo_pj/server/ent/taskcalendarevent"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskdependency"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskpin"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/calendarsync"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"

	"golang.org/x/oauth2"
)

// ProjectHandler は /projects, /workspaces/:workspace_id/projects 配下を扱う。
type ProjectHandler struct {
	client *ent.Client
	r2     *storage.R2Client
	// calCfg/encKeyはM6用。プロジェクト削除時、配下タスクのGoogleカレンダー
	// 連携イベントもベストエフォートで削除する。
	calCfg *oauth2.Config
	encKey []byte
	// pushCfg/frontendURLはM7（Web Push配信）用。
	pushCfg     pushdelivery.Config
	frontendURL string
}

func NewProjectHandler(client *ent.Client, r2 *storage.R2Client, calCfg *oauth2.Config, encKey []byte, pushCfg pushdelivery.Config, frontendURL string) *ProjectHandler {
	return &ProjectHandler{client: client, r2: r2, calCfg: calCfg, encKey: encKey, pushCfg: pushCfg, frontendURL: frontendURL}
}

// defaultStatusColumns はプロジェクト作成時に自動投入する既定4列（db-schema.md）。
// いずれも is_default=true として作成され、UIから削除できない（ユーザー確認済み）。
var defaultStatusColumns = []struct {
	name         string
	mapsToStatus projectstatuscolumn.MapsToStatus
}{
	{"未対応", projectstatuscolumn.MapsToStatusNotStarted},
	{"対応中", projectstatuscolumn.MapsToStatusInProgress},
	{"対応済", projectstatuscolumn.MapsToStatusDone},
	{"保留", projectstatuscolumn.MapsToStatusOnHold},
}

const maxStatusColumns = 5

// notifyProjectMembership はプロジェクトへの参画・除外をuserIDに通知する
// （project_members行の作成・削除に伴うイベント。role変更のみの場合は呼ばない）。
// pendingにWeb Push配信用のitemを追記する（呼び出し元がwithTx成功後にpushdelivery.Asyncへ渡す）。
func notifyProjectMembership(ctx context.Context, tx *ent.Tx, pending *[]pushdelivery.Item, frontendURL string, workspaceID, projectID uuid.UUID, projectName string, actorID uuid.UUID, actorName string, userID uuid.UUID, joined bool) error {
	notifType := "project_removed"
	if joined {
		notifType = "project_joined"
	}
	payload := map[string]any{
		"project_id":      projectID,
		"project_name":    projectName,
		"changed_by":      actorID,
		"changed_by_name": actorName,
	}
	if _, err := tx.Notification.Create().
		SetUserID(userID).
		SetType(notifType).
		SetPayload(payload).
		Save(ctx); err != nil {
		return err
	}
	*pending = append(*pending, pushdelivery.BuildItem(userID, frontendURL, workspaceID.String(), notifType, payload))
	return nil
}

// notifyProjectLifecycle はプロジェクト自体の作成・削除をuserIDに通知する
// （notifyProjectMembershipは「参画状態の変化」用で、こちらは「プロジェクトという
// モノ自体の生成・消滅」用。publicはワークスペース全員が対象、privateは
// 参画メンバー全員が対象。actor自身には呼び出し側で通知しない）。
func notifyProjectLifecycle(ctx context.Context, tx *ent.Tx, pending *[]pushdelivery.Item, frontendURL string, workspaceID, projectID uuid.UUID, projectName string, actorID uuid.UUID, actorName string, userID uuid.UUID, created bool) error {
	notifType := "project_deleted"
	if created {
		notifType = "project_created"
	}
	payload := map[string]any{
		"project_id":      projectID,
		"project_name":    projectName,
		"changed_by":      actorID,
		"changed_by_name": actorName,
	}
	if _, err := tx.Notification.Create().
		SetUserID(userID).
		SetType(notifType).
		SetPayload(payload).
		Save(ctx); err != nil {
		return err
	}
	*pending = append(*pending, pushdelivery.BuildItem(userID, frontendURL, workspaceID.String(), notifType, payload))
	return nil
}

func projectJSON(p *ent.Project) gin.H {
	return gin.H{
		"id":           p.ID,
		"workspace_id": p.WorkspaceID,
		"name":         p.Name,
		"description":  p.Description,
		"visibility":   p.Visibility,
		"created_by":   p.CreatedBy,
	}
}

// List は GET /workspaces/:workspace_id/projects。
// public全件 + privateは参画分のみを返す。各行にis_managerを含める
// （設定画面でOwner/責任者が管理対象プロジェクトを判定するため）。
func (h *ProjectHandler) List(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	projects, err := h.client.Project.Query().
		Where(
			project.WorkspaceIDEQ(m.WorkspaceID),
			project.Or(
				project.VisibilityEQ(project.VisibilityPublic),
				project.HasMembersWith(projectmember.UserIDEQ(m.UserID)),
			),
		).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}

	isOwner := m.Role == workspacemember.RoleOwner
	managedProjectIDs := map[uuid.UUID]struct{}{}
	if !isOwner && len(projects) > 0 {
		ids := make([]uuid.UUID, 0, len(projects))
		for _, p := range projects {
			ids = append(ids, p.ID)
		}
		rows, err := h.client.ProjectMember.Query().
			Where(
				projectmember.UserIDEQ(m.UserID),
				projectmember.RoleEQ(projectmember.RoleManager),
				projectmember.ProjectIDIn(ids...),
			).
			All(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve managed projects"})
			return
		}
		for _, r := range rows {
			managedProjectIDs[r.ProjectID] = struct{}{}
		}
	}

	out := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		row := projectJSON(p)
		_, managed := managedProjectIDs[p.ID]
		row["is_manager"] = isOwner || managed
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}

type createProjectRequest struct {
	Name        string      `json:"name" binding:"required"`
	Description *string     `json:"description"`
	Visibility  string      `json:"visibility" binding:"required,oneof=public private"`
	MemberIDs   []uuid.UUID `json:"member_ids"`
}

// Create は POST /workspaces/:workspace_id/projects。可視性指定必須。
// publicはワークスペース全員が閲覧できるため参画メンバーという概念が無く、
// 作成者のみをmanagerとして登録する（member_idsは無視する）。privateのみ、
// 指定されたmember_idsをstaffとして参画させる。既定4ステータス列を自動生成する。
func (h *ProjectHandler) Create(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, visibility, member_ids are required"})
		return
	}

	ctx := c.Request.Context()

	ids := []uuid.UUID{m.UserID}
	if req.Visibility == "private" {
		// member_idsが全てこのworkspaceのメンバーであることを検証する。
		memberSet := map[uuid.UUID]struct{}{m.UserID: {}}
		for _, id := range req.MemberIDs {
			memberSet[id] = struct{}{}
		}
		ids = make([]uuid.UUID, 0, len(memberSet))
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
	}

	var created *ent.Project
	var pendingPush []pushdelivery.Item
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Project.Create().
			SetWorkspaceID(m.WorkspaceID).
			SetName(req.Name).
			SetVisibility(project.Visibility(req.Visibility)).
			SetCreatedBy(m.UserID)
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
		}
		p, err := builder.Save(ctx)
		if err != nil {
			return err
		}

		memberBuilders := make([]*ent.ProjectMemberCreate, 0, len(ids))
		for _, uid := range ids {
			b := tx.ProjectMember.Create().SetProjectID(p.ID).SetUserID(uid)
			// 作成者は自動的に責任者（manager）にする。他はデフォルトのstaffのまま。
			if uid == m.UserID {
				b = b.SetRole(projectmember.RoleManager)
			}
			memberBuilders = append(memberBuilders, b)
		}
		if _, err := tx.ProjectMember.CreateBulk(memberBuilders...).Save(ctx); err != nil {
			return err
		}

		for _, uid := range ids {
			if uid == m.UserID {
				continue
			}
			if err := notifyProjectMembership(ctx, tx, &pendingPush, h.frontendURL, m.WorkspaceID, p.ID, p.Name, m.UserID, u.Name, uid, true); err != nil {
				return err
			}
		}

		// publicは参画メンバーという概念が無く前段のループが空振りするため、
		// 代わりにワークスペース全員（作成者以外）へプロジェクト作成を通知する
		// （publicは誰でも閲覧できるため、新設自体を全員に知らせる）。
		if req.Visibility == "public" {
			wms, err := tx.WorkspaceMember.Query().
				Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID)).
				All(ctx)
			if err != nil {
				return err
			}
			for _, wm := range wms {
				if wm.UserID == m.UserID {
					continue
				}
				if err := notifyProjectLifecycle(ctx, tx, &pendingPush, h.frontendURL, m.WorkspaceID, p.ID, p.Name, m.UserID, u.Name, wm.UserID, true); err != nil {
					return err
				}
			}
		}

		columnBuilders := make([]*ent.ProjectStatusColumnCreate, 0, len(defaultStatusColumns))
		for i, col := range defaultStatusColumns {
			columnBuilders = append(columnBuilders, tx.ProjectStatusColumn.Create().
				SetProjectID(p.ID).
				SetName(col.name).
				SetPosition(i).
				SetMapsToStatus(col.mapsToStatus).
				SetIsDefault(true))
		}
		if _, err := tx.ProjectStatusColumn.CreateBulk(columnBuilders...).Save(ctx); err != nil {
			return err
		}

		if err := activity.Record(ctx, tx, m.WorkspaceID, nil, &p.ID, m.UserID, "project.created",
			map[string]any{"name": p.Name, "visibility": string(p.Visibility)}); err != nil {
			return err
		}

		created = p
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	pushdelivery.Async(h.client, h.pushCfg, pendingPush)

	c.JSON(http.StatusCreated, projectJSON(created))
}

// Get は GET /projects/:project_id。RequireProjectAccessで可視性確認済み。
func (h *ProjectHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, projectJSON(middleware.CurrentProject(c)))
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Visibility  *string `json:"visibility" binding:"omitempty,oneof=public private"`
}

// Update は PATCH /projects/:project_id。RequireProjectManager限定
// （プロジェクト責任者かworkspace Ownerのみ名称・説明・可視性を変更できる）。
func (h *ProjectHandler) Update(c *gin.Context) {
	p := middleware.CurrentProject(c)
	u := middleware.CurrentUser(c)

	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()
	var updated *ent.Project
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Project.UpdateOneID(p.ID)
		changes := map[string]any{}
		if req.Name != nil {
			builder = builder.SetName(*req.Name)
			changes["name"] = *req.Name
		}
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
			changes["description"] = *req.Description
		}
		if req.Visibility != nil {
			builder = builder.SetVisibility(project.Visibility(*req.Visibility))
			changes["visibility"] = *req.Visibility
		}

		var err error
		updated, err = builder.Save(ctx)
		if err != nil {
			return err
		}
		return activity.Record(ctx, tx, p.WorkspaceID, nil, &p.ID, u.ID, "project.updated", changes)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update project"})
		return
	}
	c.JSON(http.StatusOK, projectJSON(updated))
}

// Delete は DELETE /projects/:project_id。RequireProjectManager限定。
// entのschemaにカスケード削除指定が無いため、関連テーブルを手動で正しい順に削除する。
// comments/activity_logsもtask_id・project_idを参照しているため、task.go Deleteと
// 同様に先に削除しておかないと外部キー制約違反になる。
func (h *ProjectHandler) Delete(c *gin.Context) {
	p := middleware.CurrentProject(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var attachmentKeys []string
	var calendarEventRefs []calendarsync.EventRef
	var pendingPush []pushdelivery.Item
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		taskIDs, err := tx.Task.Query().
			Where(task.ProjectIDEQ(p.ID)).
			IDs(ctx)
		if err != nil {
			return err
		}

		if len(taskIDs) > 0 {
			// Googleカレンダー連携イベント（M6）。Google側の実削除はコミット後に
			// ベストエフォートで行うため、ここでは参照だけ収集する（task.goのDeleteと
			// 同じ理由でDB行自体は先に消す必要がある）。
			calendarEvents, err := tx.TaskCalendarEvent.Query().Where(taskcalendarevent.TaskIDIn(taskIDs...)).WithUser().All(ctx)
			if err != nil {
				return err
			}
			for _, e := range calendarEvents {
				calendarEventRefs = append(calendarEventRefs, calendarsync.EventRef{User: e.Edges.User, GoogleEventID: e.GoogleEventID})
			}
			if _, err := tx.TaskCalendarEvent.Delete().Where(taskcalendarevent.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}

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
			if _, err := tx.TaskPin.Delete().Where(taskpin.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
			memoKeys, err := deleteTaskMemosForTasks(ctx, tx, taskIDs)
			if err != nil {
				return err
			}
			attachmentKeys = append(attachmentKeys, memoKeys...)
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
		// activity_logsはtask_id経由（このプロジェクトのタスク）とproject_id経由
		// （project.created/updated等）の両方で参照され得るため両方消す。
		if _, err := tx.ActivityLog.Delete().
			Where(activitylog.Or(
				activitylog.TaskIDIn(taskIDs...),
				activitylog.ProjectIDEQ(p.ID),
			)).
			Exec(ctx); err != nil {
			return err
		}
		if len(taskIDs) > 0 {
			if _, err := tx.Task.Delete().Where(task.IDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
		}

		if _, err := tx.Section.Delete().Where(section.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.ProjectStatusColumn.Delete().Where(projectstatuscolumn.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		// このプロジェクト専用タグ（project_id非nil）も削除する。
		// task_tagsはこのプロジェクトのタスクの分は既に上で削除済みだが、
		// タグ行自体はここで消す必要がある。
		projectTagIDs, err := tx.Tag.Query().Where(tag.ProjectIDEQ(p.ID)).IDs(ctx)
		if err != nil {
			return err
		}
		if len(projectTagIDs) > 0 {
			if _, err := tx.TaskTag.Delete().Where(tasktag.TagIDIn(projectTagIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.Tag.Delete().Where(tag.IDIn(projectTagIDs...)).Exec(ctx); err != nil {
				return err
			}
		}
		// project_members行を消す前に通知対象を確定しておく（private=参画メンバー
		// 全員、public=ワークスペース全員。ともにactor自身は除く）。
		var recipientIDs []uuid.UUID
		if p.Visibility == project.VisibilityPublic {
			wms, err := tx.WorkspaceMember.Query().
				Where(workspacemember.WorkspaceIDEQ(p.WorkspaceID)).
				All(ctx)
			if err != nil {
				return err
			}
			for _, wm := range wms {
				if wm.UserID != u.ID {
					recipientIDs = append(recipientIDs, wm.UserID)
				}
			}
		} else {
			pms, err := tx.ProjectMember.Query().Where(projectmember.ProjectIDEQ(p.ID)).All(ctx)
			if err != nil {
				return err
			}
			for _, pm := range pms {
				if pm.UserID != u.ID {
					recipientIDs = append(recipientIDs, pm.UserID)
				}
			}
		}

		if _, err := tx.ProjectMember.Delete().Where(projectmember.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		for _, uid := range recipientIDs {
			if err := notifyProjectLifecycle(ctx, tx, &pendingPush, h.frontendURL, p.WorkspaceID, p.ID, p.Name, u.ID, u.Name, uid, false); err != nil {
				return err
			}
		}
		// project.deleted自体は削除対象のproject_idを参照できない（削除後に外部キー
		// 違反になる）ため、project_idを付けずpayloadにだけ残す。ハイライト機能
		// （GET /workspaces/:id/activity、M5）は削除後のDB状態からこの行の可視性を
		// 判定できないため、可視性判定に必要な情報（visibility・private時のメンバー
		// 一覧）をpayloadにも複製しておく。recipientIDsは上でnotifyProjectLifecycle用に
		// 算出済み（actor自身は含まない）のものを再利用する。
		deletedPayload := map[string]any{"name": p.Name, "project_id": p.ID, "visibility": string(p.Visibility)}
		if p.Visibility == project.VisibilityPrivate {
			deletedPayload["member_user_ids"] = recipientIDs
		}
		if err := activity.Record(ctx, tx, p.WorkspaceID, nil, nil, u.ID, "project.deleted",
			deletedPayload); err != nil {
			return err
		}
		return tx.Project.DeleteOneID(p.ID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}
	deleteR2Objects(ctx, h.r2, attachmentKeys)
	// バックグラウンド化の理由はtask.goと同じ（体感速度対策、2026-08-21）。
	calendarsync.Async(func(ctx context.Context) {
		calendarsync.DeleteGoogleEventsOnly(ctx, h.calCfg, h.encKey, calendarEventRefs)
	})
	pushdelivery.Async(h.client, h.pushCfg, pendingPush)
	c.Status(http.StatusNoContent)
}

