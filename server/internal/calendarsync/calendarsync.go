// Package calendarsync はタスクのcreate/update/delete/担当者変更のたびに、
// 担当者ごとのGoogleカレンダーへイベントを反映する（M6、自動モード）。
//
// ここに集めた関数はすべてDBトランザクションの外（withTxのコミット後）で呼ぶこと。
// storage.deleteR2Objectsと同じ「外部API呼び出しをDBトランザクション内に含めない」方針
// （docs/aibo/m6-implementation-plan.md）で、失敗してもHTTPレスポンスはブロックしない
// （エラーはログとactivity_logsへの記録のみ、その場ではリトライしない）。
//
// 呼び出し元のハンドラは基本的にAsync()経由でバックグラウンド実行すること
// （token refresh + Calendar API呼び出しで数秒かかることがあり、同期呼び出しだと
// タスク保存等の体感速度を大きく損なうため。2026-08-21、実機で確認の上、同期呼び出し
// からgoroutine化に変更した）。POST /me/calendar-syncのみ、結果件数をレスポンスで
// 返す必要があるため例外的に同期のまま呼ぶ。
package calendarsync

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskcalendarevent"
	"github.com/osasadev-lab/aibo_pj/server/ent/user"
	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/googlecalendar"
)

// asyncTimeout はAsync()で起動するgoroutine1回あたりの上限（token refresh +
// Calendar API呼び出し1〜数回分の余裕を見て設定）。
const asyncTimeout = 20 * time.Second

// defaultEventDuration は開始日時・期限のどちらか一方しか設定されていないタスクを
// Googleカレンダーへ同期する際の予定の長さ（2026-08-27追加、日時範囲対応）。
// 時間指定イベントは開始=終了だと見えづらいため、片方のみ設定時は30分の
// 予定ブロックとして表示する（due_dateのみ→期限の30分前を開始時刻に、
// start_dateのみ→開始日時の30分後を終了時刻に補う）。
const defaultEventDuration = 30 * time.Minute

// jst はGoogleカレンダーへ渡す日時のタイムゾーン（handler.jstと同じくAsia/Tokyo固定、
// LoadLocation失敗時はFixedZoneにフォールバックする同じパターンをここでも使う）。
var jst = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return time.FixedZone("Asia/Tokyo", 9*60*60)
	}
	return loc
}()

// resolveEventWindow はタスクのstart_date/due_dateからGoogleカレンダーイベントの
// 開始・終了時刻を決める純粋関数（テスト容易性のため副作用を切り離す）。
// 呼び出し側はstartDate/dueDateの少なくとも一方が非nilであることを保証すること
// （両方nilの場合は同期対象外として呼ばれない想定、syncOne参照）。
func resolveEventWindow(startDate, dueDate *time.Time) (start, end time.Time) {
	switch {
	case startDate != nil && dueDate != nil:
		return startDate.In(jst), dueDate.In(jst)
	case dueDate != nil:
		end = dueDate.In(jst)
		return end.Add(-defaultEventDuration), end
	default:
		start = startDate.In(jst)
		return start, start.Add(defaultEventDuration)
	}
}

// Async はfnをHTTPリクエストのライフサイクルから切り離したcontextで
// バックグラウンド実行する。c.Request.Context()はハンドラ関数がreturnし
// レスポンスが返った時点でキャンセルされるため、goroutine側では使えない
// （そのまま使うとGoogle API呼び出しが即座にcontext canceledで失敗する）。
func Async(fn func(ctx context.Context)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), asyncTimeout)
		defer cancel()
		fn(ctx)
	}()
}

