package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

const manualSyncTestJWTSecret = "manual-sync-test-secret"

// ManualCalendarSyncの対象日フィルタが、指定日当日中（時刻を問わず）のタスクを
// 拾うことを確認する（2026-08-27追加。等価比較のままだと午前0時ちょうど以外
// 一致しなくなるバグがあった）。Google Calendar APIへの実際の同期は
// リフレッシュトークンの復号に失敗する時点で止まる（ネットワーク呼び出しなし）ため、
// synced+failedの合計＝クエリにヒットしたタスク数として日境界の検証に使う。
func TestManualCalendarSync_DayBoundary(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().
		SetGoogleSub("sub-mcs").SetEmail("mcs@example.com").SetName("MCS User").
		SetCalendarSyncEnabled(true).
		SetGoogleRefreshToken("not-valid-ciphertext").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := client.Workspace.Create().SetName("ws").Save(context.Background())
	if err != nil {
		t.Fatalf("create workspace: %v", err)
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
	mustTask("today-morning", refDay.Add(9*time.Hour))
	mustTask("today-night", refDay.Add(23*time.Hour+30*time.Minute))
	mustTask("yesterday-night", refDay.Add(-30*time.Minute))
	mustTask("tomorrow-morning", refDay.AddDate(0, 0, 1).Add(9*time.Hour))

	tok, err := internalauth.IssueToken(manualSyncTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// tokenEncryptionKeyは32バイトあれば内容は任意（DecryptTokenが復号失敗で
	// エラーを返す経路を通したいだけで、Google APIへは到達しない）。
	h := &AuthHandler{client: client, tokenEncryptionKey: make([]byte, 32)}
	r.POST("/me/calendar-sync", middleware.RequireAuth(client, manualSyncTestJWTSecret), h.ManualCalendarSync)

	w := doJSON(t, r, http.MethodPost, "/me/calendar-sync", tok, map[string]any{"date": "2026-08-23"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var res struct {
		Synced int `json:"synced"`
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := res.Synced + res.Failed; got != 2 {
		t.Errorf("synced+failed = %d, want 2 (today-morning, today-night only)", got)
	}
}
