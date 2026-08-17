package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/user"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspaceinvitation"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// AuthHandler は /auth 配下のエンドポイントを扱う。
type AuthHandler struct {
	client       *ent.Client
	oauthConfig  *oauth2.Config
	jwtSecret    string
	frontendURL  string
	cookieSecure bool
}

// NewAuthHandler はAuthHandlerを構築する。cookieSecureは本番(HTTPS)ではtrue、
// ローカルのhttp開発ではfalseを渡す。
func NewAuthHandler(client *ent.Client, clientID, clientSecret, redirectURL, jwtSecret, frontendURL string, cookieSecure bool) *AuthHandler {
	return &AuthHandler{
		client:       client,
		oauthConfig:  internalauth.NewGoogleOAuthConfig(clientID, clientSecret, redirectURL),
		jwtSecret:    jwtSecret,
		frontendURL:  frontendURL,
		cookieSecure: cookieSecure,
	}
}

// GoogleLogin は GET /auth/google/login。Google認可URLへリダイレクトする。
func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	state, err := internalauth.GenerateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start login"})
		return
	}
	internalauth.SetStateCookie(c, state, h.cookieSecure)
	c.Redirect(http.StatusTemporaryRedirect, h.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline))
}

// GoogleCallback は GET /auth/google/callback。
// codeをトークンに交換し、ユーザーを検索/作成した上でJWTを発行してフロントへリダイレクトする。
func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	ctx := c.Request.Context()

	if !internalauth.VerifyStateCookie(c, c.Query("state")) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code"})
		return
	}

	token, err := h.oauthConfig.Exchange(ctx, code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to exchange code"})
		return
	}

	info, err := internalauth.FetchUserInfo(ctx, h.oauthConfig, token)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch user info"})
		return
	}

	u, err := h.client.User.Query().Where(user.GoogleSubEQ(info.Sub)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		u, err = h.createUserAndConsumeInvitations(ctx, info)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			return
		}
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up user"})
		return
	}

	jwtStr, err := internalauth.IssueToken(h.jwtSecret, u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, h.frontendURL+"/auth/callback?token="+jwtStr)
}

// createUserAndConsumeInvitations は新規ユーザーを作成し、そのメールアドレス宛の
// 招待(workspace_invitations)があればworkspace_memberへ変換して消費する。
func (h *AuthHandler) createUserAndConsumeInvitations(ctx context.Context, info *internalauth.GoogleUserInfo) (*ent.User, error) {
	var created *ent.User
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		u, err := tx.User.Create().
			SetGoogleSub(info.Sub).
			SetEmail(info.Email).
			SetName(info.Name).
			SetNillableAvatarURL(nonEmptyPtr(info.Picture)).
			Save(ctx)
		if err != nil {
			return err
		}

		invitations, err := tx.WorkspaceInvitation.Query().
			Where(workspaceinvitation.EmailEQ(info.Email)).
			All(ctx)
		if err != nil {
			return err
		}

		for _, inv := range invitations {
			if _, err := tx.WorkspaceMember.Create().
				SetWorkspaceID(inv.WorkspaceID).
				SetUserID(u.ID).
				SetRole(workspacemember.RoleMember).
				Save(ctx); err != nil {
				return err
			}
			if err := tx.WorkspaceInvitation.DeleteOne(inv).Exec(ctx); err != nil {
				return err
			}
		}

		created = u
		return nil
	})
	return created, err
}

func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Logout は POST /auth/logout。ステートレスJWTのためサーバー側で無効化する状態は持たない。
func (h *AuthHandler) Logout(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Me は GET /auth/me。ログイン中のユーザー情報を返す。
func (h *AuthHandler) Me(c *gin.Context) {
	u := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{
		"id":         u.ID,
		"email":      u.Email,
		"name":       u.Name,
		"avatar_url": u.AvatarURL,
	})
}