// SyncTask はタスクtの担当者全員について自動モードでの同期を試みる
// （担当者ごとに連携ON/自動モードかどうかはSyncTaskForUser内で判定する）。
func SyncTask(ctx context.Context, client *ent.Client, calCfg *oauth2.Config, encKey []byte, t *ent.Task, frontendURL string) {
	assignees, err := client.TaskAssignee.Query().Where(taskassignee.TaskIDEQ(t.ID)).All(ctx)
	if err != nil {
		log.Printf("calendarsync: failed to load assignees for task %s: %v", t.ID, err)
		return
	}
	for _, a := range assignees {
		SyncTaskForUser(ctx, client, calCfg, encKey, t, a.UserID, frontendURL)
	}
}

// SyncTaskForUser は1ユーザー分の同期を行う。連携OFF・手動モードのユーザーには何もしない。
// start_date/due_dateが両方とも無いタスクは同期対象外（2026-08-27、開始日時のみの
// タスクも同期対象に含めるようspec.md 5章の方針を更新）のため、既存イベントがあれば
// 削除する（両方とも消した場合の後始末）。
func SyncTaskForUser(ctx context.Context, client *ent.Client, calCfg *oauth2.Config, encKey []byte, t *ent.Task, userID uuid.UUID, frontendURL string) {
	u, err := client.User.Get(ctx, userID)
	if err != nil {
		return
	}
	if !u.CalendarSyncEnabled || u.CalendarSyncMode == nil || *u.CalendarSyncMode != user.CalendarSyncModeAuto {
		return
	}
	syncOne(ctx, client, calCfg, encKey, u, t, frontendURL)
}

// ManualSyncForUser は手動モード用。モード確認をスキップし、連携ON（enabledのみ判定）で
// あれば同期する。POST /me/calendar-syncのハンドラから呼ぶ。
func ManualSyncForUser(ctx context.Context, client *ent.Client, calCfg *oauth2.Config, encKey []byte, u *ent.User, t *ent.Task, frontendURL string) error {
	return syncOne(ctx, client, calCfg, encKey, u, t, frontendURL)
}

func syncOne(ctx context.Context, client *ent.Client, calCfg *oauth2.Config, encKey []byte, u *ent.User, t *ent.Task, frontendURL string) error {
	if t.StartDate == nil && t.DueDate == nil {
		deleteEventForUser(ctx, client, calCfg, encKey, u, t.ID)
		return nil
	}
	if u.GoogleRefreshToken == nil {
		return fmt.Errorf("user %s has calendar sync enabled but no refresh token", u.ID)
	}

	refreshToken, err := internalauth.DecryptToken(encKey, *u.GoogleRefreshToken)
	if err != nil {
		log.Printf("calendarsync: failed to decrypt refresh token for user %s: %v", u.ID, err)
		return err
	}
	calClient, err := googlecalendar.NewClient(ctx, calCfg, refreshToken)
	if err != nil {
		log.Printf("calendarsync: failed to build calendar client for user %s: %v", u.ID, err)
		return err
	}

	existing, err := client.TaskCalendarEvent.Query().
		Where(taskcalendarevent.TaskIDEQ(t.ID), taskcalendarevent.UserIDEQ(u.ID)).
		Only(ctx)
	var existingEventID string
	if err == nil {
		existingEventID = existing.GoogleEventID
	} else if !ent.IsNotFound(err) {
		log.Printf("calendarsync: failed to look up existing event for task %s user %s: %v", t.ID, u.ID, err)
		return err
	}

	start, end := resolveEventWindow(t.StartDate, t.DueDate)

	eventID, err := calClient.UpsertEvent(ctx, existingEventID, t.Title, eventDescription(ctx, client, t, frontendURL), start, end)
	if err != nil {
		return handleSyncFailure(ctx, client, u, t, err)
	}

	if existingEventID != "" {
		_, err = client.TaskCalendarEvent.UpdateOneID(existing.ID).
			SetGoogleEventID(eventID).
			SetSyncedAt(time.Now()).
			Save(ctx)
	} else {
		_, err = client.TaskCalendarEvent.Create().
			SetTaskID(t.ID).
			SetUserID(u.ID).
			SetGoogleEventID(eventID).
			SetSyncedAt(time.Now()).
			Save(ctx)
	}
	if err != nil {
		log.Printf("calendarsync: failed to persist task_calendar_events for task %s user %s: %v", t.ID, u.ID, err)
	}
	return err
}

