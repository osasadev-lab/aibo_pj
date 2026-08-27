package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Task holds the schema definition for the Task entity.
type Task struct {
	ent.Schema
}

func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the Task.
func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		field.UUID("project_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("section_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("status").
			Values("not_started", "in_progress", "done", "on_hold").
			Default("not_started"),
		field.UUID("status_column_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("parent_task_id", uuid.UUID{}).Optional().Nillable(),
		field.String("title").NotEmpty(),
		field.Text("description").Optional().Nillable(),
		field.Enum("priority").
			Values("low", "medium", "high").
			Optional().
			Nillable(),
		// 2026-08-27追加：以前はPostgresの`date`型に固定し時刻を保持しなかったが、
		// タスクの期限を日時範囲（開始日時〜期限日時）で管理する要望に伴い、
		// created_at/updated_at等と同じent既定のtimestamptzへ変更した。
		field.Time("start_date").
			Optional().
			Nillable(),
		field.Time("due_date").
			Optional().
			Nillable(),
		field.UUID("created_by", uuid.UUID{}),
		// M8追加。プロジェクトカンバンの同一列内でのD&D並び替え用（列＝status_column_id内で
		// 昇順に表示する）。nilは「未設定」を表し、既存挙動どおりcreated_at順で末尾に扱う
		// （マイグレーション時の一括バックフィルは行わない）。
		field.Int("position").Optional().Nillable(),
	}
}

// Edges of the Task.
func (Task) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("project", Project.Type).
			Field("project_id").
			Unique(),
		edge.To("section", Section.Type).
			Field("section_id").
			Unique(),
		edge.To("status_column", ProjectStatusColumn.Type).
			Field("status_column_id").
			Unique(),
		edge.To("creator", User.Type).
			Field("created_by").
			Unique().
			Required(),
		edge.To("children", Task.Type).
			From("parent").
			Field("parent_task_id").
			Unique(),
		edge.From("assignees", TaskAssignee.Type).Ref("task"),
		edge.From("calendar_events", TaskCalendarEvent.Type).Ref("task"),
		edge.From("dependencies", TaskDependency.Type).Ref("task"),
		edge.From("dependents", TaskDependency.Type).Ref("depends_on"),
		edge.From("tags", TaskTag.Type).Ref("task"),
		edge.From("comments", Comment.Type).Ref("task"),
		edge.From("attachments", Attachment.Type).Ref("task"),
		edge.From("mentions", TaskMention.Type).Ref("task"),
		edge.From("pins", TaskPin.Type).Ref("task"),
		edge.From("memos", TaskMemo.Type).Ref("task"),
	}
}

// Indexes of the Task.
func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "status_column_id"),
		index.Fields("project_id", "status"),
		index.Fields("parent_task_id"),
	}
}
