package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
)

const ctxMembershipKey = "membership"

// RequireWorkspaceMember はパスの :workspace_id に対する呼び出しユーザーのWorkspaceMemberを
// 検証し、contextへ格納する（RequireOwnerが再利用する）。RequireAuthの後段で使う。
func RequireWorkspaceMember(client *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		workspaceID, err := uuid.Parse(c.Param("workspace_id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}

		membership, err := client.WorkspaceMember.Query().
			Where(
				workspacemember.WorkspaceIDEQ(workspaceID),
				workspacemember.UserIDEQ(u.ID),
			).
			Only(c.Request.Context())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a workspace member"})
			return
		}

		c.Set(ctxMembershipKey, membership)
		c.Next()
	}
}

// RequireOwner はRequireWorkspaceMemberの後段で使い、呼び出しユーザーがOwnerロールか検証する。
func RequireOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		m := CurrentMembership(c)
		if m == nil || m.Role != workspacemember.RoleOwner {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "owner role required"})
			return
		}
		c.Next()
	}
}

// CurrentMembership はRequireWorkspaceMemberが格納した*ent.WorkspaceMemberを取り出す。
func CurrentMembership(c *gin.Context) *ent.WorkspaceMember {
	v, ok := c.Get(ctxMembershipKey)
	if !ok {
		return nil
	}
	m, _ := v.(*ent.WorkspaceMember)
	return m
}
