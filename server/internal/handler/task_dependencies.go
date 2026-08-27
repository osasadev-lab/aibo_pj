package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskdependency"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

func dependencyTaskJSON(t *ent.Task) gin.H {
	return gin.H{"id": t.ID, "title": t.Title, "status": t.Status, "project_id": t.ProjectID}
}

// ListDependencies は GET /tasks/:task_id/dependencies。
// predecessors=このタスクが依存している先行タスク、successors=このタスクに
// 依存している後続タスク。
func (h *TaskHandler) ListDependencies(c *gin.Context) {
	t := middleware.CurrentTask(c)
	ctx := c.Request.Context()

	preds, err := h.client.TaskDependency.Query().
		Where(taskdependency.TaskIDEQ(t.ID)).
		WithDependsOn().
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dependencies"})
		return
	}
	succs, err := h.client.TaskDependency.Query().
		Where(taskdependency.DependsOnTaskIDEQ(t.ID)).
		WithTask().
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dependencies"})
		return
	}

	predecessors := make([]gin.H, 0, len(preds))
	for _, d := range preds {
		if d.Edges.DependsOn == nil {
			continue
		}
		predecessors = append(predecessors, gin.H{"id": d.ID, "task": dependencyTaskJSON(d.Edges.DependsOn)})
	}
	successors := make([]gin.H, 0, len(succs))
	for _, d := range succs {
		if d.Edges.Task == nil {
			continue
		}
		successors = append(successors, gin.H{"id": d.ID, "task": dependencyTaskJSON(d.Edges.Task)})
	}
	c.JSON(http.StatusOK, gin.H{"predecessors": predecessors, "successors": successors})
}

// wouldCreateCycle は、taskIDがdependsOnTaskIDに依存する辺を追加した場合に
// 循環依存が生じるかを判定する。dependsOnTaskIDを起点に既存のdepends_on辺を
// 辿って（＝depends_on_task_idがさらに依存している先行タスクを辿って）taskIDに
// 到達できれば、新しい辺を足すと閉路になる。
func (h *TaskHandler) wouldCreateCycle(ctx context.Context, taskID, dependsOnTaskID uuid.UUID) (bool, error) {
	visited := map[uuid.UUID]struct{}{}
	queue := []uuid.UUID{dependsOnTaskID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == taskID {
			return true, nil
		}
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}

		preds, err := h.client.TaskDependency.Query().Where(taskdependency.TaskIDEQ(current)).All(ctx)
		if err != nil {
			return false, err
		}
		for _, d := range preds {
			queue = append(queue, d.DependsOnTaskID)
		}
	}
	return false, nil
}

type createDependencyRequest struct {
	DependsOnTaskID uuid.UUID `json:"depends_on_task_id" binding:"required"`
}

// CreateDependency は POST /tasks/:task_id/dependencies。先行タスクを追加する。
// 自己参照・ワークスペース跨ぎ・循環依存を拒否する。
func (h *TaskHandler) CreateDependency(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req createDependencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "depends_on_task_id is required"})
		return
	}
	if req.DependsOnTaskID == t.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task cannot depend on itself"})
		return
	}

	ctx := c.Request.Context()

	target, err := h.client.Task.Get(ctx, req.DependsOnTaskID)
	if err != nil || target.WorkspaceID != t.WorkspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "depends_on_task_id must be in the same workspace"})
		return
	}

	cyclic, err := h.wouldCreateCycle(ctx, t.ID, req.DependsOnTaskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check circular dependency"})
		return
	}
	if cyclic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "circular_dependency"})
		return
	}

	var created *ent.TaskDependency
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		var txErr error
		created, txErr = tx.TaskDependency.Create().
			SetTaskID(t.ID).
			SetDependsOnTaskID(req.DependsOnTaskID).
			Save(ctx)
		if txErr != nil {
			return txErr
		}
		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.dependency_added",
			map[string]any{"depends_on_task_id": req.DependsOnTaskID})
	})
	if ent.IsConstraintError(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "already_depends_on"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create dependency"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": created.ID, "task": dependencyTaskJSON(target)})
}

// DeleteDependency は DELETE /tasks/:task_id/dependencies/:dependency_id。
// このタスクの先行タスク一覧からの解除のみを扱う（＝task_idがこのタスクである行に限定）。
func (h *TaskHandler) DeleteDependency(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	depID, err := uuid.Parse(c.Param("dependency_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dependency not found"})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.client.TaskDependency.Query().
		Where(taskdependency.IDEQ(depID), taskdependency.TaskIDEQ(t.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dependency not found"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if err := tx.TaskDependency.DeleteOneID(existing.ID).Exec(ctx); err != nil {
			return err
		}
		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.dependency_removed",
			map[string]any{"depends_on_task_id": existing.DependsOnTaskID})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete dependency"})
		return
	}
	c.Status(http.StatusNoContent)
}
