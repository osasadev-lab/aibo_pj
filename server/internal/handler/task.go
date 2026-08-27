package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"entgo.io/ent/dialect/sql"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/activitylog"
	"github.com/osasadev-lab/aibo_pj/server/ent/attachment"
	"github.com/osasadev-lab/aibo_pj/server/ent/comment"
	"github.com/osasadev-lab/aibo_pj/server/ent/commentmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmchannel"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmchannelmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmmessage"
	"github.com/osasadev-lab/aibo_pj/server/ent/project"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectstatuscolumn"
	"github.com/osasadev-lab/aibo_pj/server/ent/reaction"
	"github.com/osasadev-lab/aibo_pj/server/ent/section"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskcalendarevent"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskdependency"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskpin"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/calendarsync"
	"github.com/osasadev-lab/aibo_pj/server/internal/githubissue"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"

	"golang.org/x/oauth2"
)

const dateLayout = "2006-01-02"

// dateTimeLayoutはタスクの開始日時・期限（2026-08-27追加、日時範囲での管理）専用。
// 基準日フィルタ等、日単位で十分な箇所は引き続きdateLayoutを使う。
const dateTimeLayout = "2006-01-02 15:04"

var errInvalidIDs = errors.New("one or more ids are invalid")

// TaskHandler は /tasks, /workspaces/:workspace_id/tasks 配下を扱う。
type TaskHandler struct {
	client *ent.Client
	r2     *storage.R2Client
	// calCfg/encKey/frontendURLはM6（Googleカレンダー自動同期）用。
	// タスクのcreate/update/delete/担当者変更のたびにcalendarsync.SyncTask系を呼ぶ。
	calCfg      *oauth2.Config
	encKey      []byte
	frontendURL string
	// pushCfgはM7（Web Push配信）用。
	pushCfg pushdelivery.Config
}

// GitHub Issue連携（M8.5後追加）用のトークンはワークスペースごとにOwnerが設定し
// workspaces.github_tokenへ暗号化して保存する（グローバルなAPIキーではない）ため、
// TaskHandlerはConfigを固定で持たず、都度h.encKeyで復号する（task_github_issue.go）。
func NewTaskHandler(client *ent.Client, r2 *storage.R2Client, calCfg *oauth2.Config, encKey []byte, frontendURL string, pushCfg pushdelivery.Config) *TaskHandler {
	return &TaskHandler{client: client, r2: r2, calCfg: calCfg, encKey: encKey, frontendURL: frontendURL, pushCfg: pushCfg}
}

func taskJSON(t *ent.Task) gin.H {
	row := gin.H{
		"id":               t.ID,
		"workspace_id":     t.WorkspaceID,
		"project_id":       t.ProjectID,
		"section_id":       t.SectionID,
		"status":           t.Status,
		"status_column_id": t.StatusColumnID,
		"parent_task_id":   t.ParentTaskID,
		"title":            t.Title,
		"description":      t.Description,
		"priority":         t.Priority,
		"start_date":       formatDateTime(t.StartDate),
		"due_date":         formatDateTime(t.DueDate),
		"created_by":       t.CreatedBy,
		"github_issue_url": t.GithubIssueURL,
	}
	// assigneesがeager-load済み（WithAssignees()）の場合のみassignee_idsを含める。
	if t.Edges.Assignees != nil {
		ids := make([]uuid.UUID, 0, len(t.Edges.Assignees))
		for _, a := range t.Edges.Assignees {
			ids = append(ids, a.UserID)
		}
		row["assignee_ids"] = ids
	}
	// mentionsがeager-load済み（WithMentions()）の場合のみmentioned_user_idsを含める。
	if t.Edges.Mentions != nil {
		ids := make([]uuid.UUID, 0, len(t.Edges.Mentions))
		for _, tm := range t.Edges.Mentions {
			ids = append(ids, tm.MentionedUserID)
		}
		row["mentioned_user_ids"] = ids
	}
	// tagsがeager-load済み（WithTags(func(q){ q.WithTag() })）の場合のみtagsを含める。
	if t.Edges.Tags != nil {
		tags := make([]gin.H, 0, len(t.Edges.Tags))
		for _, tt := range t.Edges.Tags {
			if tt.Edges.Tag == nil {
				continue
			}
			tags = append(tags, tagJSON(tt.Edges.Tag))
		}
		row["tags"] = tags
	}
	// dependenciesがeager-load済み（WithDependencies(func(q){ q.WithDependsOn() })）の
	// 場合のみhas_incomplete_dependenciesを含める（先行タスクにstatus!=doneが1件でも
	// あればtrue。カンバンカード・タスク詳細のバッジ表示用、spec.md 4.6）。
	if t.Edges.Dependencies != nil {
		incomplete := false
		ids := make([]uuid.UUID, 0, len(t.Edges.Dependencies))
		for _, d := range t.Edges.Dependencies {
			ids = append(ids, d.DependsOnTaskID)
			if d.Edges.DependsOn != nil && d.Edges.DependsOn.Status != task.StatusDone {
				incomplete = true
			}
		}
		row["has_incomplete_dependencies"] = incomplete
		// depends_on_task_idsはプロジェクトカンバンのホバー強調（個人設定、
		// docs/aibo/m4-implementation-plan.md 4章）がAPIを追加で叩かずクライアント側
		// だけで先行/後続関係を判定できるようにするための付随情報。「後続」側は
		// このtask_idを他タスクのdepends_on_task_idsから逆引きすればよいため、
		// パフォーマンス上の理由でDependentsは別途eager-loadしない（往復が1回減る）。
		row["depends_on_task_ids"] = ids
	}
	return row
}

func formatDateTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.In(jst).Format(dateTimeLayout)
	return &s
}

// Search は GET /workspaces/:workspace_id/tasks。
// 可視性フィルタ（単体タスクはworkspace全員、publicプロジェクトは全員、
// privateプロジェクトは参画メンバーのみ）を必ず適用する。
func (h *TaskHandler) Search(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	query := h.client.Task.Query().Where(
		task.WorkspaceIDEQ(m.WorkspaceID),
		task.Or(
			task.ProjectIDIsNil(),
			task.HasProjectWith(project.VisibilityEQ(project.VisibilityPublic)),
			task.HasProjectWith(project.HasMembersWith(projectmember.UserIDEQ(m.UserID))),
		),
	)

	if v := c.Query("project_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
			return
		}
		query = query.Where(task.ProjectIDEQ(id))
	}
	if v := c.Query("assignee_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignee_id"})
			return
		}
		query = query.Where(task.HasAssigneesWith(taskassignee.UserIDEQ(id)))
	}
	if v := c.Query("status"); v != "" {
		query = query.Where(task.StatusEQ(task.Status(v)))
	}
	if v := c.Query("status_column_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status_column_id"})
			return
		}
		query = query.Where(task.StatusColumnIDEQ(id))
	}
	if v := c.Query("due_before"); v != "" {
		d, err := time.ParseInLocation(dateLayout, v, jst)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_before"})
			return
		}
		query = query.Where(task.DueDateLT(d))
	}

	tasks, err := query.
		// M8追加：プロジェクトカンバンの同一列内D&D並び替え用。positionが未設定
		// （nil）のタスクはNullsLastでcreated_at順の末尾寄せに扱う（段階的移行、
		// docs/aibo/m8-implementation-plan.md スコープ追加A参照）。
		Order(task.ByStatusColumnID(), task.ByPosition(sql.OrderNullsLast()), task.ByCreatedAt()).
		WithAssignees().
		WithDependencies(func(q *ent.TaskDependencyQuery) { q.WithDependsOn() }).
		// Tagsはプロジェクトカンバンのタグバッジ・ホバー強調（tagモード）用。
		// MyTasksでは対象外（ユーザー確認済み）のためSearchのみ追加する。Dependentsは
		// あえてeager-loadしない（他タスクのdepends_on_task_idsから逆引きできるため、
		// 往復を1回減らすパフォーマンス上の判断）。
		WithTags(func(q *ent.TaskTagQuery) { q.WithTag() }).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search tasks"})
		return
	}

	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskJSON(t))
	}
	c.JSON(http.StatusOK, out)
}

// maxSearchResults は全文検索1回あたりの結果件数上限（M7スコープ外のページネーションの
// 代わりに、まずは直近更新順で上位のみ返す簡易な割り切り）。
const maxSearchResults = 50

