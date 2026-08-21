package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleUserInfo はGoogleのuserinfoエンドポイントから取得するプロフィール。
type GoogleUserInfo struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// NewGoogleOAuthConfig はログイン用スコープ（openid/email/profile）のみのoauth2.Configを返す。
// カレンダー連携用スコープ(calendar.events)はM6で別途・任意タイミングで同意を取る。
func NewGoogleOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

// NewGoogleCalendarOAuthConfig はカレンダー連携用（M6）のoauth2.Configを返す。
// ログイン用スコープに加えcalendar.eventsスコープを要求する。ログイン用の
// NewGoogleOAuthConfigとはredirect URLを分ける（docs/aibo/m6-implementation-plan.md
// の設計判断：stateパラメータにaibo JWTをそのまま載せて本人確認を行う専用フローのため）。
func NewGoogleCalendarOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile", "https://www.googleapis.com/auth/calendar.events"},
		Endpoint:     google.Endpoint,
	}
}

// FetchUserInfo はアクセストークンを使ってGoogleのuserinfoエンドポイントからプロフィールを取得する。
func FetchUserInfo(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (*GoogleUserInfo, error) {
	client := cfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var info GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}
