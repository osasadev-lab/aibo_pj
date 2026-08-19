package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/tag"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// TagHandler は /projects/:project_id/tags, /workspaces/:workspace_id/common-tags 配下を扱う。
// タグはプロジェクト専用（project_id非nil）かワークスペース共通（project_id nil）のいずれか。
type TagHandler struct {
	client *ent.Client
}

func NewTagHandler(client *ent.Client) *TagHandler {
	return &TagHandler{client: client}
}

func tagJSON(t *ent.Tag) gin.H {
	return gin.H{
		"id":         t.ID,
		"project_id": t.ProjectID,
		"name":       t.Name,
		"color":      t.Color,
	}
}

type tagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color" binding:"required"`
}

type updateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

// duplicateTagName はworkspaceID・projectID（nilなら共通タグ）の中で同名タグが
// 既に存在するかを確認する。PostgresのUNIQUE INDEXはNULLを区別しないため、
// project_idがnil同士の重複はDB制約で検出できず、アプリ層でのチェックが必須。
func (h *TagHandler) duplicateTagName(ctx context.Context, workspaceID uuid.UUID, projectID *uuid.UUID, name string, excludeID *uuid.UUID) (bool, error) {
	q := h.client.Tag.Query().Where(tag.WorkspaceIDEQ(workspaceID), tag.NameEQ(name))
	if projectID != nil {
		q = q.Where(tag.ProjectIDEQ(*projectID))
	} else {
		q = q.Where(tag.ProjectIDIsNil())
	}
	if excludeID != nil {
		q = q.Where(tag.IDNEQ(*excludeID))
	}
	return q.Exist(ctx)
}

// --- プロジェクト専用タグ ---

// ListProjectTags は GET /projects/:project_id/tags。権限ゲート無し（RequireProjectAccessで閲覧可否は担保済み）。
func (h *TagHandler) ListProjectTags(c *gin.Context) {
	p := middleware.CurrentProject(c)

	tags, err := h.client.Tag.Query().
		Where(tag.ProjectIDEQ(p.ID)).
		Order(tag.ByName()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tags"})
		return
	}

	out := make([]gin.H, 0, len(tags))
	for _, t := range tags {
		out = append(out, tagJSON(t))
	}
	c.JSON(http.StatusOK, out)
}

// CreateProjectTag は POST /projects/:project_id/tags。RequireProjectManager限定。
func (h *TagHandler) CreateProjectTag(c *gin.Context) {
	p := middleware.CurrentProject(c)

	var req tagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and color are required"})
		return
	}

	ctx := c.Request.Context()

	dup, err := h.duplicateTagName(ctx, p.WorkspaceID, &p.ID, req.Name, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check duplicate tag name"})
		return
	}
	if dup {
		c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate_tag_name"})
		return
	}

	created, err := h.client.Tag.Create().
		SetWorkspaceID(p.WorkspaceID).
		SetProjectID(p.ID).
		SetName(req.Name).
		SetColor(req.Color).
		Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tag"})
		return
	}
	c.JSON(http.StatusCreated, tagJSON(created))
}

// UpdateProjectTag は PATCH /projects/:project_id/tags/:tag_id。RequireProjectManager限定。
func (h *TagHandler) UpdateProjectTag(c *gin.Context) {
	p := middleware.CurrentProject(c)

	tagID, err := uuid.Parse(c.Param("tag_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	var req updateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.client.Tag.Query().
		Where(tag.IDEQ(tagID), tag.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	if req.Name != nil {
		dup, err := h.duplicateTagName(ctx, p.WorkspaceID, &p.ID, *req.Name, &existing.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check duplicate tag name"})
			return
		}
		if dup {
			c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate_tag_name"})
			return
		}
	}

	builder := h.client.Tag.UpdateOneID(existing.ID)
	if req.Name != nil {
		builder = builder.SetName(*req.Name)
	}
	if req.Color != nil {
		builder = builder.SetColor(*req.Color)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tag"})
		return
	}
	c.JSON(http.StatusOK, tagJSON(updated))
}

// DeleteProjectTag は DELETE /projects/:project_id/tags/:tag_id。RequireProjectManager限定。
func (h *TagHandler) DeleteProjectTag(c *gin.Context) {
	p := middleware.CurrentProject(c)

	tagID, err := uuid.Parse(c.Param("tag_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.client.Tag.Query().
		Where(tag.IDEQ(tagID), tag.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	if err := h.deleteTag(ctx, existing.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tag"})
		return
	}
	c.Status(http.StatusNoContent)
}

// --- ワークスペース共通タグ ---

// ListCommonTags は GET /workspaces/:workspace_id/common-tags。RequireWorkspaceMemberのみ
// （タスクへの付与候補として全メンバーが見える必要があるため、Owner限定にしない）。
func (h *TagHandler) ListCommonTags(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	tags, err := h.client.Tag.Query().
		Where(tag.WorkspaceIDEQ(m.WorkspaceID), tag.ProjectIDIsNil()).
		Order(tag.ByName()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list common tags"})
		return
	}

	out := make([]gin.H, 0, len(tags))
	for _, t := range tags {
		out = append(out, tagJSON(t))
	}
	c.JSON(http.StatusOK, out)
}

// CreateCommonTag は POST /workspaces/:workspace_id/common-tags。RequireOwner限定。
func (h *TagHandler) CreateCommonTag(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	var req tagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and color are required"})
		return
	}

	ctx := c.Request.Context()

	dup, err := h.duplicateTagName(ctx, m.WorkspaceID, nil, req.Name, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check duplicate tag name"})
		return
	}
	if dup {
		c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate_tag_name"})
		return
	}

	created, err := h.client.Tag.Create().
		SetWorkspaceID(m.WorkspaceID).
		SetName(req.Name).
		SetColor(req.Color).
		Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tag"})
		return
	}
	c.JSON(http.StatusCreated, tagJSON(created))
}

// UpdateCommonTag は PATCH /workspaces/:workspace_id/common-tags/:tag_id。RequireOwner限定。
func (h *TagHandler) UpdateCommonTag(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	tagID, err := uuid.Parse(c.Param("tag_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	var req updateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.client.Tag.Query().
		Where(tag.IDEQ(tagID), tag.WorkspaceIDEQ(m.WorkspaceID), tag.ProjectIDIsNil()).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	if req.Name != nil {
		dup, err := h.duplicateTagName(ctx, m.WorkspaceID, nil, *req.Name, &existing.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check duplicate tag name"})
			return
		}
		if dup {
			c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate_tag_name"})
			return
		}
	}

	builder := h.client.Tag.UpdateOneID(existing.ID)
	if req.Name != nil {
		builder = builder.SetName(*req.Name)
	}
	if req.Color != nil {
		builder = builder.SetColor(*req.Color)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tag"})
		return
	}
	c.JSON(http.StatusOK, tagJSON(updated))
}

// DeleteCommonTag は DELETE /workspaces/:workspace_id/common-tags/:tag_id。RequireOwner限定。
func (h *TagHandler) DeleteCommonTag(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	tagID, err := uuid.Parse(c.Param("tag_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.client.Tag.Query().
		Where(tag.IDEQ(tagID), tag.WorkspaceIDEQ(m.WorkspaceID), tag.ProjectIDIsNil()).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	if err := h.deleteTag(ctx, existing.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tag"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TagHandler) deleteTag(ctx context.Context, tagID uuid.UUID) error {
	return withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.TaskTag.Delete().Where(tasktag.TagIDEQ(tagID)).Exec(ctx); err != nil {
			return err
		}
		return tx.Tag.DeleteOneID(tagID).Exec(ctx)
	})
}
