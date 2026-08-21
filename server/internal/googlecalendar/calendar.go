// Package googlecalendar はGoogle Calendar APIへの薄いラッパー（M6、Googleカレンダー連携）。
// storage.R2Clientと同じ「外部サービスへの最小限のラッパー」という設計方針を踏襲する。
package googlecalendar

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	primaryCalendarID = "primary"
	dateLayout        = "2006-01-02"
)

// Client はrefresh tokenを1本だけ保持するユーザースコープのCalendar APIクライアント。
type Client struct {
	svc *calendar.Service
}

// NewClient はrefresh tokenからCalendar APIクライアントを構築する。
// oauthCfgはinternal/auth.NewGoogleCalendarOAuthConfigで作ったものを渡す。
func NewClient(ctx context.Context, oauthCfg *oauth2.Config, refreshToken string) (*Client, error) {
	tokenSource := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	svc, err := calendar.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc}, nil
}

// UpsertEvent は終日イベントを作成/更新する。existingEventIDが空文字なら新規作成、
// 指定されていれば更新する。endDateExclusiveはGoogle Calendarの終日イベント仕様
// （end.dateはその日を含まない排他的な日付）に従い、呼び出し側で最終日の翌日を渡すこと。
// 戻り値はGoogle Calendar側のイベントID。
func (c *Client) UpsertEvent(ctx context.Context, existingEventID, title, description string, startDate, endDateExclusive time.Time) (string, error) {
	event := &calendar.Event{
		Summary:     title,
		Description: description,
		Start:       &calendar.EventDateTime{Date: startDate.Format(dateLayout)},
		End:         &calendar.EventDateTime{Date: endDateExclusive.Format(dateLayout)},
	}

	if existingEventID != "" {
		updated, err := c.svc.Events.Update(primaryCalendarID, existingEventID, event).Context(ctx).Do()
		if err == nil {
			return updated.Id, nil
		}
		// 既にユーザー側でイベントが削除されている等で404/410の場合は、
		// 新規作成として作り直す（同期の取りこぼしを防ぐ）。
		if !IsNotFoundError(err) {
			return "", err
		}
	}

	created, err := c.svc.Events.Insert(primaryCalendarID, event).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return created.Id, nil
}

// DeleteEvent はイベントを削除する。既に存在しない（404/410）場合はエラーにしない
// （削除したい状態には既になっているため）。
func (c *Client) DeleteEvent(ctx context.Context, eventID string) error {
	err := c.svc.Events.Delete(primaryCalendarID, eventID).Context(ctx).Do()
	if err != nil && !IsNotFoundError(err) {
		return err
	}
	return nil
}

// IsInvalidGrantError はリフレッシュトークンが失効している（ユーザーがGoogle側で
// アクセスを取り消した等）ことを示すエラーかどうかを判定する。呼び出し側はこの場合、
// そのユーザーのcalendar_sync_enabledをfalseに落として再認証を促す
// （spec.md 5章「失効時は再認証を促す」）。
func IsInvalidGrantError(err error) bool {
	if err == nil {
		return false
	}
	if retrieveErr, ok := errors.AsType[*oauth2.RetrieveError](err); ok && retrieveErr.ErrorCode == "invalid_grant" {
		return true
	}
	// TokenSourceのエラーはgoogleapi呼び出し側でラップされ型情報が失われる場合があるため、
	// フォールバックとして文字列判定も行う。
	return strings.Contains(err.Error(), "invalid_grant")
}

// IsNotFoundError はGoogle Calendar API側で既にイベントが存在しないことを示す(404/410)。
func IsNotFoundError(err error) bool {
	apiErr, ok := errors.AsType[*googleapi.Error](err)
	if !ok {
		return false
	}
	return apiErr.Code == http.StatusNotFound || apiErr.Code == http.StatusGone
}
