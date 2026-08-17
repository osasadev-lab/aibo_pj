package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// WorkspaceHandler は /workspaces 配下のエンドポイントを扱う。
type WorkspaceHandler struct {
	client *ent.Client
}

func NewWorkspaceHandler(client *ent.Client) *WorkspaceHandler {
	return &WorkspaceHandler{client: client}
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