// EventRef はDeleteGoogleEventsOnly向けの最小限の参照情報。task/project/workspace削除は
// task_calendar_events行自体をFK制約回避のためDBトランザクション内で先に削除する
// 必要があるため（他のedgeテーブルと同じ手動カスケード削除パターン、db-schema.md参照）、
// コミット後に「DB行は既に無い状態でGoogle側のイベントだけ削除する」ための橋渡し役。
type EventRef struct {
	User          *ent.User
	GoogleEventID string
}

// DeleteGoogleEventsOnly はGoogle Calendar側のイベントのみを削除する（DB行は呼び出し元が
// 既にトランザクション内で削除済みという前提）。task/project/workspaceの削除ハンドラから、
// deleteR2Objectsと同じ「コミット後にベストエフォートで外部APIを叩く」箇所で呼ぶ。
func DeleteGoogleEventsOnly(ctx context.Context, calCfg *oauth2.Config, encKey []byte, refs []EventRef) {
	for _, ref := range refs {
		if ref.User == nil || ref.User.GoogleRefreshToken == nil {
			continue
		}
		refreshToken, err := internalauth.DecryptToken(encKey, *ref.User.GoogleRefreshToken)
		if err != nil {
			log.Printf("calendarsync: failed to decrypt refresh token for user %s: %v", ref.User.ID, err)
			continue
		}
		calClient, err := googlecalendar.NewClient(ctx, calCfg, refreshToken)
		if err != nil {
			log.Printf("calendarsync: failed to build calendar client for user %s: %v", ref.User.ID, err)
			continue
		}
		if err := calClient.DeleteEvent(ctx, ref.GoogleEventID); err != nil {
			log.Printf("calendarsync: failed to delete google event %s for user %s: %v", ref.GoogleEventID, ref.User.ID, err)
		}
	}
}

// SyncTaskForUserOnUnassign は担当者から外れた1ユーザーのイベントのみ削除する
// （PutAssignees、そのユーザーは他の担当者に影響を与えない）。
func SyncTaskForUserOnUnassign(ctx context.Context, client *ent.Client, calCfg *oauth2.Config, encKey []byte, taskID uuid.UUID, userID uuid.UUID) {
	u, err := client.User.Get(ctx, userID)
	if err != nil {
		return
	}
	deleteEventForUser(ctx, client, calCfg, encKey, u, taskID)
}

// DeleteAllEventsForUser はそのユーザーのtask_calendar_events全件を削除する
// （PATCH /me/calendar-settingsでenabled=falseにした際の後始末。連携OFF＝カレンダーに
// 残骸を残さない方針、docs/aibo/m6-implementation-plan.md 設計判断6参照）。
func DeleteAllEventsForUser(ctx context.Context, client *ent.Client, calCfg *oauth2.Config, encKey []byte, userID uuid.UUID) {
	u, err := client.User.Get(ctx, userID)
	if err != nil {
		return
	}
	events, err := client.TaskCalendarEvent.Query().Where(taskcalendarevent.UserIDEQ(userID)).All(ctx)
	if err != nil {
		log.Printf("calendarsync: failed to load calendar events for user %s: %v", userID, err)
		return
	}
	for _, e := range events {
		deleteEventForUser(ctx, client, calCfg, encKey, u, e.TaskID)
	}
}

