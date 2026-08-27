package handler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/remindersend"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

// due_dateが時刻を持つようになったことで、CronHandler.processReminderの
// 「本日期限」「期限超過」判定がJSTの日境界で正しく動くことを確認する
// （2026-08-27追加、日時範囲対応）。

func createCronTestTask(t *testing.T, client *ent.Client, workspaceID, userID uuid.UUID, title string, due time.Time) {
	t.Helper()
	tk, err := client.Task.Create().
		SetWorkspaceID(workspaceID).
		SetTitle(title).
		SetCreatedBy(userID).
		SetDueDate(due).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create task %s: %v", title, err)
	}
	if _, err := client.TaskAssignee.Create().
		SetTaskID(tk.ID).SetUserID(userID).
		Save(context.Background()); err != nil {
		t.Fatalf("assign task %s: %v", title, err)
	}
}

func TestCronProcessReminder_DueToday_DayBoundary(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().SetGoogleSub("sub-cron").SetEmail("cron@example.com").SetName("Cron User").Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// 「今日」= 2026-08-23 JST。当日00:00〜23:59に期限のタスクは「本日期限」に
	// 含まれ、前日23:59・翌日00:01のタスクは含まれないことを確認する。
	today := time.Date(2026, 8, 23, 0, 0, 0, 0, jst)
	createCronTestTask(t, client, ws.ID, u.ID, "today-early", today.Add(1*time.Minute))
	createCronTestTask(t, client, ws.ID, u.ID, "today-late", today.Add(23*time.Hour+59*time.Minute))
	createCronTestTask(t, client, ws.ID, u.ID, "yesterday-late", today.Add(-1*time.Minute))
	createCronTestTask(t, client, ws.ID, u.ID, "tomorrow-early", today.AddDate(0, 0, 1).Add(1*time.Minute))

	h := &CronHandler{client: client, frontendURL: "https://example.test"}
	var pending []pushdelivery.Item
	created := h.processReminder(context.Background(), u, remindersend.TypeDueToday, today, &pending)
	if !created {
		t.Fatal("expected a due-today reminder notification to be created")
	}

	notifs, err := client.Notification.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query notifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifs))
	}
	payload := notifs[0].Payload
	count, _ := payload["task_count"].(float64)
	if int(count) != 2 {
		t.Errorf("due-today task_count = %v, want 2 (today-early, today-late only)", payload["task_count"])
	}
}

func TestCronProcessReminder_Overdue_DayBoundary(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().SetGoogleSub("sub-cron2").SetEmail("cron2@example.com").SetName("Cron User 2").Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	today := time.Date(2026, 8, 23, 0, 0, 0, 0, jst)
	createCronTestTask(t, client, ws.ID, u.ID, "yesterday-late", today.Add(-1*time.Minute))
	createCronTestTask(t, client, ws.ID, u.ID, "today-early", today.Add(1*time.Minute))

	h := &CronHandler{client: client, frontendURL: "https://example.test"}
	var pending []pushdelivery.Item
	created := h.processReminder(context.Background(), u, remindersend.TypeOverdue, today, &pending)
	if !created {
		t.Fatal("expected an overdue reminder notification to be created")
	}

	notifs, err := client.Notification.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query notifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifs))
	}
	count, _ := notifs[0].Payload["task_count"].(float64)
	// 今日00:01期限のタスクは「本日期限」であって「超過」ではないため含まれない。
	if int(count) != 1 {
		t.Errorf("overdue task_count = %v, want 1 (yesterday-late only)", notifs[0].Payload["task_count"])
	}
}
