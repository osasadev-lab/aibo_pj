package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskDependency holds the schema definition for the TaskDependency entity.
// "task"は後続タスク（successor）、"depends_on"は先に完了させる必要がある
// 先行タスク（predecessor）。自己参照の禁止・循環依存チェックは
// db-schema.md記載の通りアプリケーション層で行う。
type TaskDependency struct {
	ent.Schema
}

func (TaskDependency) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskDependency.
func (TaskDependency) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("depends_on_task_id", uuid.UUID{}),
	}
}

// Edges of the TaskDependency.
func (TaskDependency) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).
			Field("task_id").
			Unique().
			Required(),
		edge.To("depends_on", Task.Type).
			Field("depends_on_task_id").
			Unique().
			Required(),
	}
}

// Indexes of the TaskDependency.
func (TaskDependency) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "depends_on_task_id").Unique(),
	}
}