func deleteEventForUser(ctx context.Context, client *ent.Client, calCfg *oauth2.Config, encKey []byte, u *ent.User, taskID uuid.UUID) {
	existing, err := client.TaskCalendarEvent.Query().
		Where(taskcalendarevent.TaskIDEQ(taskID), taskcalendarevent.UserIDEQ(u.ID)).
		Only(ctx)
	if err != nil {
		return // 連携イベントが無ければ何もしない（ent.IsNotFound含む）
	}
	if u.GoogleRefreshToken != nil {
		refreshToken, err := internalauth.DecryptToken(encKey, *u.GoogleRefreshToken)
		if err == nil {
			if calClient, err := googlecalendar.NewClient(ctx, calCfg, refreshToken); err == nil {
				if err := calClient.DeleteEvent(ctx, existing.GoogleEventID); err != nil {
					log.Printf("calendarsync: failed to delete google event for task %s user %s: %v", taskID, u.ID, err)
				}
			}
		}
	}
	if err := client.TaskCalendarEvent.DeleteOne(existing).Exec(ctx); err != nil {
		log.Printf("calendarsync: failed to delete task_calendar_events row for task %s user %s: %v", taskID, u.ID, err)
	}
}

// eventDescription はイベント説明欄の文言を組み立てる（プロジェクト名＋タスク詳細への
// フロントリンク）。プロジェクト取得に失敗してもリンクだけは含めて返す（低確度の見た目、
// 実機確認時に調整する。docs/aibo/m6-implementation-plan.md 設計判断3参照）。
// 2026-08-27追加：開始日時・期限のどちらか一方しか設定されていないタスクは、
// イベントの表示時間帯がresolveEventWindowによる補完（defaultEventDuration分の
// 仮の予定枠）であって実際の範囲ではないことが説明欄からも分かるよう、
// その旨を先頭に注記する。
func eventDescription(ctx context.Context, client *ent.Client, t *ent.Task, frontendURL string) string {
	link := fmt.Sprintf("%s/w/%s/my-tasks?task=%s", frontendURL, t.WorkspaceID, t.ID)

	var note string
	switch {
	case t.StartDate != nil && t.DueDate == nil:
		note = "※期限は未設定です（表示中の時間帯は開始日時のみを基にした仮の予定枠です）\n"
	case t.StartDate == nil && t.DueDate != nil:
		note = "※開始日時は未設定です（表示中の時間帯は期限のみを基にした仮の予定枠です）\n"
	}

	if t.ProjectID == nil {
		return note + link
	}
	p, err := client.Project.Get(ctx, *t.ProjectID)
	if err != nil {
		return note + link
	}
	return fmt.Sprintf("%sプロジェクト: %s\n%s", note, p.Name, link)
}

// handleSyncFailure はGoogle Calendar API呼び出し失敗時の共通処理。
// リフレッシュトークン失効ならそのユーザーの連携を無効化し(spec.md 5章「失効時は
// 再認証を促す」)、それ以外はactivity_logsに記録してその場ではリトライしない
// （api-spec.md「非同期処理・外部連携」）。
func handleSyncFailure(ctx context.Context, client *ent.Client, u *ent.User, t *ent.Task, syncErr error) error {
	if googlecalendar.IsInvalidGrantError(syncErr) {
		_, err := client.User.UpdateOneID(u.ID).
			SetCalendarSyncEnabled(false).
			ClearGoogleRefreshToken().
			ClearCalendarSyncMode().
			Save(ctx)
		if err != nil {
			log.Printf("calendarsync: failed to disable sync for user %s after invalid_grant: %v", u.ID, err)
		}
		return syncErr
	}

	log.Printf("calendarsync: sync failed for task %s user %s: %v", t.ID, u.ID, syncErr)
	if _, err := client.ActivityLog.Create().
		SetWorkspaceID(t.WorkspaceID).
		SetTaskID(t.ID).
		SetNillableProjectID(t.ProjectID).
		SetActorID(u.ID).
		SetActionType("task.calendar_sync_failed").
		SetPayload(map[string]any{"error": syncErr.Error()}).
		Save(ctx); err != nil {
		log.Printf("calendarsync: failed to record sync failure activity log: %v", err)
	}
	return syncErr
}