// FullTextSearch は GET /workspaces/:workspace_id/search?q=。
// タスクのtitle/description/コメント本文/添付ファイル名を対象にした部分一致検索
// （docs/aibo/m7-implementation-plan.md 設計判断5・6）。Searchと同じ可視性フィルタを
// 適用した上で、qが空なら空配列を返す。
// DMメッセージも検索対象に含める（2026-08-28追加、ユーザー要望）。ただし自分が
// 参加しているこのワークスペースのDMチャンネルのメッセージのみを対象にし、
// やりとりしていない他人同士のDMは一切検索結果に出さない。
func (h *TaskHandler) FullTextSearch(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"tasks": []gin.H{}, "dm_messages": []gin.H{}})
		return
	}

	tasks, err := h.client.Task.Query().
		Where(
			task.WorkspaceIDEQ(m.WorkspaceID),
			task.Or(
				task.ProjectIDIsNil(),
				task.HasProjectWith(project.VisibilityEQ(project.VisibilityPublic)),
				task.HasProjectWith(project.HasMembersWith(projectmember.UserIDEQ(m.UserID))),
			),
			task.Or(
				task.TitleContainsFold(q),
				task.DescriptionContainsFold(q),
				task.HasCommentsWith(comment.BodyContainsFold(q)),
				task.HasAttachmentsWith(attachment.FileNameContainsFold(q)),
			),
		).
		Order(task.ByUpdatedAt(sql.OrderDesc())).
		Limit(maxSearchResults).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search tasks"})
		return
	}

	// どの項目に、どの内容で一致したかをフロントで表示するため（ユーザーフィードバック、
	// タスク/説明/コメント/添付ファイルの4区分ごとに「ヒットなし」or一致した内容を
	// 一覧表示したい、という具体的なレイアウト要望）。コメント・添付ファイルは1タスクに
	// 複数一致しうるため、それぞれの一致内容（抜粋・ファイル名）をタスクID単位で集める
	// （N+1を避けるためtask_id INの一括クエリ2回、tasksが0件なら実行しない）。
	commentMatches := map[uuid.UUID][]string{}
	attachmentMatches := map[uuid.UUID][]string{}
	if len(tasks) > 0 {
		taskIDs := make([]uuid.UUID, 0, len(tasks))
		for _, t := range tasks {
			taskIDs = append(taskIDs, t.ID)
		}

		matchedComments, err := h.client.Comment.Query().
			Where(comment.TaskIDIn(taskIDs...), comment.BodyContainsFold(q)).
			Order(comment.ByCreatedAt()).
			All(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search tasks"})
			return
		}
		for _, cm := range matchedComments {
			commentMatches[cm.TaskID] = append(commentMatches[cm.TaskID], excerpt(cm.Body, 80))
		}

		matchedAttachments, err := h.client.Attachment.Query().
			Where(attachment.TaskIDIn(taskIDs...), attachment.FileNameContainsFold(q)).
			Order(attachment.ByCreatedAt()).
			All(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search tasks"})
			return
		}
		for _, a := range matchedAttachments {
			attachmentMatches[a.TaskID] = append(attachmentMatches[a.TaskID], a.FileName)
		}
	}

	lowerQ := strings.ToLower(q)
	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		row := taskJSON(t)
		matches := gin.H{
			"comments":    orEmptyStrings(commentMatches[t.ID]),
			"attachments": orEmptyStrings(attachmentMatches[t.ID]),
		}
		if strings.Contains(strings.ToLower(t.Title), lowerQ) {
			matches["title"] = t.Title
		}
		if t.Description != nil && strings.Contains(strings.ToLower(*t.Description), lowerQ) {
			matches["description"] = excerpt(*t.Description, 150)
		}
		row["matches"] = matches
		out = append(out, row)
	}

	// DMメッセージ検索（2026-08-28追加）。自分がメンバーとして参加しているこの
	// ワークスペースのDMチャンネルに限定し、他人同士のやりとりは対象にしない。
	dmMessages, err := h.client.DMMessage.Query().
		Where(
			dmmessage.HasChannelWith(
				dmchannel.WorkspaceIDEQ(m.WorkspaceID),
				dmchannel.HasMembersWith(dmchannelmember.UserIDEQ(m.UserID)),
			),
			dmmessage.BodyContainsFold(q),
		).
		WithUser().
		WithChannel(func(cq *ent.DMChannelQuery) {
			cq.WithMembers(func(mq *ent.DMChannelMemberQuery) { mq.WithUser() })
		}).
		Order(dmmessage.ByCreatedAt(sql.OrderDesc())).
		Limit(maxSearchResults).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search dm messages"})
		return
	}
	dmOut := make([]gin.H, 0, len(dmMessages))
	for _, msg := range dmMessages {
		if msg.Body == nil || msg.Edges.Channel == nil {
			continue
		}
		ch := msg.Edges.Channel
		users := make([]*ent.User, 0, len(ch.Edges.Members))
		for _, mem := range ch.Edges.Members {
			if mem.Edges.User != nil {
				users = append(users, mem.Edges.User)
			}
		}
		dmOut = append(dmOut, gin.H{
			"channel_id":   ch.ID,
			"channel_name": displayName(ch, users, m.UserID),
			"message_id":   msg.ID,
			"sender_name":  msg.Edges.User.Name,
			"excerpt":      excerpt(*msg.Body, 100),
			"created_at":   msg.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tasks": out, "dm_messages": dmOut})
}

// orEmptyStrings はnilスライスをJSON上で`null`ではなく`[]`にするためのヘルパー
// （フロントが常に配列として扱えるようにする）。
func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// MyTasks は GET /workspaces/:workspace_id/my-tasks。
// 呼び出しユーザーがassigneeになっているタスクをプロジェクト横断で共通statusで
// 集約する。可視性フィルタも防御的に適用する（担当者なら通常閲覧可能なはずだが、
// Searchと同じ絞り込みを維持する）。
// ?dateで基準日を指定すると、その日以前が期限のタスクだけに絞り込む
// （未指定時は今まで通り全件表示、due_todayは基準日=今日で判定）。
func (h *TaskHandler) MyTasks(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	query := h.client.Task.Query().Where(
		task.WorkspaceIDEQ(m.WorkspaceID),
		task.HasAssigneesWith(taskassignee.UserIDEQ(m.UserID)),
		task.Or(
			task.ProjectIDIsNil(),
			task.HasProjectWith(project.VisibilityEQ(project.VisibilityPublic)),
			task.HasProjectWith(project.HasMembersWith(projectmember.UserIDEQ(m.UserID))),
		),
	)
	if v := c.Query("status"); v != "" {
		query = query.Where(task.StatusEQ(task.Status(v)))
	}

	referenceDate := time.Now()
	if v := c.Query("date"); v != "" {
		d, err := time.ParseInLocation(dateLayout, v, jst)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date"})
			return
		}
		referenceDate = d
		// due_dateが時刻を持つようになったため、基準日当日中（〜翌日0時未満）を
		// 「基準日以前」に含める（LTEのままだと基準日当日の午後締切を取りこぼす）。
		// 期限が無く開始日時のみのタスクは開始日時で判定する（2026-08-27、
		// 「日次範囲が片方だけでもヒットするように」との要望）。
		dayEnd := d.AddDate(0, 0, 1)
		query = query.Where(task.Or(
			task.DueDateLT(dayEnd),
			task.And(task.DueDateIsNil(), task.StartDateNotNil(), task.StartDateLT(dayEnd)),
		))
	}

	tasks, err := query.
		WithAssignees().
		WithDependencies(func(q *ent.TaskDependencyQuery) { q.WithDependsOn() }).
		WithTags(func(q *ent.TaskTagQuery) { q.WithTag() }).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list my-tasks"})
		return
	}

	refDateStr := referenceDate.In(jst).Format(dateLayout)
	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		row := taskJSON(t)
		// due_todayも同様に、期限が無ければ開始日時を基準日と比較する。
		effectiveDate := t.DueDate
		if effectiveDate == nil {
			effectiveDate = t.StartDate
		}
		row["due_today"] = effectiveDate != nil && effectiveDate.In(jst).Format(dateLayout) == refDateStr
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}

