package middleware

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
)

const ctxProjectKey = "project"

// ErrNotProjectMember はCheckProjectVisibilityがprivateプロジェクトへの
// 非参画メンバーからのアクセスを検出した際に返す。
var ErrNotProjectMember = errors.New("not a project member")

// RequireProjectAccess はパスの :project_id を解決し、呼び出しユーザーが
// そのプロジェクトを閲覧できるか検証する（:workspace_id を含まないパス用。
// RequireWorkspaceMemberとは別経路でworkspace所属を解決する点が異なる）。
// public projectはworkspaceメンバー全員、privateはproject_membersのみ閲覧可。
// RequireAuthの後段で使う。
func RequireProjectAccess(client *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		projectID, err := uuid.Parse(c.Param("project_id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}

		ctx := c.Request.Context()

		p, err := client.Project.Get(ctx, projectID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}

		membership, err := client.WorkspaceMember.Query().
			Where(
				workspacemember.WorkspaceIDEQ(p.WorkspaceID),
				workspacemember.UserIDEQ(u.ID),
			).
			Only(ctx)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a workspace member"})
			return
		}

		if err := CheckProjectVisibility(ctx, client, p, u.ID); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a project member"})
			return
		}

		c.Set(ctxMembershipKey, membership)
		c.Set(ctxProjectKey, p)
		c.Next()
	}
}

// CheckProjectVisibility はprojectがpublicか、privateならuserIDがproject_membersに
// 含まれるかを検証する。RequireProjectAccessと、RequireTaskAccess・タスク作成時の
// project_id検証など複数箇所から共有して使う。
func CheckProjectVisibility(ctx context.Context, client *ent.Client, p *ent.Project, userID uuid.UUID) error {
	if p.Visibility == project.VisibilityPublic {
		return nil
	}
	ok, err := client.ProjectMember.Query().
		Where(
			projectmember.ProjectIDEQ(p.ID),
			projectmember.UserIDEQ(userID),
		).
		Exist(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotProjectMember
	}
	return nil
}

// CurrentProject はRequireProjectAccessが格納した*ent.Projectを取り出す。
func CurrentProject(c *gin.Context) *ent.Project {
	v, ok := c.Get(ctxProjectKey)
	if !ok {
		return nil
	}
	p, _ := v.(*ent.Project)
	return p
}

// RequireProjectManager はRequireProjectAccessの後段で使い、呼び出しユーザーが
// そのプロジェクトのmanager（責任者）かworkspace Ownerであることを検証する
// （PJ名変更・メンバー管理・列操作・プロジェクト削除用。workspace Ownerは常に
// managerを上書きできる、というユーザー確認済みの方針）。
func RequireProjectManager(client *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		p := CurrentProject(c)
		m := CurrentMembership(c)
		if u == nil || p == nil || m == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		if m.Role == workspacemember.RoleOwner {
			c.Next()
			return
		}

		isManager, err := client.ProjectMember.Query().
			Where(
				projectmember.ProjectIDEQ(p.ID),
				projectmember.UserIDEQ(u.ID),
				projectmember.RoleEQ(projectmember.RoleManager),
			).
			Exist(c.Request.Context())
		if err != nil || !isManager {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "project manager required"})
			return
		}
		c.Next()
	}
}
