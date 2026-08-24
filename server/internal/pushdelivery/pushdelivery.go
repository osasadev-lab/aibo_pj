// Package pushdelivery はアプリ内通知の作成に付随して、Web Push（VAPID）で
// ブラウザへベストエフォート配信する（M7）。
//
// calendarsyncパッケージと同じ「DBトランザクションの外・コミット後にgoroutineで
// 実行する」方針（docs/aibo/m7-implementation-plan.md 設計判断7）。呼び出し元の
// ハンドラは必ずwithTxの成功後にAsync()経由で呼ぶこと。
package pushdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/pushsubscription"
)

// asyncTimeout はAsync()で起動するgoroutine1回あたりの上限
// （calendarsync.asyncTimeoutと同じ考え方）。
const asyncTimeout = 20 * time.Second

// Config はVAPID関連の3値をまとめたもの（calCfg/encKeyと同じ渡し方をする）。
type Config struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
}

// Item は1ユーザーへ配信する1件のPush通知。
type Item struct {
	UserID uuid.UUID
	Title  string
	Body   string
	URL    string // クリック時に開くフロントURL
}

type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// Async はitemsをgoroutine内でベストエフォート配信する（呼び出し元はDBコミット後に呼ぶこと。
// c.Request.Context()はハンドラreturn後にキャンセルされるため使えない、calendarsync.Asyncと同じ理由）。
func Async(client *ent.Client, cfg Config, items []Item) {
	if len(items) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), asyncTimeout)
		defer cancel()
		for _, item := range items {
			deliver(ctx, client, cfg, item)
		}
	}()
}

// deliver は1ユーザー分。そのユーザーのpush_subscriptions全件へ送信を試みる。
func deliver(ctx context.Context, client *ent.Client, cfg Config, item Item) {
	subs, err := client.PushSubscription.Query().Where(pushsubscription.UserIDEQ(item.UserID)).All(ctx)
	if err != nil {
		log.Printf("pushdelivery: failed to load subscriptions for user %s: %v", item.UserID, err)
		return
	}
	if len(subs) == 0 {
		return
	}

	message, err := json.Marshal(pushPayload{Title: item.Title, Body: item.Body, URL: item.URL})
	if err != nil {
		log.Printf("pushdelivery: failed to marshal payload for user %s: %v", item.UserID, err)
		return
	}

	for _, sub := range subs {
		resp, err := webpush.SendNotificationWithContext(ctx, message, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				Auth:   sub.Auth,
				P256dh: sub.P256dh,
			},
		}, &webpush.Options{
			Subscriber:      cfg.VAPIDSubject,
			VAPIDPublicKey:  cfg.VAPIDPublicKey,
			VAPIDPrivateKey: cfg.VAPIDPrivateKey,
			TTL:             60 * 60,
		})
		if err != nil {
			log.Printf("pushdelivery: failed to send to subscription %s: %v", sub.ID, err)
			continue
		}
		resp.Body.Close()

		// 410 Gone / 404 Not Foundは購読が失効している（ブラウザ側でunsubscribeされた等）
		// ため、そのままDBから削除する。
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			if err := client.PushSubscription.DeleteOne(sub).Exec(ctx); err != nil {
				log.Printf("pushdelivery: failed to delete stale subscription %s: %v", sub.ID, err)
			}
		}
	}
}

// BuildItem は通知種別・payloadから、Push通知用の短いtitle/bodyを組み立てる。
// NotificationsPanel.tsxのdescribeNotificationと同じ出し分けロジックをGo側にも
// 持つ形になる（Push通知のtitle/bodyはクライアント側でAPIを叩いて動的生成できないため、
// フロント/バックエンドでの文言重複は許容する）。
func BuildItem(userID uuid.UUID, frontendURL, workspaceID, notifType string, payload map[string]any) Item {
	title, body, path := describe(workspaceID, notifType, payload)
	return Item{
		UserID: userID,
		Title:  title,
		Body:   body,
		URL:    frontendURL + path,
	}
}

