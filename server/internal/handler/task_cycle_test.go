package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

// taskFixture は循環依存テスト用に、1ワークスペース内に指定数のタスクを作る。
func taskFixture(t *testing.T, client *ent.Client, n int) (uuid.UUID, []*ent.Task) {
	t.Helper()
	ctx := context.Background()

	u, err := client.User.Create().
		SetGoogleSub("sub").SetEmail("u@example.com").SetName("u").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(ctx)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	tasks := make([]*ent.Task, n)
	for i := range n {
		task, err := client.Task.Create().
			SetWorkspaceID(ws.ID).
			SetTitle("task").
			SetCreatedBy(u.ID).
			Save(ctx)
		if err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
		tasks[i] = task
	}
	return ws.ID, tasks
}

func mustCreateDependency(t *testing.T, client *ent.Client, taskID, dependsOnID uuid.UUID) {
	t.Helper()
	if _, err := client.TaskDependency.Create().
		SetTaskID(taskID).
		SetDependsOnTaskID(dependsOnID).
		Save(context.Background()); err != nil {
		t.Fatalf("create dependency: %v", err)
	}
}

func TestWouldCreateCycle_NoExistingDependencies(t *testing.T) {
	client := testutil.NewClient(t)
	_, tasks := taskFixture(t, client, 2)
	h := &TaskHandler{client: client}

	cyclic, err := h.wouldCreateCycle(context.Background(), tasks[0].ID, tasks[1].ID)
	if err != nil {
		t.Fatalf("wouldCreateCycle: %v", err)
	}
	if cyclic {
		t.Error("expected no cycle when no dependencies exist yet")
	}
}

func TestWouldCreateCycle_DirectCycle(t *testing.T) {
	client := testutil.NewClient(t)
	_, tasks := taskFixture(t, client, 2)
	a, b := tasks[0].ID, tasks[1].ID

	// A already depends on B. Adding "B depends on A" would close a 2-node cycle.
	mustCreateDependency(t, client, a, b)

	h := &TaskHandler{client: client}
	cyclic, err := h.wouldCreateCycle(context.Background(), b, a)
	if err != nil {
		t.Fatalf("wouldCreateCycle: %v", err)
	}
	if !cyclic {
		t.Error("expected a direct A->B, B->A cycle to be detected")
	}
}

func TestWouldCreateCycle_TransitiveCycle(t *testing.T) {
	client := testutil.NewClient(t)
	_, tasks := taskFixture(t, client, 3)
	a, b, c := tasks[0].ID, tasks[1].ID, tasks[2].ID

	// Chain: B depends on A, C depends on B (i.e. A -> B -> C in "must finish before" order).
	mustCreateDependency(t, client, b, a)
	mustCreateDependency(t, client, c, b)

	h := &TaskHandler{client: client}
	// Adding "A depends on C" would close the loop A -> B -> C -> A.
	cyclic, err := h.wouldCreateCycle(context.Background(), a, c)
	if err != nil {
		t.Fatalf("wouldCreateCycle: %v", err)
	}
	if !cyclic {
		t.Error("expected a transitive 3-node cycle to be detected")
	}
}

func TestWouldCreateCycle_UnrelatedTaskIsFine(t *testing.T) {
	client := testutil.NewClient(t)
	_, tasks := taskFixture(t, client, 4)
	a, b, c, d := tasks[0].ID, tasks[1].ID, tasks[2].ID, tasks[3].ID

	mustCreateDependency(t, client, b, a)
	mustCreateDependency(t, client, c, b)

	h := &TaskHandler{client: client}
	// D is unrelated to the A->B->C chain; depending on it should never be flagged as cyclic.
	cyclic, err := h.wouldCreateCycle(context.Background(), a, d)
	if err != nil {
		t.Fatalf("wouldCreateCycle: %v", err)
	}
	if cyclic {
		t.Error("expected no cycle when the new dependency target is unrelated")
	}
}

func TestWouldCreateCycle_SelfReference(t *testing.T) {
	client := testutil.NewClient(t)
	_, tasks := taskFixture(t, client, 1)
	a := tasks[0].ID

	h := &TaskHandler{client: client}
	cyclic, err := h.wouldCreateCycle(context.Background(), a, a)
	if err != nil {
		t.Fatalf("wouldCreateCycle: %v", err)
	}
	// CreateDependencyハンドラは自己参照をwouldCreateCycle呼び出し前に別途弾いているが
	// （task.go参照）、このメソッド単体としても自己参照はcycle=trueとして扱われることを
	// 確認しておく（防御的な二重チェックの妥当性を保証する）。
	if !cyclic {
		t.Error("expected self-reference to be treated as a cycle")
	}
}
