package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskDependency holds the schema definition for the TaskDependency entity.
// "task" is the successor, "depends_on" is the predecessor that must be
// completed first. Self-loop prevention and cycle detection are enforced
// in the application layer per db-schema.md.
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