type createTaskRequest struct {
	Title          string      `json:"title" binding:"required"`
	Description    *string     `json:"description"`
	ProjectID      *uuid.UUID  `json:"project_id"`
	SectionID      *uuid.UUID  `json:"section_id"`
	Status         *string     `json:"status" binding:"omitempty,oneof=not_started in_progress done on_hold"`
	StatusColumnID *uuid.UUID  `json:"status_column_id"`
	Priority       *string     `json:"priority" binding:"omitempty,oneof=low medium high"`
	StartDate      *string     `json:"start_date"`
	DueDate        *string     `json:"due_date"`
	AssigneeIDs    []uuid.UUID `json:"assignee_ids"`
	TagIDs         []uuid.UUID `json:"tag_ids"`
	GithubIssueURL *string     `json:"github_issue_url"`
}

// Create は POST /workspaces/:workspace_id/tasks。project_id未指定なら単体タスク。
func (h *TaskHandler) Create(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	var proj *ent.Project
	if req.ProjectID != nil {
		p, err := h.client.Project.Get(ctx, *req.ProjectID)
		if err != nil || p.WorkspaceID != m.WorkspaceID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "project not found in this workspace"})
			return
		}
		if err := middleware.CheckProjectVisibility(ctx, h.client, p, u.ID); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "not a project member"})
			return
		}
		proj = p

		if req.SectionID != nil {
			ok, err := h.client.Section.Query().
				Where(section.IDEQ(*req.SectionID), section.ProjectIDEQ(proj.ID)).
				Exist(ctx)
			if err != nil || !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "section not found in this project"})
				return
			}
		}
	} else if req.SectionID != nil || req.StatusColumnID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "section_id/status_column_id require project_id"})
		return
	}

	startDate, err := parseDateTimeInput(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date"})
		return
	}
	dueDate, err := parseDateTimeInput(req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_date"})
		return
	}
	if err := validateDateRange(startDate, dueDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	assigneeIDs, err := h.validWorkspaceUserIDs(ctx, m.WorkspaceID, req.AssigneeIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assignee_ids must all be workspace members"})
		return
	}
	var projID *uuid.UUID
	if proj != nil {
		projID = &proj.ID
	}
	tagIDs, err := h.validTaskTagIDs(ctx, m.WorkspaceID, projID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must be assignable to this task"})
		return
	}

	var created *ent.Task
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Task.Create().
			SetWorkspaceID(m.WorkspaceID).
			SetTitle(req.Title).
			SetCreatedBy(u.ID)
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
		}
		if req.Priority != nil {
			builder = builder.SetPriority(task.Priority(*req.Priority))
		}
		if req.GithubIssueURL != nil {
			builder = builder.SetGithubIssueURL(*req.GithubIssueURL)
		}
		if startDate.provided && !startDate.clear {
			builder = builder.SetStartDate(startDate.value)
		}
		if dueDate.provided && !dueDate.clear {
			builder = builder.SetDueDate(dueDate.value)
		}
		if req.SectionID != nil {
			builder = builder.SetSectionID(*req.SectionID)
		}

		if proj != nil {
			builder = builder.SetProjectID(proj.ID)
			statusColumnID, status, err := resolveStatusColumn(ctx, tx.Client(), proj.ID, req.StatusColumnID, req.Status)
			if err != nil {
				return err
			}
			if statusColumnID != nil {
				builder = builder.SetStatusColumnID(*statusColumnID)
			}
			builder = builder.SetStatus(status)
		} else if req.Status != nil {
			builder = builder.SetStatus(task.Status(*req.Status))
		}

		t, err := builder.Save(ctx)
		if err != nil {
			return err
		}

		if len(assigneeIDs) > 0 {
			builders := make([]*ent.TaskAssigneeCreate, 0, len(assigneeIDs))
			for _, uid := range assigneeIDs {
				builders = append(builders, tx.TaskAssignee.Create().SetTaskID(t.ID).SetUserID(uid))
			}
			if _, err := tx.TaskAssignee.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}
		if len(tagIDs) > 0 {
			builders := make([]*ent.TaskTagCreate, 0, len(tagIDs))
			for _, tid := range tagIDs {
				builders = append(builders, tx.TaskTag.Create().SetTaskID(t.ID).SetTagID(tid))
			}
			if _, err := tx.TaskTag.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}

		if err := activity.Record(ctx, tx, m.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.created",
			map[string]any{"title": t.Title}); err != nil {
			return err
		}

		created = t
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	// Googleカレンダー自動同期（M6）。token refresh + Calendar API呼び出しで数秒
	// かかることがあり、レスポンスを待たせると体感速度を損なうためバックグラウンド化
	// している（2026-08-21、実機で確認の上、同期呼び出しから変更した）。
	calendarsync.Async(func(ctx context.Context) {
		calendarsync.SyncTask(ctx, h.client, h.calCfg, h.encKey, created, h.frontendURL)
	})

	c.JSON(http.StatusCreated, taskJSON(created))
}

