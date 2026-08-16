package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskTag holds the schema definition for the TaskTag entity.
type TaskTag struct {
	ent.Schema
}

func (TaskTag) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskTag.
func (TaskTag) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("tag_id", uuid.UUID{}),
	}
}

// Edges of the TaskTag.
func (TaskTag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).
			Field("task_id").
			Unique().
			Required(),
		edge.To("tag", Tag.Type).
			Field("tag_id").
			Unique().
			Required(),
	}
}

// Indexes of the TaskTag.
func (TaskTag) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "tag_id").Unique(),
	}
}
