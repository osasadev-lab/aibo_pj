package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/user"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspaceinvitation"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/calendarsync"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"
)

// AuthHandler は /auth 配下のエンドポイントを扱う。
type AuthHandler struct {
	client            *ent.Client
	oauthConfig       *oauth2.Config
	jwtSecret         string
	supabaseJWTSecret string
	frontendURL       string
	cookieSecure      bool
	r2                *storage.R2Client
	// calendarOAuthConfig/tokenEncryptionKeyはM6（Googleカレンダー連携）用。
	// /me/calendar-settings・/me/calendar-syncで使う。
	calendarOAuthConfig *oauth2.Config
	tokenEncryptionKey  []byte
}

// NewAuthHandler はAuthHandlerを構築する。cookieSecureは本番(HTTPS)ではtrue、
// ローカルのhttp開発ではfalseを渡す。r2はGET /auth/meのstorage_enabledフラグに
// 使う（未設定＝ローカル開発でR2を無効化中でも、フロントが添付ファイルUIを
// 出さないようにするための情報。ユーザー確認済みの方針）。calendarOAuthConfig/
// tokenEncryptionKeyはM6用（internal/auth.NewGoogleCalendarOAuthConfig・
// DecodeEncryptionKeyで作ったものを渡す）。
func NewAuthHandler(client *ent.Client, clientID, clientSecret, redirectURL, jwtSecret, supabaseJWTSecret, frontendURL string, cookieSecure bool, r2 *storage.R2Client, calendarOAuthConfig *oauth2.Config, tokenEncryptionKey []byte) *AuthHandler {
	return &AuthHandler{
		client:              client,
		oauthConfig:         internalauth.NewGoogleOAuthConfig(clientID, clientSecret, redirectURL),
		jwtSecret:           jwtSecret,
		supabaseJWTSecret:   supabaseJWTSecret,
		frontendURL:         frontendURL,
		cookieSecure:        cookieSecure,
		r2:                  r2,
		calendarOAuthConfig: calendarOAuthConfig,
		tokenEncryptionKey:  tokenEncryptionKey,
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
		"id":              u.ID,
		"email":           u.Email,
		"name":            u.Name,
		"avatar_url":      u.AvatarURL,
		"storage_enabled": h.r2 != nil,
	})
}

// GetHoverSettings は GET /me/hover-settings。カンバンのホバー強調モードの
// 個人設定を返す（M4追加）。
func (h *AuthHandler) GetHoverSettings(c *gin.Context) {
	u := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{"mode": u.HoverHighlightMode})
}

type updateHoverSettingsRequest struct {
	Mode string `json:"mode" binding:"required,oneof=off tag dependency subtask"`
}

// UpdateHoverSettings は PATCH /me/hover-settings。
func (h *AuthHandler) UpdateHoverSettings(c *gin.Context) {
	u := middleware.CurrentUser(c)

	var req updateHoverSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be one of off/tag/dependency"})
		return
	}

	updated, err := h.client.User.UpdateOneID(u.ID).
		SetHoverHighlightMode(user.HoverHighlightMode(req.Mode)).
		Save(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update hover settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": updated.HoverHighlightMode})
}

// SupabaseToken は GET /me/supabase-token。Supabase Realtimeのチャンネル認証用に、
// このアプリのセッションJWTとは別の鍵（SUPABASE_JWT_SECRET）で署名した短命JWTを発行する。
func (h *AuthHandler) SupabaseToken(c *gin.Context) {
	u := middleware.CurrentUser(c)
	token, err := internalauth.IssueSupabaseToken(h.supabaseJWTSecret, u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue supabase token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// GetCalendarSettings は GET /me/calendar-settings（M6追加）。
// connectedはgoogle_refresh_tokenを保持しているか（＝一度でもGoogle同意フローを
// 完了しているか）を表す。フロントはこれを見て「連携する」ボタンと
// ON/OFFトグルのどちらを出すか判断する。
func (h *AuthHandler) GetCalendarSettings(c *gin.Context) {
	u := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{
		"enabled":   u.CalendarSyncEnabled,
		"mode":      u.CalendarSyncMode,
		"connected": u.GoogleRefreshToken != nil,
	})
}

type updateCalendarSettingsRequest struct {
	Enabled bool    `json:"enabled"`
	Mode    *string `json:"mode" binding:"omitempty,oneof=auto manual"`
}

// UpdateCalendarSettings は PATCH /me/calendar-settings（M6追加）。
// enabled=trueにするにはGoogle側の同意（GET /auth/google/calendar/connect）が
// 完了しrefresh_tokenを保持している必要がある。enabled=falseにする場合は、
// 連携済みイベントを全削除する（連携OFF＝カレンダーに残骸を残さない方針、
// docs/aibo/m6-implementation-plan.md 設計判断6参照）。
func (h *AuthHandler) UpdateCalendarSettings(c *gin.Context) {
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var req updateCalendarSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Enabled && u.GoogleRefreshToken == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "google_calendar_not_connected"})
		return
	}
	if req.Enabled && req.Mode == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode is required when enabling"})
		return
	}

	builder := h.client.User.UpdateOneID(u.ID).SetCalendarSyncEnabled(req.Enabled)
	if req.Enabled {
		builder = builder.SetCalendarSyncMode(user.CalendarSyncMode(*req.Mode))
	} else {
		builder = builder.ClearCalendarSyncMode()
	}
	updated, err := builder.Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update calendar settings"})
		return
	}

	if !req.Enabled {
		// バックグラウンド化の理由はtask.goと同じ（体感速度対策、2026-08-21）。
		userID := u.ID
		calendarsync.Async(func(ctx context.Context) {
			calendarsync.DeleteAllEventsForUser(ctx, h.client, h.calendarOAuthConfig, h.tokenEncryptionKey, userID)
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":   updated.CalendarSyncEnabled,
		"mode":      updated.CalendarSyncMode,
		"connected": updated.GoogleRefreshToken != nil,
	})
}

