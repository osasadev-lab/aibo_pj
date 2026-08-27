package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/remindersend"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/user"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
)

// jst は「今日」「期限超過」の判定に使う固定タイムゾーン（M7設計判断10、
// ワークスペース/ユーザーごとのタイムゾーン設定には対応しない）。
var jst = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		// ローカル環境等でタイムゾーンDBが無い場合のフォールバック。
		return time.FixedZone("Asia/Tokyo", 9*60*60)
	}
	return loc
}()

// maxSummaryTasks は集約リマインダー通知1件に含めるタスクの最大件数
// （docs/aibo/m7-implementation-plan.md 設計判断3・リマインダー要件5）。
const maxSummaryTasks = 5

// CronHandler は /internal/cron 配下（Cloud Schedulerからの内部呼び出し専用、
// RequireAuthを経由しない）を扱う。
type CronHandler struct {
	client      *ent.Client
	pushCfg     pushdelivery.Config
	frontendURL string
}

func NewCronHandler(client *ent.Client, pushCfg pushdelivery.Config, frontendURL string) *CronHandler {
	return &CronHandler{client: client, pushCfg: pushCfg, frontendURL: frontendURL}
}

// Reminders は POST /internal/cron/reminders。Cloud Schedulerから15分ごとに呼ばれる想定
// （docs/aibo/m7-implementation-plan.md 設計判断1）。各ユーザーへの送信は1日1回のみ
// （reminder_sendsで冪等性を担保、同一スロットの二重実行や日をまたいだ再実行に強い）。
func (h *CronHandler) Reminders(c *gin.Context) {
	ctx := c.Request.Context()

	now := time.Now().In(jst)
	currentSlot := fmt.Sprintf("%02d:%02d", now.Hour(), (now.Minute()/15)*15)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, jst)

	users, err := h.client.User.Query().
		Where(user.Or(
			user.And(user.ReminderDueTodayEnabledEQ(true), user.ReminderDueTodayTimeEQ(currentSlot)),
			user.And(user.ReminderOverdueEnabledEQ(true), user.ReminderOverdueTimeEQ(currentSlot)),
		)).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load reminder users"})
		return
	}

	var pendingPush []pushdelivery.Item
	notified := 0
	for _, u := range users {
		if u.ReminderDueTodayEnabled && u.ReminderDueTodayTime != nil && *u.ReminderDueTodayTime == currentSlot {
			if h.processReminder(ctx, u, remindersend.TypeDueToday, today, &pendingPush) {
				notified++
			}
		}
		if u.ReminderOverdueEnabled && u.ReminderOverdueTime != nil && *u.ReminderOverdueTime == currentSlot {
			if h.processReminder(ctx, u, remindersend.TypeOverdue, today, &pendingPush) {
				notified++
			}
		}
	}

	pushdelivery.Async(h.client, h.pushCfg, pendingPush)

	c.JSON(http.StatusOK, gin.H{"matched_users": len(users), "notified": notified})
}

// processReminder は1ユーザー・1種別（due_today/overdue）分の処理。
// 既に本日分を送信済みならスキップする。対象タスクが1件以上あれば集約通知
// （payload: task_count, tasks: [{task_id, workspace_id, project_id, title}] 最大5件）を
// 作成し、pendingへWeb Push配信用のitemを追記する。戻り値は通知を作成したかどうか。
func (h *CronHandler) processReminder(ctx context.Context, u *ent.User, remType remindersend.Type, today time.Time, pending *[]pushdelivery.Item) bool {
	alreadySent, err := h.client.ReminderSend.Query().
		Where(remindersend.UserIDEQ(u.ID), remindersend.TypeEQ(remType), remindersend.SentDateEQ(today)).
		Exist(ctx)
	if err != nil {
		log.Printf("cron: failed to check reminder_sends for user %s type %s: %v", u.ID, remType, err)
		return false
	}
	if alreadySent {
		return false
	}

	query := h.client.Task.Query().Where(
		task.HasAssigneesWith(taskassignee.UserIDEQ(u.ID)),
		task.StatusNEQ(task.StatusDone),
	)
	if remType == remindersend.TypeDueToday {
		// due_dateが時刻を持つようになったため、当日中（今日0時〜翌日0時未満）を
		// 範囲で判定する（等価比較のままだと午前0時ちょうど以外一致しなくなる）。
		query = query.Where(task.DueDateGTE(today), task.DueDateLT(today.AddDate(0, 0, 1)))
	} else {
		query = query.Where(task.DueDateLT(today))
	}
	tasks, err := query.All(ctx)
	if err != nil {
		log.Printf("cron: failed to load tasks for user %s type %s: %v", u.ID, remType, err)
		return false
	}

	notifType := "due_today_summary"
	if remType == remindersend.TypeOverdue {
		notifType = "overdue_summary"
	}

	created := false
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.ReminderSend.Create().
			SetUserID(u.ID).
			SetType(remType).
			SetSentDate(today).
			Save(ctx); err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}

		taskPayloads := make([]any, 0, maxSummaryTasks)
		for i, t := range tasks {
			if i >= maxSummaryTasks {
				break
			}
			entry := map[string]any{
				"task_id":      t.ID.String(),
				"workspace_id": t.WorkspaceID.String(),
				"title":        t.Title,
			}
			if t.ProjectID != nil {
				entry["project_id"] = t.ProjectID.String()
			}
			taskPayloads = append(taskPayloads, entry)
		}
		payload := map[string]any{
			"task_count": len(tasks),
			"tasks":      taskPayloads,
		}
		if _, err := tx.Notification.Create().
			SetUserID(u.ID).
			SetType(notifType).
			SetPayload(payload).
			Save(ctx); err != nil {
			return err
		}
		// workspaceIDは通知種別ごとに複数ありうるため、pushdelivery側はpayload内の
		// tasks[0].workspace_idを使う（BuildItemの第3引数は使われない、設計判断3）。
		*pending = append(*pending, pushdelivery.BuildItem(u.ID, h.frontendURL, "", notifType, payload))
		created = true
		return nil
	})
	if err != nil {
		log.Printf("cron: failed to record reminder for user %s type %s: %v", u.ID, remType, err)
		return false
	}
	return created
}