// Get は GET /tasks/:task_id。RequireTaskAccessで可視性確認済み。
// Get は GET /tasks/:task_id。assignee_ids/mentioned_user_ids/tagsを含めるための
// eager-loadはここでだけ行う（RequireTaskAccessは全タスク系エンドポイント共通のため
// 意図的に軽量化してある）。
func (h *TaskHandler) Get(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()
	full, err := h.client.Task.Query().
		Where(task.IDEQ(t.ID)).
		WithAssignees().
		WithMentions().
		WithTags(func(q *ent.TaskTagQuery) { q.WithTag() }).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task"})
		return
	}
	row := taskJSON(full)
	// M8追加：呼び出しユーザー視点のピン留め状態（タスク詳細のピンボタン用）。
	pinned, err := h.client.TaskPin.Query().
		Where(taskpin.UserIDEQ(u.ID), taskpin.TaskIDEQ(t.ID)).
		Exist(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task"})
		return
	}
	row["is_pinned"] = pinned
	reactions, err := loadTaskReactions(ctx, h.client, t.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task"})
		return
	}
	row["reactions"] = reactions
	c.JSON(http.StatusOK, row)
}

type updateTaskRequest struct {
	Title          *string    `json:"title"`
	Description    *string    `json:"description"`
	Status         *string    `json:"status" binding:"omitempty,oneof=not_started in_progress done on_hold"`
	StatusColumnID *uuid.UUID `json:"status_column_id"`
	Priority       *string    `json:"priority" binding:"omitempty,oneof=low medium high"`
	StartDate      *string    `json:"start_date"`
	DueDate        *string    `json:"due_date"`
	// ポインタにして「フィールド省略＝メンション不変」と「空配列＝全メンション解除」を
	// 区別する（ステータス変更等の部分PATCHが誤ってメンションを消さないようにするため）。
	MentionedUserIDs *[]uuid.UUID `json:"mentioned_user_ids"`
	// M8追加：プロジェクトカンバンの同一列内D&D並び替え用。列を跨ぐ移動時は
	// status_column_idと同時送信、同一列内の並び替え時はpositionのみ送信する
	// （docs/aibo/m8-implementation-plan.md スコープ追加A）。
	Position *int `json:"position"`
	// GitHub Issueへの手動リンク（M8.5後追加）。省略時は変更なし、空文字で解除。
	GithubIssueURL *string `json:"github_issue_url"`
}

