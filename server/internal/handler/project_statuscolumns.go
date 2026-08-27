package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectstatuscolumn"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

func statusColumnJSON(col *ent.ProjectStatusColumn) gin.H {
	return gin.H{
		"id":             col.ID,
		"project_id":     col.ProjectID,
		"name":           col.Name,
		"position":       col.Position,
		"maps_to_status": col.MapsToStatus,
		"is_default":     col.IsDefault,
	}
}

// ListStatusColumns は GET /projects/:project_id/status-columns。
func (h *ProjectHandler) ListStatusColumns(c *gin.Context) {
	p := middleware.CurrentProject(c)

	cols, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.ProjectIDEQ(p.ID)).
		Order(projectstatuscolumn.ByPosition()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list status columns"})
		return
	}

	out := make([]gin.H, 0, len(cols))
	for _, col := range cols {
		out = append(out, statusColumnJSON(col))
	}
	c.JSON(http.StatusOK, out)
}

type createStatusColumnRequest struct {
	Name         string `json:"name" binding:"required"`
	MapsToStatus string `json:"maps_to_status" binding:"required,oneof=not_started in_progress done on_hold"`
}

// CreateStatusColumn は POST /projects/:project_id/status-columns。
// プロジェクトあたり最大5列。
func (h *ProjectHandler) CreateStatusColumn(c *gin.Context) {
	p := middleware.CurrentProject(c)

	var req createStatusColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and maps_to_status are required"})
		return
	}

	ctx := c.Request.Context()

	count, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.ProjectIDEQ(p.ID)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count status columns"})
		return
	}
	if count >= maxStatusColumns {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_columns_exceeded"})
		return
	}

	col, err := h.client.ProjectStatusColumn.Create().
		SetProjectID(p.ID).
		SetName(req.Name).
		SetPosition(count).
		SetMapsToStatus(projectstatuscolumn.MapsToStatus(req.MapsToStatus)).
		Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create status column"})
		return
	}
	c.JSON(http.StatusCreated, statusColumnJSON(col))
}

type updateStatusColumnRequest struct {
	Name         *string `json:"name"`
	Position     *int    `json:"position"`
	MapsToStatus *string `json:"maps_to_status" binding:"omitempty,oneof=not_started in_progress done on_hold"`
}

// UpdateStatusColumn は PATCH /projects/:project_id/status-columns/:column_id。
func (h *ProjectHandler) UpdateStatusColumn(c *gin.Context) {
	p := middleware.CurrentProject(c)

	columnID, err := uuid.Parse(c.Param("column_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	var req updateStatusColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	col, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.IDEQ(columnID), projectstatuscolumn.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	builder := h.client.ProjectStatusColumn.UpdateOneID(col.ID)
	if req.Name != nil {
		builder = builder.SetName(*req.Name)
	}
	if req.Position != nil {
		builder = builder.SetPosition(*req.Position)
	}
	if req.MapsToStatus != nil {
		builder = builder.SetMapsToStatus(projectstatuscolumn.MapsToStatus(*req.MapsToStatus))
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status column"})
		return
	}
	c.JSON(http.StatusOK, statusColumnJSON(updated))
}

// DeleteStatusColumn は DELETE /projects/:project_id/status-columns/:column_id。
// 列にタスクが残っている場合、クエリパラメータtarget_column_idで移動先を
// 指定しない限り拒否する（db-schema.md「タスクが残っている場合は移動を強制」）。
func (h *ProjectHandler) DeleteStatusColumn(c *gin.Context) {
	p := middleware.CurrentProject(c)

	columnID, err := uuid.Parse(c.Param("column_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	ctx := c.Request.Context()

	col, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.IDEQ(columnID), projectstatuscolumn.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	if col.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "default_column_not_deletable"})
		return
	}

	taskCount, err := h.client.Task.Query().
		Where(task.StatusColumnIDEQ(col.ID)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count tasks"})
		return
	}

	if taskCount == 0 {
		if err := h.client.ProjectStatusColumn.DeleteOneID(col.ID).Exec(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete status column"})
			return
		}
		c.Status(http.StatusNoContent)
		return
	}

	targetIDStr := c.Query("target_column_id")
	if targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tasks_present", "task_count": taskCount})
		return
	}
	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_column_id"})
		return
	}
	target, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.IDEQ(targetID), projectstatuscolumn.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_column_id must belong to the same project"})
		return
	}
	if target.ID == col.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_column_id must differ from the deleted column"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.Task.Update().
			Where(task.StatusColumnIDEQ(col.ID)).
			SetStatusColumnID(target.ID).
			SetStatus(task.Status(target.MapsToStatus)).
			Save(ctx); err != nil {
			return err
		}
		return tx.ProjectStatusColumn.DeleteOneID(col.ID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete status column"})
		return
	}
	c.Status(http.StatusNoContent)
}