func describe(workspaceID, notifType string, payload map[string]any) (title, body, path string) {
	str := func(key string) string {
		if v, ok := payload[key].(string); ok {
			return v
		}
		return ""
	}

	switch notifType {
	case "mentioned":
		by := str("mentioned_by_name")
		if by == "" {
			by = "誰か"
		}
		return "メンションされました", fmt.Sprintf("%sさんにメンションされました: %s", by, str("excerpt")), taskPath(workspaceID, payload)
	case "assigned":
		by := str("changed_by_name")
		if by == "" {
			by = "誰か"
		}
		return "タスクの担当者に追加されました", fmt.Sprintf("%sさんがあなたを担当者に追加しました: %s", by, str("task_title")), taskPath(workspaceID, payload)
	case "unassigned":
		by := str("changed_by_name")
		if by == "" {
			by = "誰か"
		}
		return "タスクの担当者から外れました", fmt.Sprintf("%sさんがあなたを担当者から外しました: %s", by, str("task_title")), taskPath(workspaceID, payload)
	case "project_joined":
		by := str("changed_by_name")
		if by == "" {
			by = "誰か"
		}
		return "プロジェクトに追加されました", fmt.Sprintf("%sさんがあなたをプロジェクトに追加しました: %s", by, str("project_name")), projectPath(workspaceID, payload)
	case "project_removed":
		by := str("changed_by_name")
		if by == "" {
			by = "誰か"
		}
		return "プロジェクトから外れました", fmt.Sprintf("%sさんがあなたをプロジェクトから外しました: %s", by, str("project_name")), projectPath(workspaceID, payload)
	case "project_created":
		by := str("changed_by_name")
		if by == "" {
			by = "誰か"
		}
		return "新しいプロジェクト", fmt.Sprintf("%sさんが新しいプロジェクトを作成しました: %s", by, str("project_name")), projectPath(workspaceID, payload)
	case "project_deleted":
		by := str("changed_by_name")
		if by == "" {
			by = "誰か"
		}
		return "プロジェクトが削除されました", fmt.Sprintf("%sさんがプロジェクトを削除しました: %s", by, str("project_name")), ""
	case "due_today_summary":
		return "本日期限のタスク", summaryBody(payload, "本日期限のタスクが%d件あります"), summaryPath(payload)
	case "overdue_summary":
		return "期限を過ぎているタスク", summaryBody(payload, "期限を過ぎているタスクが%d件あります"), summaryPath(payload)
	default:
		return "通知", notifType, ""
	}
}

func summaryBody(payload map[string]any, format string) string {
	count := 0
	if v, ok := payload["task_count"].(float64); ok {
		count = int(v)
	}
	titles := summaryTitles(payload, 3)
	base := fmt.Sprintf(format, count)
	if len(titles) == 0 {
		return base
	}
	return base + ": " + strings.Join(titles, "、")
}

func summaryTitles(payload map[string]any, limit int) []string {
	tasks, ok := payload["tasks"].([]any)
	if !ok {
		return nil
	}
	titles := make([]string, 0, limit)
	for _, t := range tasks {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if title, ok := m["title"].(string); ok {
			titles = append(titles, title)
		}
		if len(titles) >= limit {
			break
		}
	}
	return titles
}

// summaryPath は集約通知内、先頭タスクへのリンク（設計判断3：タスクごとにworkspace_idを
// payloadへ明示しているため、表示中ワークスペースに依存せず正しい遷移先を組み立てられる）。
func summaryPath(payload map[string]any) string {
	tasks, ok := payload["tasks"].([]any)
	if !ok || len(tasks) == 0 {
		return ""
	}
	first, ok := tasks[0].(map[string]any)
	if !ok {
		return ""
	}
	workspaceID := idString(first["workspace_id"])
	taskID := idString(first["task_id"])
	if workspaceID == "" || taskID == "" {
		return ""
	}
	if projectID := idString(first["project_id"]); projectID != "" {
		return fmt.Sprintf("/w/%s/projects/%s?task=%s", workspaceID, projectID, taskID)
	}
	return fmt.Sprintf("/w/%s/my-tasks?task=%s", workspaceID, taskID)
}

// taskPath はmentioned/assigned/unassigned用。既存の通知payloadはworkspace_idを
// 持たない（NotificationsPanel.tsxが表示中ワークスペースを流用する既存の割り切りと同じ）ため、
// BuildItemの呼び出し元（そのハンドラが今扱っているworkspace_id）を代わりに使う。
func taskPath(workspaceID string, payload map[string]any) string {
	taskID := idString(payload["task_id"])
	if workspaceID == "" || taskID == "" {
		return ""
	}
	if projectID := idString(payload["project_id"]); projectID != "" {
		return fmt.Sprintf("/w/%s/projects/%s?task=%s", workspaceID, projectID, taskID)
	}
	return fmt.Sprintf("/w/%s/my-tasks?task=%s", workspaceID, taskID)
}

func projectPath(workspaceID string, payload map[string]any) string {
	projectID := idString(payload["project_id"])
	if workspaceID == "" || projectID == "" {
		return ""
	}
	return fmt.Sprintf("/w/%s/projects/%s", workspaceID, projectID)
}

// idString はpayload内のID系フィールドを文字列化する。BuildItemはDB保存前の
// map（値はuuid.UUID/*uuid.UUID）とDBから読み戻したJSON（値はstring）の両方から
// 呼ばれうるため、両方のケースを吸収する。
func idString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case uuid.UUID:
		return t.String()
	case *uuid.UUID:
		if t == nil {
			return ""
		}
		return t.String()
	default:
		return ""
	}
}