// Update は PATCH /tasks/:task_id。
// カンバンD&Dはstatus_column_idを、マイタスクD&Dはstatusを送る想定で、
// サーバー側でもう一方を自動同期する（db-schema.md）。両方指定時はstatus_column_id優先。
func (h *TaskHandler) Update(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req updateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	startDate, err := parseDateTimeInput(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date"})
		return
	}
	dueDate, err := parseDateTimeInput(req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_date"})
		return
	}
	if err := validateDateRange(startDate, dueDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	var mentionIDs []uuid.UUID
	if req.MentionedUserIDs != nil {
		mentionIDs = dedupUUIDs(*req.MentionedUserIDs)
		if len(mentionIDs) > 0 {
			visibleIDs, err := middleware.TaskVisibleUserIDs(ctx, h.client, t)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate mentions"})
				return
			}
			visibleSet := map[uuid.UUID]struct{}{}
			for _, id := range visibleIDs {
				visibleSet[id] = struct{}{}
			}
			for _, id := range mentionIDs {
				if _, ok := visibleSet[id]; !ok {
					c.JSON(http.StatusBadRequest, gin.H{"error": "mentioned_user_ids must be visible to this task"})
					return
				}
			}
		}
	}

	statusChanging := req.Status != nil || req.StatusColumnID != nil
	previousStatus := t.Status

	var updated *ent.Task
	var pendingPush []pushdelivery.Item
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Task.UpdateOneID(t.ID)
		changes := map[string]any{}

		if req.Title != nil {
			builder = builder.SetTitle(*req.Title)
			changes["title"] = *req.Title
		}
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
			changes["description"] = *req.Description
		}
		if req.Priority != nil {
			builder = builder.SetPriority(task.Priority(*req.Priority))
			changes["priority"] = *req.Priority
		}
		if req.GithubIssueURL != nil {
			builder = builder.SetGithubIssueURL(*req.GithubIssueURL)
			changes["github_issue_url"] = *req.GithubIssueURL
		}
		if startDate.provided {
			if startDate.clear {
				builder = builder.ClearStartDate()
				changes["start_date"] = nil
			} else {
				builder = builder.SetStartDate(startDate.value)
				changes["start_date"] = *req.StartDate
			}
		}
		if dueDate.provided {
			if dueDate.clear {
				builder = builder.ClearDueDate()
				changes["due_date"] = nil
			} else {
				builder = builder.SetDueDate(dueDate.value)
				changes["due_date"] = *req.DueDate
			}
		}
		if req.Position != nil {
			builder = builder.SetPosition(*req.Position)
		}

		switch {
		case req.StatusColumnID != nil:
			col, err := tx.ProjectStatusColumn.Get(ctx, *req.StatusColumnID)
			if err != nil || t.ProjectID == nil || col.ProjectID != *t.ProjectID {
				return errInvalidIDs
			}
			builder = builder.SetStatusColumnID(col.ID).SetStatus(task.Status(col.MapsToStatus))
		case req.Status != nil:
			if t.ProjectID != nil {
				col, err := tx.ProjectStatusColumn.Query().
					Where(
						projectstatuscolumn.ProjectIDEQ(*t.ProjectID),
						projectstatuscolumn.MapsToStatusEQ(projectstatuscolumn.MapsToStatus(*req.Status)),
					).
					Order(projectstatuscolumn.ByPosition()).
					First(ctx)
				if err == nil {
					builder = builder.SetStatusColumnID(col.ID)
				} else {
					builder = builder.ClearStatusColumnID()
				}
			}
			builder = builder.SetStatus(task.Status(*req.Status))
		}

		var err error
		updated, err = builder.Save(ctx)
		if err != nil {
			return err
		}

		if req.MentionedUserIDs != nil {
			existing, err := tx.TaskMention.Query().Where(taskmention.TaskIDEQ(t.ID)).All(ctx)
			if err != nil {
				return err
			}
			oldSet := make(map[uuid.UUID]struct{}, len(existing))
			for _, tm := range existing {
				oldSet[tm.MentionedUserID] = struct{}{}
			}

			if _, err := tx.TaskMention.Delete().Where(taskmention.TaskIDEQ(t.ID)).Exec(ctx); err != nil {
				return err
			}
			for _, id := range mentionIDs {
				if _, err := tx.TaskMention.Create().SetTaskID(t.ID).SetMentionedUserID(id).Save(ctx); err != nil {
					return err
				}
			}

			description := t.Description
			if req.Description != nil {
				description = req.Description
			}
			var descText string
			if description != nil {
				descText = *description
			}
			for _, id := range mentionIDs {
				if _, wasMentioned := oldSet[id]; wasMentioned {
					continue
				}
				payload := map[string]any{
					"task_id":           t.ID,
					"project_id":        t.ProjectID,
					"mentioned_by":      u.ID,
					"mentioned_by_name": u.Name,
					"excerpt":           excerpt(descText, 100),
				}
				if _, err := tx.Notification.Create().
					SetUserID(id).
					SetType("mentioned").
					SetPayload(payload).
					Save(ctx); err != nil {
					return err
				}
				pendingPush = append(pendingPush, pushdelivery.BuildItem(id, h.frontendURL, t.WorkspaceID.String(), "mentioned", payload))
			}
		}

		if statusChanging {
			if err := activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.status_changed",
				map[string]any{"from": string(previousStatus), "to": string(updated.Status)}); err != nil {
				return err
			}
		}
		if len(changes) > 0 {
			if err := activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.updated", changes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errInvalidIDs) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": "failed to update task"})
		return
	}

	// Googleカレンダー自動同期（M6）。イベント内容（タイトル・期間）に影響する
	// フィールドの変更時のみ同期する（無条件に同期すると外部API呼び出しが過剰になる
	// ため。docs/aibo/m6-implementation-plan.md 設計判断4）。バックグラウンド化の
	// 理由はCreateと同じ（体感速度対策、2026-08-21）。
	if req.Title != nil || startDate.provided || dueDate.provided {
		calendarsync.Async(func(ctx context.Context) {
			calendarsync.SyncTask(ctx, h.client, h.calCfg, h.encKey, updated, h.frontendURL)
		})
	}

	// Web Push配信（M7）。DBコミット後にベストエフォートで送る
	// （docs/aibo/m7-implementation-plan.md 設計判断7、calendarsync.Asyncと同じ理由）。
	pushdelivery.Async(h.client, h.pushCfg, pendingPush)

	c.JSON(http.StatusOK, taskJSON(updated))
}