// reminderTimeSlots は15分刻みの許容値集合（"00:00"〜"23:45"、M7設計判断11）。
var reminderTimeSlots = func() map[string]bool {
	slots := make(map[string]bool, 96)
	for h := 0; h < 24; h++ {
		for _, m := range []int{0, 15, 30, 45} {
			slots[fmt.Sprintf("%02d:%02d", h, m)] = true
		}
	}
	return slots
}()

// GetReminderSettings は GET /me/reminder-settings（M7追加）。
func (h *AuthHandler) GetReminderSettings(c *gin.Context) {
	u := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{
		"due_today_enabled": u.ReminderDueTodayEnabled,
		"due_today_time":    u.ReminderDueTodayTime,
		"overdue_enabled":   u.ReminderOverdueEnabled,
		"overdue_time":      u.ReminderOverdueTime,
	})
}

type updateReminderSettingsRequest struct {
	DueTodayEnabled bool    `json:"due_today_enabled"`
	DueTodayTime    *string `json:"due_today_time"`
	OverdueEnabled  bool    `json:"overdue_enabled"`
	OverdueTime     *string `json:"overdue_time"`
}

// UpdateReminderSettings は PATCH /me/reminder-settings（M7追加）。
// *_enabled=trueにする場合は対応する*_timeが必須かつ15分刻みの"HH:MM"形式であることを
// 検証する（設計判断11）。*_enabled=falseにする場合は*_timeをnullクリアする
// （OFF時は時刻設定自体に意味が無いため、calendar_sync_modeとは異なりクリアする方針）。
func (h *AuthHandler) UpdateReminderSettings(c *gin.Context) {
	u := middleware.CurrentUser(c)

	var req updateReminderSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.DueTodayEnabled && (req.DueTodayTime == nil || !reminderTimeSlots[*req.DueTodayTime]) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "due_today_time must be a 15-minute slot (HH:MM) when due_today_enabled"})
		return
	}
	if req.OverdueEnabled && (req.OverdueTime == nil || !reminderTimeSlots[*req.OverdueTime]) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "overdue_time must be a 15-minute slot (HH:MM) when overdue_enabled"})
		return
	}

	builder := h.client.User.UpdateOneID(u.ID).
		SetReminderDueTodayEnabled(req.DueTodayEnabled).
		SetReminderOverdueEnabled(req.OverdueEnabled)
	if req.DueTodayEnabled {
		builder = builder.SetReminderDueTodayTime(*req.DueTodayTime)
	} else {
		builder = builder.ClearReminderDueTodayTime()
	}
	if req.OverdueEnabled {
		builder = builder.SetReminderOverdueTime(*req.OverdueTime)
	} else {
		builder = builder.ClearReminderOverdueTime()
	}

	updated, err := builder.Save(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update reminder settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"due_today_enabled": updated.ReminderDueTodayEnabled,
		"due_today_time":    updated.ReminderDueTodayTime,
		"overdue_enabled":   updated.ReminderOverdueEnabled,
		"overdue_time":      updated.ReminderOverdueTime,
	})
}

type manualCalendarSyncRequest struct {
	Date string `json:"date" binding:"required"`
}

// maxManualSyncTasks は手動連携1回あたりの対象タスク数上限（api-spec.md
// 「非同期処理・外部連携」、1リクエストのタイムアウトを避けるため）。
const maxManualSyncTasks = 50

// ManualCalendarSync は POST /me/calendar-sync（M6追加、手動モード用）。
// 指定日が期限になっている自分の担当タスクをまとめてGoogleカレンダーへ反映する。
func (h *AuthHandler) ManualCalendarSync(c *gin.Context) {
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var req manualCalendarSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is required"})
		return
	}
	date, err := time.Parse(dateLayout, req.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date"})
		return
	}
	if !u.CalendarSyncEnabled || u.GoogleRefreshToken == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "calendar_sync_not_enabled"})
		return
	}

	tasks, err := h.client.Task.Query().
		Where(task.DueDateEQ(date), task.HasAssigneesWith(taskassignee.UserIDEQ(u.ID))).
		Limit(maxManualSyncTasks).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tasks"})
		return
	}

	synced, failed := 0, 0
	for _, t := range tasks {
		if err := calendarsync.ManualSyncForUser(ctx, h.client, h.calendarOAuthConfig, h.tokenEncryptionKey, u, t, h.frontendURL); err != nil {
			failed++
		} else {
			synced++
		}
	}
	c.JSON(http.StatusOK, gin.H{"synced": synced, "failed": failed})
}
