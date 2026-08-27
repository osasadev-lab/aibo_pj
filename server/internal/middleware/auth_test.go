package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/testutil"
)

const testJWTSecret = "test-jwt-secret"

func newTestRouter(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", append(handlers, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})...)
	return r
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	client := testutil.NewClient(t)
	r := newTestRouter(RequireAuth(client, testJWTSecret))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_MalformedHeader(t *testing.T) {
	client := testutil.NewClient(t)
	r := newTestRouter(RequireAuth(client, testJWTSecret))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "NotBearer sometoken")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	client := testutil.NewClient(t)
	r := newTestRouter(RequireAuth(client, testJWTSecret))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer this.is.not.valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_TokenForNonexistentUser(t *testing.T) {
	client := testutil.NewClient(t)
	r := newTestRouter(RequireAuth(client, testJWTSecret))

	// 有効な署名だが、DB上に存在しないユーザーIDのトークン（削除済みユーザー等を想定）。
	token, err := internalauth.IssueToken(testJWTSecret, uuid.New())
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_ValidTokenPassesThrough(t *testing.T) {
	client := testutil.NewClient(t)
	u, err := client.User.Create().
		SetGoogleSub("sub-1").
		SetEmail("valid@example.com").
		SetName("Valid User").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, err := internalauth.IssueToken(testJWTSecret, u.ID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	var seenUserID string
	r := newTestRouter(RequireAuth(client, testJWTSecret), func(c *gin.Context) {
		if got := CurrentUser(c); got != nil {
			seenUserID = got.ID.String()
		}
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if seenUserID != u.ID.String() {
		t.Errorf("CurrentUser in downstream handler = %q, want %q", seenUserID, u.ID.String())
	}
}
