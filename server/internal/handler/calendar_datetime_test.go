package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

const calendarDateTimeTestJWTSecret = "calendar-datetime-test-secret"

// GetCalendarの月範囲判定がJSTの月境界で行われることを確認する
// （2026-08-27追加。UTC基準のままだと月初・月末付近のタスクが隣の月に
// 漏れる/消えるバグがあった）。

func TestGetCalendar_MonthBoundary_JST(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().SetGoogleSub("sub-cal").SetEmail("cal@example.com").SetName("Cal User").Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := client.WorkspaceMember.Create().
		SetWorkspaceID(ws.ID).SetUserID(u.ID).SetRole(workspacemember.RoleOwner).
		Save(context.Background()); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	mustTask := func(title string, due time.Time) {
		tk, err := client.Task.Create().
			SetWorkspaceID(ws.ID).SetTitle(title).SetCreatedBy(u.ID).SetDueDate(due).
			Save(context.Background())
		if err != nil {
			t.Fatalf("create task %s: %v", title, err)
		}
		if _, err := client.TaskAssignee.Create().SetTaskID(tk.ID).SetUserID(u.ID).Save(context.Background()); err != nil {
			t.Fatalf("assign task %s: %v", title, err)
		}
	}

	// 8月の最終日23:59 JSTは8月に含まれ、9月1日0:30 JSTは9月（8月には含まれない）。
	mustTask("aug-last-late", time.Date(2026, 8, 31, 23, 59, 0, 0, jst))
	mustTask("sep-first-early", time.Date(2026, 9, 1, 0, 30, 0, 0, jst))

	tok, err := internalauth.IssueToken(calendarDateTimeTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewCalendarHandler(client)
	requireAuth := middleware.RequireAuth(client, calendarDateTimeTestJWTSecret)
	r.GET("/workspaces/:workspace_id/calendar",
		requireAuth, middleware.RequireWorkspaceMember(client), h.GetCalendar)

	w := doGet(t, r, "/workspaces/"+ws.ID.String()+"/calendar?month=2026-08", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var augTasks []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &augTasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(augTasks) != 1 || augTasks[0]["title"] != "aug-last-late" {
		t.Errorf("2026-08 tasks = %v, want exactly [aug-last-late]", titlesOf(augTasks))
	}

	w = doGet(t, r, "/workspaces/"+ws.ID.String()+"/calendar?month=2026-09", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var sepTasks []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sepTasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sepTasks) != 1 || sepTasks[0]["title"] != "sep-first-early" {
		t.Errorf("2026-09 tasks = %v, want exactly [sep-first-early]", titlesOf(sepTasks))
	}
}

// GetCalendarが、期限（due_date）が無く開始日時（start_date）のみのタスクも
// 開始日時を基準にその月に含めることを確認する（2026-08-27追加、「日次範囲が
// 片方だけでも表示してほしい」との要望）。期限が設定されているタスクは
// 引き続き期限を優先する。
func TestGetCalendar_StartDateOnlyTask_IncludedByStartDate(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().SetGoogleSub("sub-cal2").SetEmail("cal2@example.com").SetName("Cal User 2").Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := client.WorkspaceMember.Create().
		SetWorkspaceID(ws.ID).SetUserID(u.ID).SetRole(workspacemember.RoleOwner).
		Save(context.Background()); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	mustStartOnlyTask := func(title string, start time.Time) {
		tk, err := client.Task.Create().
			SetWorkspaceID(ws.ID).SetTitle(title).SetCreatedBy(u.ID).SetStartDate(start).
			Save(context.Background())
		if err != nil {
			t.Fatalf("create task %s: %v", title, err)
		}
		if _, err := client.TaskAssignee.Create().SetTaskID(tk.ID).SetUserID(u.ID).Save(context.Background()); err != nil {
			t.Fatalf("assign task %s: %v", title, err)
		}
	}
	mustBothTask := func(title string, start, due time.Time) {
		tk, err := client.Task.Create().
			SetWorkspaceID(ws.ID).SetTitle(title).SetCreatedBy(u.ID).SetStartDate(start).SetDueDate(due).
			Save(context.Background())
		if err != nil {
			t.Fatalf("create task %s: %v", title, err)
		}
		if _, err := client.TaskAssignee.Create().SetTaskID(tk.ID).SetUserID(u.ID).Save(context.Background()); err != nil {
			t.Fatalf("assign task %s: %v", title, err)
		}
	}

	mustStartOnlyTask("start-only-in-august", time.Date(2026, 8, 15, 9, 0, 0, 0, jst))
	// 期限が9月にあるタスクは、開始日時が8月でも期限（9月）優先で8月には含まれない。
	mustBothTask("due-takes-priority", time.Date(2026, 8, 20, 9, 0, 0, 0, jst), time.Date(2026, 9, 5, 9, 0, 0, 0, jst))

	tok, err := internalauth.IssueToken(calendarDateTimeTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewCalendarHandler(client)
	requireAuth := middleware.RequireAuth(client, calendarDateTimeTestJWTSecret)
	r.GET("/workspaces/:workspace_id/calendar",
		requireAuth, middleware.RequireWorkspaceMember(client), h.GetCalendar)

	w := doGet(t, r, "/workspaces/"+ws.ID.String()+"/calendar?month=2026-08", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var augTasks []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &augTasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(augTasks) != 1 || augTasks[0]["title"] != "start-only-in-august" {
		t.Errorf("2026-08 tasks = %v, want exactly [start-only-in-august]", titlesOf(augTasks))
	}

	w = doGet(t, r, "/workspaces/"+ws.ID.String()+"/calendar?month=2026-09", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var sepTasks []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sepTasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sepTasks) != 1 || sepTasks[0]["title"] != "due-takes-priority" {
		t.Errorf("2026-09 tasks = %v, want exactly [due-takes-priority]", titlesOf(sepTasks))
	}
}

func titlesOf(tasks []map[string]any) []string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		title, _ := t["title"].(string)
		out = append(out, title)
	}
	return out
}

// MyTasksの?date=基準日フィルタが、基準日当日中（時刻を問わず）を含むことを
// 確認する（2026-08-27追加。LTEのままだと基準日当日の午後締切を取りこぼす
// バグがあった）。
func TestMyTasks_DateFilter_IncludesWholeReferenceDay(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().SetGoogleSub("sub-mt").SetEmail("mt@example.com").SetName("MT User").Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := client.WorkspaceMember.Create().
		SetWorkspaceID(ws.ID).SetUserID(u.ID).SetRole(workspacemember.RoleOwner).
		Save(context.Background()); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	mustTask := func(title string, due time.Time) {
		tk, err := client.Task.Create().
			SetWorkspaceID(ws.ID).SetTitle(title).SetCreatedBy(u.ID).SetDueDate(due).
			Save(context.Background())
		if err != nil {
			t.Fatalf("create task %s: %v", title, err)
		}
		if _, err := client.TaskAssignee.Create().SetTaskID(tk.ID).SetUserID(u.ID).Save(context.Background()); err != nil {
			t.Fatalf("assign task %s: %v", title, err)
		}
	}

	refDay := time.Date(2026, 8, 23, 0, 0, 0, 0, jst)
	mustTask("today-afternoon", refDay.Add(18*time.Hour))
	mustTask("tomorrow-morning", refDay.AddDate(0, 0, 1).Add(1*time.Hour))

	tok, err := internalauth.IssueToken(calendarDateTimeTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTaskHandler(client, nil, nil, nil, "", pushdelivery.Config{})
	requireAuth := middleware.RequireAuth(client, calendarDateTimeTestJWTSecret)
	r.GET("/workspaces/:workspace_id/my-tasks",
		requireAuth, middleware.RequireWorkspaceMember(client), h.MyTasks)

	w := doGet(t, r, "/workspaces/"+ws.ID.String()+"/my-tasks?date=2026-08-23", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var tasks []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 || tasks[0]["title"] != "today-afternoon" {
		t.Errorf("date=2026-08-23 tasks = %v, want exactly [today-afternoon]", titlesOf(tasks))
	}
	if dueToday, _ := tasks[0]["due_today"].(bool); !dueToday {
		t.Errorf("today-afternoon due_today = %v, want true", tasks[0]["due_today"])
	}
}

// MyTasksの?date=基準日フィルタが、期限（due_date）が無く開始日時（start_date）
// のみのタスクも開始日時を基準にヒットさせることを確認する（2026-08-27追加、
// 「日次範囲が片方だけでもヒットするように」との要望）。期限があるタスクは
// 引き続き期限を優先する。
func TestMyTasks_DateFilter_StartDateOnlyTaskMatchesByStartDate(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().SetGoogleSub("sub-mt2").SetEmail("mt2@example.com").SetName("MT User 2").Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := client.WorkspaceMember.Create().
		SetWorkspaceID(ws.ID).SetUserID(u.ID).SetRole(workspacemember.RoleOwner).
		Save(context.Background()); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	refDay := time.Date(2026, 8, 23, 0, 0, 0, 0, jst)

	mustStartOnlyTask := func(title string, start time.Time) {
		tk, err := client.Task.Create().
			SetWorkspaceID(ws.ID).SetTitle(title).SetCreatedBy(u.ID).SetStartDate(start).
			Save(context.Background())
		if err != nil {
			t.Fatalf("create task %s: %v", title, err)
		}
		if _, err := client.TaskAssignee.Create().SetTaskID(tk.ID).SetUserID(u.ID).Save(context.Background()); err != nil {
			t.Fatalf("assign task %s: %v", title, err)
		}
	}
	mustBothTask := func(title string, start, due time.Time) {
		tk, err := client.Task.Create().
			SetWorkspaceID(ws.ID).SetTitle(title).SetCreatedBy(u.ID).SetStartDate(start).SetDueDate(due).
			Save(context.Background())
		if err != nil {
			t.Fatalf("create task %s: %v", title, err)
		}
		if _, err := client.TaskAssignee.Create().SetTaskID(tk.ID).SetUserID(u.ID).Save(context.Background()); err != nil {
			t.Fatalf("assign task %s: %v", title, err)
		}
	}

	mustStartOnlyTask("start-only-today", refDay.Add(9*time.Hour))
	mustStartOnlyTask("start-only-tomorrow", refDay.AddDate(0, 0, 1).Add(1*time.Hour))
	// 開始日時は基準日だが期限が翌日にあるタスクは、期限優先のため基準日には含まれない。
	mustBothTask("due-takes-priority", refDay.Add(9*time.Hour), refDay.AddDate(0, 0, 1).Add(9*time.Hour))

	tok, err := internalauth.IssueToken(calendarDateTimeTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTaskHandler(client, nil, nil, nil, "", pushdelivery.Config{})
	requireAuth := middleware.RequireAuth(client, calendarDateTimeTestJWTSecret)
	r.GET("/workspaces/:workspace_id/my-tasks",
		requireAuth, middleware.RequireWorkspaceMember(client), h.MyTasks)

	w := doGet(t, r, "/workspaces/"+ws.ID.String()+"/my-tasks?date=2026-08-23", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var tasks []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 || tasks[0]["title"] != "start-only-today" {
		t.Errorf("date=2026-08-23 tasks = %v, want exactly [start-only-today]", titlesOf(tasks))
	}
	if dueToday, _ := tasks[0]["due_today"].(bool); !dueToday {
		t.Errorf("start-only-today due_today = %v, want true (falls back to start_date)", tasks[0]["due_today"])
	}
}
