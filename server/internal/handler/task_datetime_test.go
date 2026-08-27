package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

// --- parseDateTimeInput / validateDateRange の単体テスト ---

func TestParseDateTimeInput(t *testing.T) {
	t.Run("nil is not provided", func(t *testing.T) {
		got, err := parseDateTimeInput(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.provided {
			t.Errorf("provided = true, want false")
		}
	})

	t.Run("empty string means clear", func(t *testing.T) {
		s := ""
		got, err := parseDateTimeInput(&s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.provided || !got.clear {
			t.Errorf("got = %+v, want provided=true clear=true", got)
		}
	})

	t.Run("valid value parses as JST", func(t *testing.T) {
		s := "2026-08-23 14:30"
		got, err := parseDateTimeInput(&s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.provided || got.clear {
			t.Fatalf("got = %+v, want provided=true clear=false", got)
		}
		if _, offset := got.value.Zone(); offset != 9*60*60 {
			t.Errorf("offset = %d, want +9h (JST)", offset)
		}
		if got.value.Hour() != 14 || got.value.Minute() != 30 {
			t.Errorf("value = %v, want 14:30", got.value)
		}
	})

	t.Run("invalid format is rejected", func(t *testing.T) {
		s := "2026-08-23"
		if _, err := parseDateTimeInput(&s); err == nil {
			t.Error("expected error for date-only (no time) input, got nil")
		}
	})
}

func TestValidateDateRange(t *testing.T) {
	mustParse := func(s string) dateTimeInput {
		v, err := parseDateTimeInput(&s)
		if err != nil {
			t.Fatalf("parseDateTimeInput(%q): %v", s, err)
		}
		return v
	}

	t.Run("start before due is fine", func(t *testing.T) {
		if err := validateDateRange(mustParse("2026-08-23 09:00"), mustParse("2026-08-23 18:00")); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("start after due is rejected", func(t *testing.T) {
		if err := validateDateRange(mustParse("2026-08-23 20:00"), mustParse("2026-08-23 09:00")); err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("only one side provided is fine", func(t *testing.T) {
		empty := ""
		notProvided, _ := parseDateTimeInput(nil)
		clearInput, _ := parseDateTimeInput(&empty)
		if err := validateDateRange(notProvided, mustParse("2026-08-23 09:00")); err != nil {
			t.Errorf("unexpected error (start not provided): %v", err)
		}
		if err := validateDateRange(clearInput, mustParse("2026-08-23 09:00")); err != nil {
			t.Errorf("unexpected error (start cleared): %v", err)
		}
	})
}

// --- HTTPレベル：Create/Update経由での日時範囲の設定・クリア・バリデーション ---

const dateTimeTestJWTSecret = "datetime-test-secret"

func setupDateTimeTestRouter(t *testing.T) (r *gin.Engine, token string, workspaceID string) {
	t.Helper()
	client := testutil.NewClient(t)

	u, err := client.User.Create().
		SetGoogleSub("sub-dt").SetEmail("dt@example.com").SetName("DT User").
		Save(context.Background())
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

	tok, err := internalauth.IssueToken(dateTimeTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r = gin.New()
	h := NewTaskHandler(client, nil, nil, nil, "", pushdelivery.Config{})
	requireAuth := middleware.RequireAuth(client, dateTimeTestJWTSecret)

	workspaces := r.Group("/workspaces/:workspace_id", requireAuth, middleware.RequireWorkspaceMember(client))
	workspaces.POST("/tasks", h.Create)

	tasks := r.Group("/tasks", requireAuth)
	withTask := tasks.Group("/:task_id", middleware.RequireTaskAccess(client))
	withTask.GET("", h.Get)
	withTask.PATCH("", h.Update)

	return r, tok, ws.ID.String()
}

func doJSON(t *testing.T, r *gin.Engine, method, path, token string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(b))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTaskCreate_DateTimeRange(t *testing.T) {
	r, token, wsID := setupDateTimeTestRouter(t)

	w := doJSON(t, r, http.MethodPost, "/workspaces/"+wsID+"/tasks", token, map[string]any{
		"title":      "range task",
		"start_date": "2026-08-23 09:00",
		"due_date":   "2026-08-23 18:30",
	})
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created["start_date"] != "2026-08-23 09:00" {
		t.Errorf("start_date = %v, want 2026-08-23 09:00", created["start_date"])
	}
	if created["due_date"] != "2026-08-23 18:30" {
		t.Errorf("due_date = %v, want 2026-08-23 18:30", created["due_date"])
	}
}

func TestTaskCreate_DateTimeRange_InvalidRange(t *testing.T) {
	r, token, wsID := setupDateTimeTestRouter(t)

	w := doJSON(t, r, http.MethodPost, "/workspaces/"+wsID+"/tasks", token, map[string]any{
		"title":      "backwards range",
		"start_date": "2026-08-23 20:00",
		"due_date":   "2026-08-23 09:00",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400", w.Code, w.Body.String())
	}
}

func TestTaskUpdate_DateTimeRange_SetThenClear(t *testing.T) {
	r, token, wsID := setupDateTimeTestRouter(t)

	w := doJSON(t, r, http.MethodPost, "/workspaces/"+wsID+"/tasks", token, map[string]any{"title": "t"})
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	taskID, _ := created["id"].(string)
	if taskID == "" {
		t.Fatal("could not extract created task id")
	}

	// 設定
	w = doJSON(t, r, http.MethodPatch, "/tasks/"+taskID, token, map[string]any{
		"start_date": "2026-08-23 09:00",
		"due_date":   "2026-08-23 18:30",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch(set) status = %d, body = %s", w.Code, w.Body.String())
	}

	// 再取得して保持されていることを確認。
	w = doJSON(t, r, http.MethodGet, "/tasks/"+taskID, token, nil)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if got["start_date"] != "2026-08-23 09:00" || got["due_date"] != "2026-08-23 18:30" {
		t.Fatalf("after set: start_date=%v due_date=%v", got["start_date"], got["due_date"])
	}

	// クリア（既存バグの修正確認：空文字送信で実際にnilへ戻ること）。
	w = doJSON(t, r, http.MethodPatch, "/tasks/"+taskID, token, map[string]any{
		"start_date": "",
		"due_date":   "",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch(clear) status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doJSON(t, r, http.MethodGet, "/tasks/"+taskID, token, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get after clear: %v", err)
	}
	if got["start_date"] != nil {
		t.Errorf("start_date after clear = %v, want nil", got["start_date"])
	}
	if got["due_date"] != nil {
		t.Errorf("due_date after clear = %v, want nil", got["due_date"])
	}
}

func TestTaskUpdate_DateTimeRange_OmittedFieldUnchanged(t *testing.T) {
	r, token, wsID := setupDateTimeTestRouter(t)

	w := doJSON(t, r, http.MethodPost, "/workspaces/"+wsID+"/tasks", token, map[string]any{
		"title":    "t2",
		"due_date": "2026-08-23 18:30",
	})
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	taskID, _ := created["id"].(string)

	// due_dateを送らない部分PATCH（タイトルのみ変更）。
	w = doJSON(t, r, http.MethodPatch, "/tasks/"+taskID, token, map[string]any{
		"title": "renamed",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doJSON(t, r, http.MethodGet, "/tasks/"+taskID, token, nil)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if got["due_date"] != "2026-08-23 18:30" {
		t.Errorf("due_date changed by omitted field: got %v, want unchanged 2026-08-23 18:30", got["due_date"])
	}
}

func TestTaskUpdate_DateTimeRange_InvalidRange(t *testing.T) {
	r, token, wsID := setupDateTimeTestRouter(t)

	w := doJSON(t, r, http.MethodPost, "/workspaces/"+wsID+"/tasks", token, map[string]any{"title": "t3"})
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	taskID, _ := created["id"].(string)

	w = doJSON(t, r, http.MethodPatch, "/tasks/"+taskID, token, map[string]any{
		"start_date": "2026-08-23 20:00",
		"due_date":   "2026-08-23 09:00",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400", w.Code, w.Body.String())
	}
}

func TestTaskUpdate_DateTimeRange_RejectsLegacyDateOnlyFormat(t *testing.T) {
	r, token, wsID := setupDateTimeTestRouter(t)

	w := doJSON(t, r, http.MethodPost, "/workspaces/"+wsID+"/tasks", token, map[string]any{"title": "t4"})
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	taskID, _ := created["id"].(string)

	w = doJSON(t, r, http.MethodPatch, "/tasks/"+taskID, token, map[string]any{"due_date": "2026-08-23"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400 for legacy date-only format", w.Code, w.Body.String())
	}
}

// formatDateTimeがJSTで整形することの単体確認（往復のズレ検出用）。
func TestFormatDateTime_JST(t *testing.T) {
	utc := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC) // = 2026-08-23 18:00 JST
	got := formatDateTime(&utc)
	if got == nil || *got != "2026-08-23 18:00" {
		t.Errorf("formatDateTime = %v, want 2026-08-23 18:00", got)
	}
	if formatDateTime(nil) != nil {
		t.Error("formatDateTime(nil) should return nil")
	}
}
