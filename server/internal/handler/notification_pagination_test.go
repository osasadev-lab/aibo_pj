package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

const notifTestJWTSecret = "notif-test-secret"

type notifListResponse struct {
	Items   []map[string]any `json:"items"`
	HasMore bool             `json:"has_more"`
}

// setupNotificationTest は認証込みの/notificationsルートを持つルーターと、
// 対象ユーザー宛のトークンを用意する（通知0件の状態）。
func setupNotificationTest(t *testing.T) (router *gin.Engine, token string) {
	t.Helper()
	client := testutil.NewClient(t)

	u, err := client.User.Create().
		SetGoogleSub("sub").SetEmail("notif@example.com").SetName("Notif User").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	tok, err := internalauth.IssueToken(notifTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewNotificationHandler(client, pushdelivery.Config{})
	notifications := r.Group("/notifications", middleware.RequireAuth(client, notifTestJWTSecret))
	notifications.GET("", h.List)

	return r, tok
}

func doGet(t *testing.T, r *gin.Engine, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestNotificationList_EmptyByDefault(t *testing.T) {
	r, token := setupNotificationTest(t)

	w := doGet(t, r, "/notifications", token)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var res notifListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.Items) != 0 || res.HasMore {
		t.Errorf("expected empty result, got items=%d has_more=%v", len(res.Items), res.HasMore)
	}
}

func TestNotificationList_PaginationNoOverlap(t *testing.T) {
	client := testutil.NewClient(t)

	u, err := client.User.Create().
		SetGoogleSub("sub2").SetEmail("notif2@example.com").SetName("Notif User 2").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 25件の通知を、created_atをずらして作成する（同時刻だとDBのソート順が
	// 不定になり得るため、1ミリ秒ずつずらして厳密な順序を保証する）。
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		_, err := client.Notification.Create().
			SetUserID(u.ID).
			SetType("assigned").
			SetCreatedAt(base.Add(time.Duration(i) * time.Millisecond)).
			Save(context.Background())
		if err != nil {
			t.Fatalf("create notification %d: %v", i, err)
		}
	}

	tok, err := internalauth.IssueToken(notifTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewNotificationHandler(client, pushdelivery.Config{})
	notifications := r.Group("/notifications", middleware.RequireAuth(client, notifTestJWTSecret))
	notifications.GET("", h.List)

	// 1ページ目：limit=10
	w1 := doGet(t, r, "/notifications?limit=10", tok)
	var page1 notifListResponse
	if err := json.Unmarshal(w1.Body.Bytes(), &page1); err != nil {
		t.Fatalf("unmarshal page1: %v", err)
	}
	if len(page1.Items) != 10 {
		t.Fatalf("page1 items = %d, want 10", len(page1.Items))
	}
	if !page1.HasMore {
		t.Error("page1 has_more = false, want true (25 total, limit 10)")
	}

	lastID, _ := page1.Items[len(page1.Items)-1]["id"].(string)
	if lastID == "" {
		t.Fatal("could not extract id from last item of page1")
	}

	// 2ページ目：before=最後のID
	w2 := doGet(t, r, "/notifications?limit=10&before="+lastID, tok)
	var page2 notifListResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &page2); err != nil {
		t.Fatalf("unmarshal page2: %v", err)
	}
	if len(page2.Items) != 10 {
		t.Fatalf("page2 items = %d, want 10", len(page2.Items))
	}
	if !page2.HasMore {
		t.Error("page2 has_more = false, want true (25 total, 10+10=20 consumed)")
	}

	// ページ間でIDの重複が無いことを確認する。
	seen := map[string]bool{}
	for _, item := range page1.Items {
		id, _ := item["id"].(string)
		seen[id] = true
	}
	for _, item := range page2.Items {
		id, _ := item["id"].(string)
		if seen[id] {
			t.Errorf("id %s appears in both page1 and page2", id)
		}
	}

	// 3ページ目：残り5件、has_more=falseになるはず。
	lastID2, _ := page2.Items[len(page2.Items)-1]["id"].(string)
	w3 := doGet(t, r, "/notifications?limit=10&before="+lastID2, tok)
	var page3 notifListResponse
	if err := json.Unmarshal(w3.Body.Bytes(), &page3); err != nil {
		t.Fatalf("unmarshal page3: %v", err)
	}
	if len(page3.Items) != 5 {
		t.Fatalf("page3 items = %d, want 5", len(page3.Items))
	}
	if page3.HasMore {
		t.Error("page3 has_more = true, want false (all 25 consumed)")
	}
}

func TestNotificationList_UnreadFilter(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().
		SetGoogleSub("sub3").SetEmail("notif3@example.com").SetName("Notif User 3").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := client.Notification.Create().SetUserID(u.ID).SetType("assigned").Save(context.Background()); err != nil {
		t.Fatalf("create unread notification: %v", err)
	}
	readAt := time.Now()
	if _, err := client.Notification.Create().SetUserID(u.ID).SetType("assigned").SetReadAt(readAt).Save(context.Background()); err != nil {
		t.Fatalf("create read notification: %v", err)
	}

	tok, err := internalauth.IssueToken(notifTestJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewNotificationHandler(client, pushdelivery.Config{})
	notifications := r.Group("/notifications", middleware.RequireAuth(client, notifTestJWTSecret))
	notifications.GET("", h.List)

	w := doGet(t, r, "/notifications?unread=true", tok)
	var res notifListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("unread items = %d, want 1", len(res.Items))
	}
}