// Delete は DELETE /tasks/:task_id。子タスクも道連れに削除する。
// entのschemaにカスケード削除指定が無いため、関連テーブルを手動で削除する。
// comments/activity_logsもtask_idを参照しているため、他のedgeテーブルと同様に
// 先に削除しておかないとタスク本体の削除時に外部キー制約違反になる。
func (h *TaskHandler) Delete(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var attachmentKeys []string
	var calendarEventRefs []calendarsync.EventRef
	var githubCommentRefs []githubCommentRef
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		childIDs, err := tx.Task.Query().Where(task.ParentTaskIDEQ(t.ID)).IDs(ctx)
		if err != nil {
			return err
		}
		ids := append(childIDs, t.ID)

		// GitHub Issueへ投稿済みのコメント（M8.5後追加）も、DBコミット確定後に
		// ベストエフォートで削除する（R2/Googleカレンダーと同じ「外部API呼び出しを
		// トランザクション内で行わない」方針）。参照だけここで収集しておく。
		withComments, err := tx.Task.Query().
			Where(task.IDIn(ids...), task.GithubIssueURLNotNil(), task.GithubIssueCommentIDNotNil()).
			All(ctx)
		if err != nil {
			return err
		}
		for _, wc := range withComments {
			if ref, err := githubissue.ParseIssueURL(*wc.GithubIssueURL); err == nil {
				githubCommentRefs = append(githubCommentRefs, githubCommentRef{ref: ref, commentID: *wc.GithubIssueCommentID})
			}
		}

		// Googleカレンダー連携イベント（M6）もtask_idを参照しているため、他のedge
		// テーブルと同様に先に削除する必要がある。Google側の実イベント削除は
		// DBコミット確定後にベストエフォートで行う（R2オブジェクトと同じ方針）ため、
		// ここでは削除対象の参照だけ収集しておく。
		calendarEvents, err := tx.TaskCalendarEvent.Query().Where(taskcalendarevent.TaskIDIn(ids...)).WithUser().All(ctx)
		if err != nil {
			return err
		}
		for _, e := range calendarEvents {
			calendarEventRefs = append(calendarEventRefs, calendarsync.EventRef{User: e.Edges.User, GoogleEventID: e.GoogleEventID})
		}
		if _, err := tx.TaskCalendarEvent.Delete().Where(taskcalendarevent.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.TaskDependency.Delete().
			Where(taskdependency.Or(
				taskdependency.TaskIDIn(ids...),
				taskdependency.DependsOnTaskIDIn(ids...),
			)).
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.TaskTag.Delete().Where(tasktag.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.TaskAssignee.Delete().Where(taskassignee.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.TaskMention.Delete().Where(taskmention.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.TaskPin.Delete().Where(taskpin.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		memoKeys, err := deleteTaskMemosForTasks(ctx, tx, ids)
		if err != nil {
			return err
		}
		attachmentKeys = append(attachmentKeys, memoKeys...)
		if _, err := tx.CommentMention.Delete().
			Where(commentmention.HasCommentWith(comment.TaskIDIn(ids...))).
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.Comment.Delete().Where(comment.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		// タスク説明欄自体へのリアクション（target_id=タスクID）と、配下コメントへの
		// リアクション（denormalizeしたtask_id列で一括指定）の両方を削除する。
		if _, err := tx.Reaction.Delete().
			Where(reaction.Or(
				reaction.And(reaction.TargetTypeEQ(reaction.TargetTypeTask), reaction.TargetIDIn(ids...)),
				reaction.And(reaction.TargetTypeEQ(reaction.TargetTypeComment), reaction.TaskIDIn(ids...)),
			)).
			Exec(ctx); err != nil {
			return err
		}
		// R2オブジェクトはDBコミットが確定してから削除する（トランザクション内で
		// 外部API呼び出しをしない）。ここではキーだけ収集しておく。
		attachments, err := tx.Attachment.Query().Where(attachment.TaskIDIn(ids...)).All(ctx)
		if err != nil {
			return err
		}
		for _, a := range attachments {
			attachmentKeys = append(attachmentKeys, a.StorageKey)
		}
		if _, err := tx.Attachment.Delete().Where(attachment.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.ActivityLog.Delete().Where(activitylog.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		// task.deleted自体は削除対象のtask_idを参照できない（削除後に外部キー違反になる）ため、
		// task_idを付けずproject_idのみで記録する。
		if err := activity.Record(ctx, tx, t.WorkspaceID, nil, t.ProjectID, u.ID, "task.deleted",
			map[string]any{"title": t.Title, "task_id": t.ID, "child_ids": childIDs}); err != nil {
			return err
		}
		_, err = tx.Task.Delete().Where(task.IDIn(ids...)).Exec(ctx)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete task"})
		return
	}
	deleteR2Objects(ctx, h.r2, attachmentKeys)
	// Googleカレンダー側のイベント削除（M6、モードに関わらず。spec.md 5章）。
	// バックグラウンド化の理由はCreate/Updateと同じ（体感速度対策、2026-08-21）。
	calendarsync.Async(func(ctx context.Context) {
		calendarsync.DeleteGoogleEventsOnly(ctx, h.calCfg, h.encKey, calendarEventRefs)
	})
	h.deleteGitHubComments(t.WorkspaceID, githubCommentRefs)
	c.Status(http.StatusNoContent)
}

// githubCommentRef はタスク削除に伴い後始末が必要なGitHub Issueコメントの参照
// （M8.5後追加）。project.go/workspace.goの一括削除では、対象タスク数によっては
// GitHub APIコールが多数発生しうるためこのクリーンアップは行わない（既知のスコープ外、
// 単体タスク削除に限定。孤立したコメントが残る程度の影響で、aisu側のデータ整合性には
// 影響しない）。
type githubCommentRef struct {
	ref       githubissue.IssueRef
	commentID int64
}

func (h *TaskHandler) deleteGitHubComments(workspaceID uuid.UUID, refs []githubCommentRef) {
	if len(refs) == 0 {
		return
	}
	calendarsync.Async(func(ctx context.Context) {
		cfg, err := h.resolveGitHubConfig(ctx, workspaceID)
		if err != nil || !cfg.Configured() {
			return
		}
		for _, r := range refs {
			if err := githubissue.DeleteComment(ctx, cfg, r.ref, r.commentID); err != nil {
				log.Printf("task: failed to delete github issue comment %d: %v", r.commentID, err)
			}
		}
	})
}

// --- helpers ---

// dateTimeInputはstart_date/due_dateのPATCH入力を3状態で表す
// （未指定=変更なし／空文字=クリア／値あり=設定。mentioned_user_idsと同じ「ポインタで
// 区別する」考え方をtime.Timeに適用したもの）。
type dateTimeInput struct {
	provided bool
	clear    bool
	value    time.Time
}

func parseDateTimeInput(s *string) (dateTimeInput, error) {
	if s == nil {
		return dateTimeInput{}, nil
	}
	if *s == "" {
		return dateTimeInput{provided: true, clear: true}, nil
	}
	t, err := time.ParseInLocation(dateTimeLayout, *s, jst)
	if err != nil {
		return dateTimeInput{}, err
	}
	return dateTimeInput{provided: true, value: t}, nil
}

// validateDateRange は開始日時・期限が同じリクエストで両方指定されている場合のみ、
// 開始が期限より後になっていないかを検証する（日時範囲としての整合性チェック）。
func validateDateRange(start, due dateTimeInput) error {
	if start.provided && !start.clear && due.provided && !due.clear && start.value.After(due.value) {
		return errors.New("start_date must not be after due_date")
	}
	return nil
}

// resolveStatusColumn は指定されたstatus_column_id/statusから
// (status_column_id, status)を決定する。両方nilなら、そのプロジェクトの
// not_started列（position最小）をデフォルトにする。
func resolveStatusColumn(ctx context.Context, client *ent.Client, projectID uuid.UUID, statusColumnID *uuid.UUID, status *string) (*uuid.UUID, task.Status, error) {
	if statusColumnID != nil {
		col, err := client.ProjectStatusColumn.Get(ctx, *statusColumnID)
		if err != nil || col.ProjectID != projectID {
			return nil, "", errInvalidIDs
		}
		id := col.ID
		return &id, task.Status(col.MapsToStatus), nil
	}

	var mapsTo projectstatuscolumn.MapsToStatus
	if status != nil {
		mapsTo = projectstatuscolumn.MapsToStatus(*status)
	} else {
		mapsTo = projectstatuscolumn.MapsToStatusNotStarted
	}

	col, err := client.ProjectStatusColumn.Query().
		Where(
			projectstatuscolumn.ProjectIDEQ(projectID),
			projectstatuscolumn.MapsToStatusEQ(mapsTo),
		).
		Order(projectstatuscolumn.ByPosition()).
		First(ctx)
	if err != nil {
		return nil, task.Status(mapsTo), nil
	}
	id := col.ID
	return &id, task.Status(mapsTo), nil
}
