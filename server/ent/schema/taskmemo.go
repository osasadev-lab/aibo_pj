package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskMemo holds the schema definition for the TaskMemo entity.
// M8.5（メモ機能）で追加。ユーザー本人にだけ表示されるタスクごとの個人メモ
// （TaskPinと同じユーザー×タスクの中間テーブル方式、docs/aibo/m8.5-implementation-plan.md参照）。
type TaskMemo struct {
	ent.Schema
}

func (TaskMemo) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskMemo.
func (TaskMemo) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		field.Text("body").Optional().Nillable(),
	}
}

// Edges of the TaskMemo.
func (TaskMemo) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).
			Field("task_id").
			Unique().
			Required(),
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
		edge.From("attachments", TaskMemoAttachment.Type).Ref("memo"),
	}
}

// Indexes of the TaskMemo.
func (TaskMemo) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "user_id").Unique(),
	}
}
