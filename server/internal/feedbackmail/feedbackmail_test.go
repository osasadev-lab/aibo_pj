package feedbackmail

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConfig_Configured(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"all fields set", Config{Host: "smtp.gmail.com", Port: "587", Username: "a@b.com", Password: "x", NotifyEmail: "c@d.com"}, true},
		{"missing host", Config{Port: "587", Username: "a@b.com", Password: "x", NotifyEmail: "c@d.com"}, false},
		{"missing password", Config{Host: "smtp.gmail.com", Port: "587", Username: "a@b.com", NotifyEmail: "c@d.com"}, false},
		{"zero value", Config{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Configured(); got != tc.want {
				t.Errorf("Configured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEncodeBase64Body_RoundTrip(t *testing.T) {
	// 日本語を含む本文が、経路の8bit透過性に依存せず正しく復元できることを
	// 確認する（2026-08-27、文字化け修正の回帰防止テスト）。
	original := "投稿者: 佐々木裕哉\n本文: テストです。日本語が正しく表示されるか確認。"

	encoded := encodeBase64Body(original)

	// RFC 2045: base64本文は76文字ごとにCRLFで折り返す必要がある。
	for _, line := range strings.Split(strings.TrimSuffix(encoded, "\r\n"), "\r\n") {
		if len(line) > 76 {
			t.Errorf("line exceeds 76 chars (%d): %q", len(line), line)
		}
	}

	rawB64 := strings.ReplaceAll(encoded, "\r\n", "")
	decoded, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if string(decoded) != original {
		t.Errorf("round trip mismatch:\n got: %q\nwant: %q", string(decoded), original)
	}
}

func TestBuildBody_IncludesKeyFields(t *testing.T) {
	wsID := uuid.New()
	item := Item{
		SenderName:  "佐々木裕哉",
		SenderEmail: "osasadev@example.com",
		WorkspaceID: &wsID,
		PagePath:    "/w/abc/projects",
		Body:        "フィードバック本文",
		CreatedAt:   time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC),
	}
	body := buildBody(item)

	for _, want := range []string{item.SenderName, item.SenderEmail, wsID.String(), item.PagePath, item.Body} {
		if !strings.Contains(body, want) {
			t.Errorf("buildBody() does not contain %q\nfull body: %s", want, body)
		}
	}
}

func TestBuildBody_NilWorkspaceID(t *testing.T) {
	item := Item{
		SenderName:  "test",
		SenderEmail: "test@example.com",
		WorkspaceID: nil,
		Body:        "body",
		CreatedAt:   time.Now(),
	}
	body := buildBody(item)
	if !strings.Contains(body, "ワークスペース外") {
		t.Errorf("expected placeholder for nil workspace id, got: %s", body)
	}
}
