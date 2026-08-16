package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskAssignee holds the schema definition for the TaskAssignee entity.
// db-schema.md models this table with a composite primary key
// (task_id, user_id); ent has no native composite-PK support, so we keep
// the generated UUID id as PK and enforce the pair's uniqueness via index.
type TaskAssignee struct {
	ent.Schema
}

func (TaskAssignee) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskAssignee.
func (TaskAssignee) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
	}
}

// Edges of the TaskAssignee.
func (TaskAssignee) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).
			Field("task_id").
			Unique().
			Required(),
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
	}
}

// Indexes of the TaskAssignee.
func (TaskAssignee) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "user_id").Unique(),
	}
}
