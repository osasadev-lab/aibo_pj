package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

// newWorkspaceTestRouter は、既にRequireAuthを通過してCurrentUserがcontextに
// 格納されている前提で、指定ユーザーとしてRequireWorkspaceMember以降の
// ミドルウェアを検証するためのルーターを組み立てる。
func newWorkspaceTestRouter(u *ent.User, handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	setUser := func(c *gin.Context) {
		if u != nil {
			c.Set(ctxUserKey, u)
		}
		c.Next()
	}
	all := append([]gin.HandlerFunc{setUser}, handlers...)
	all = append(all, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/workspaces/:workspace_id/protected", all...)
	return r
}

func mustCreateUser(t *testing.T, client *ent.Client, email string) *ent.User {
	t.Helper()
	u, err := client.User.Create().
		SetGoogleSub("sub-" + email).
		SetEmail(email).
		SetName(email).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func TestRequireWorkspaceMember_Unauthenticated(t *testing.T) {
	client := testutil.NewClient(t)
	r := newWorkspaceTestRouter(nil, RequireWorkspaceMember(client))

	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+ws.ID.String()+"/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireWorkspaceMember_InvalidWorkspaceID(t *testing.T) {
	client := testutil.NewClient(t)
	u := mustCreateUser(t, client, "a@example.com")
	r := newWorkspaceTestRouter(u, RequireWorkspaceMember(client))

	req := httptest.NewRequest(http.MethodGet, "/workspaces/not-a-uuid/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestRequireWorkspaceMember_NotAMember(t *testing.T) {
	client := testutil.NewClient(t)
	u := mustCreateUser(t, client, "outsider@example.com")
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	r := newWorkspaceTestRouter(u, RequireWorkspaceMember(client))

	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+ws.ID.String()+"/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireWorkspaceMember_MemberPassesThrough(t *testing.T) {
	client := testutil.NewClient(t)
	u := mustCreateUser(t, client, "member@example.com")
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := client.WorkspaceMember.Create().
		SetWorkspaceID(ws.ID).
		SetUserID(u.ID).
		SetRole(workspacemember.RoleMember).
		Save(context.Background()); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	r := newWorkspaceTestRouter(u, RequireWorkspaceMember(client))

	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+ws.ID.String()+"/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireOwner_MemberRoleForbidden(t *testing.T) {
	client := testutil.NewClient(t)
	u := mustCreateUser(t, client, "member2@example.com")
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := client.WorkspaceMember.Create().
		SetWorkspaceID(ws.ID).
		SetUserID(u.ID).
		SetRole(workspacemember.RoleMember).
		Save(context.Background()); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	r := newWorkspaceTestRouter(u, RequireWorkspaceMember(client), RequireOwner())

	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+ws.ID.String()+"/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireOwner_OwnerRolePassesThrough(t *testing.T) {
	client := testutil.NewClient(t)
	u := mustCreateUser(t, client, "owner@example.com")
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := client.WorkspaceMember.Create().
		SetWorkspaceID(ws.ID).
		SetUserID(u.ID).
		SetRole(workspacemember.RoleOwner).
		Save(context.Background()); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	r := newWorkspaceTestRouter(u, RequireWorkspaceMember(client), RequireOwner())

	req := httptest.NewRequest(http.MethodGet, "/workspaces/"+ws.ID.String()+"/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
