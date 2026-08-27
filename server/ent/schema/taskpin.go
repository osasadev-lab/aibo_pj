package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskPin holds the schema definition for the TaskPin entity.
// M8（PWA・UI仕上げ）で追加。タスクのピン留め（ユーザー×タスクの中間テーブル、
// task_tags/task_assigneesと同型）。ピン日時はBaseMixinのcreated_atを流用する。
type TaskPin struct {
	ent.Schema
}

func (TaskPin) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskPin.
func (TaskPin) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("task_id", uuid.UUID{}),
	}
}

// Edges of the TaskPin.
func (TaskPin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
		edge.To("task", Task.Type).
			Field("task_id").
			Unique().
			Required(),
	}
}

// Indexes of the TaskPin.
func (TaskPin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "task_id").Unique(),
	}
}
