package calendarsync

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

// resolveEventWindowは日時範囲対応（2026-08-27追加）でGoogleカレンダーイベントの
// 開始・終了時刻を決める純粋関数。終日イベントから時間指定イベントへの変更に伴い、
// start_date/due_dateどちらか一方のみ設定時のフォールバック（defaultEventDuration分
// 補う）を検証する。

func TestResolveEventWindow_BothDatesSet(t *testing.T) {
	start := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 8, 23, 18, 30, 0, 0, time.UTC)

	gotStart, gotEnd := resolveEventWindow(&start, &due)

	if !gotEnd.Equal(due) {
		t.Errorf("end = %v, want %v", gotEnd, due)
	}
	if !gotStart.Equal(start) {
		t.Errorf("start = %v, want %v (explicit start_date should be used as-is)", gotStart, start)
	}
}

func TestResolveEventWindow_OnlyDueDateSet(t *testing.T) {
	due := time.Date(2026, 8, 23, 18, 30, 0, 0, time.UTC)

	gotStart, gotEnd := resolveEventWindow(nil, &due)

	if !gotEnd.Equal(due) {
		t.Errorf("end = %v, want %v", gotEnd, due)
	}
	wantStart := due.Add(-defaultEventDuration)
	if !gotStart.Equal(wantStart) {
		t.Errorf("start = %v, want %v (due_date - defaultEventDuration)", gotStart, wantStart)
	}
	if !gotStart.Before(gotEnd) {
		t.Errorf("start (%v) should be before end (%v)", gotStart, gotEnd)
	}
}

func TestResolveEventWindow_OnlyStartDateSet(t *testing.T) {
	start := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)

	gotStart, gotEnd := resolveEventWindow(&start, nil)

	if !gotStart.Equal(start) {
		t.Errorf("start = %v, want %v", gotStart, start)
	}
	wantEnd := start.Add(defaultEventDuration)
	if !gotEnd.Equal(wantEnd) {
		t.Errorf("end = %v, want %v (start_date + defaultEventDuration)", gotEnd, wantEnd)
	}
	if !gotStart.Before(gotEnd) {
		t.Errorf("start (%v) should be before end (%v)", gotStart, gotEnd)
	}
}

// eventDescriptionは開始日時・期限のどちらか一方しか設定されていないタスクについて、
// Googleカレンダーの表示時間帯が仮の予定枠であることを説明欄に明記する
// （2026-08-27追加、ユーザー要望：「startだけの場合はその旨を、dueだけの場合は
// その旨をGoogleカレンダーに記載して」）。

func mustCreateSyncTestTask(t *testing.T, client *ent.Client, start, due *time.Time) *ent.Task {
	t.Helper()
	ctx := context.Background()
	u, err := client.User.Create().SetGoogleSub("sub-desc").SetEmail("desc@example.com").SetName("Desc User").Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(ctx)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	builder := client.Task.Create().SetWorkspaceID(ws.ID).SetTitle("task").SetCreatedBy(u.ID)
	if start != nil {
		builder = builder.SetStartDate(*start)
	}
	if due != nil {
		builder = builder.SetDueDate(*due)
	}
	tk, err := builder.Save(ctx)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return tk
}

func TestEventDescription_OnlyStartDateSet_NotesMissingDueDate(t *testing.T) {
	client := testutil.NewClient(t)
	start := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	tk := mustCreateSyncTestTask(t, client, &start, nil)

	desc := eventDescription(context.Background(), client, tk, "https://example.test")

	if !strings.Contains(desc, "期限は未設定") {
		t.Errorf("description = %q, want a note that due_date is unset", desc)
	}
}

func TestEventDescription_OnlyDueDateSet_NotesMissingStartDate(t *testing.T) {
	client := testutil.NewClient(t)
	due := time.Date(2026, 8, 23, 18, 30, 0, 0, time.UTC)
	tk := mustCreateSyncTestTask(t, client, nil, &due)

	desc := eventDescription(context.Background(), client, tk, "https://example.test")

	if !strings.Contains(desc, "開始日時は未設定") {
		t.Errorf("description = %q, want a note that start_date is unset", desc)
	}
}

func TestEventDescription_BothDatesSet_NoNote(t *testing.T) {
	client := testutil.NewClient(t)
	start := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 8, 23, 18, 30, 0, 0, time.UTC)
	tk := mustCreateSyncTestTask(t, client, &start, &due)

	desc := eventDescription(context.Background(), client, tk, "https://example.test")

	if strings.Contains(desc, "未設定") {
		t.Errorf("description = %q, should not contain a missing-date note when both are set", desc)
	}
}

func TestResolveEventWindow_ResultIsJST(t *testing.T) {
	due := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC) // = 2026-08-23 18:00 JST

	_, gotEnd := resolveEventWindow(nil, &due)

	if _, offset := gotEnd.Zone(); offset != 9*60*60 {
		t.Errorf("end zone offset = %d, want +9h (JST)", offset)
	}
	if gotEnd.Hour() != 18 {
		t.Errorf("end hour = %d, want 18 (JST-converted)", gotEnd.Hour())
	}
}
