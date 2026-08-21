package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
)

// stateDelimiter でaibo JWTとworkspace_idをつなげてGoogleのstateパラメータに載せる。
// JWTはbase64url文字（英数字・-・_）とドットのみで構成されるため衝突しない。
const stateDelimiter = "::"

// CalendarConnectHandler はGoogleカレンダー連携（M6）の同意フロー専用ハンドラ。
// ログイン用の GET /auth/google/login・/callback とは別経路にする理由は
// docs/aibo/m6-implementation-plan.md「設計判断1」参照：Bearer JWT方式のため
// フルページ遷移ではAuthorizationヘッダーを送れず、代わりにaibo JWTそのものを
// oauth2の`state`パラメータに載せて本人確認を行う。
type CalendarConnectHandler struct {
	client      *ent.Client
	oauthConfig *oauth2.Config
	jwtSecret   string
	encKey      []byte
	frontendURL string
}

func NewCalendarConnectHandler(client *ent.Client, oauthConfig *oauth2.Config, jwtSecret string, encKey []byte, frontendURL string) *CalendarConnectHandler {
	return &CalendarConnectHandler{
		client:      client,
		oauthConfig: oauthConfig,
		jwtSecret:   jwtSecret,
		encKey:      encKey,
		frontendURL: frontendURL,
	}
}

// Connect は GET /auth/google/calendar/connect?token=<aibo JWT>&workspace_id=<省略可>。
// フロントはフルページ遷移（Authorizationヘッダーが使えないため）でここに来る想定。
// workspace_idは連携完了後にどのワークスペースの設定画面へ戻すかを覚えておくために
// stateへ載せる（連携自体はワークスペースに紐づかないメンバー個人設定のため、
// あくまでUXのための往復情報）。
func (h *CalendarConnectHandler) Connect(c *gin.Context) {
	token := c.Query("token")
	if _, err := internalauth.ParseToken(h.jwtSecret, token); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}

	state := token
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		if _, err := uuid.Parse(workspaceID); err == nil {
			state = token + stateDelimiter + workspaceID
		}
	}

	// access_type=offlineでrefresh tokenを要求し、prompt=consentで再認可時にも
	// 確実にrefresh tokenを取得する（offlineアクセスは初回同意時のみ返る仕様のため）。
	url := h.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// Callback は GET /auth/google/calendar/callback。
// stateに載せておいたaibo JWT（＋任意でworkspace_id）を検証してuser_idを特定し、
// codeをトークン交換して得たrefresh tokenを暗号化して保存する。
func (h *CalendarConnectHandler) Callback(c *gin.Context) {
	ctx := c.Request.Context()

	tokenPart, workspaceID := splitConnectState(c.Query("state"))
	userID, err := internalauth.ParseToken(h.jwtSecret, tokenPart)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.frontendURL+"/settings/calendar-callback?error=1")
		return
	}

	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusTemporaryRedirect, h.connectResultURL(workspaceID, "error"))
		return
	}

	token, err := h.oauthConfig.Exchange(ctx, code)
	if err != nil || token.RefreshToken == "" {
		// token.RefreshTokenが空＝同意画面をスキップした等でrefresh tokenが
		// 取得できなかったケース（低確度の解釈、docs/aibo/m6-implementation-plan.md）。
		c.Redirect(http.StatusTemporaryRedirect, h.connectResultURL(workspaceID, "error"))
		return
	}

	encrypted, err := internalauth.EncryptToken(h.encKey, token.RefreshToken)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.connectResultURL(workspaceID, "error"))
		return
	}

	if _, err := h.client.User.UpdateOneID(userID).SetGoogleRefreshToken(encrypted).Save(ctx); err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.connectResultURL(workspaceID, "error"))
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, h.connectResultURL(workspaceID, "connected"))
}

// splitConnectState はConnectで組み立てたstateを分解する。旧形式（workspace_id無し、
// JWTのみ）とも互換性を保つため、区切り文字が無ければworkspaceIDは空文字を返す。
func splitConnectState(state string) (token string, workspaceID string) {
	if idx := strings.LastIndex(state, stateDelimiter); idx != -1 {
		return state[:idx], state[idx+len(stateDelimiter):]
	}
	return state, ""
}

// connectResultURL は連携結果の戻り先を組み立てる。workspace_idが分かっていれば
// 連携を開始したワークスペースの設定画面へ、無ければ汎用の中継ページへ戻す。
func (h *CalendarConnectHandler) connectResultURL(workspaceID, status string) string {
	if workspaceID != "" {
		return fmt.Sprintf("%s/w/%s/settings?calendar=%s", h.frontendURL, workspaceID, status)
	}
	return fmt.Sprintf("%s/settings/calendar-callback?%s=1", h.frontendURL, status)
}
