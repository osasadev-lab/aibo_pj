package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskCalendarEvent holds the schema definition for the TaskCalendarEvent entity.
type TaskCalendarEvent struct {
	ent.Schema
}

func (TaskCalendarEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskCalendarEvent.
func (TaskCalendarEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		field.String("google_event_id").NotEmpty(),
		field.Time("synced_at").Default(time.Now),
	}
}

// Edges of the TaskCalendarEvent.
func (TaskCalendarEvent) Edges() []ent.Edge {
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

// Indexes of the TaskCalendarEvent.
func (TaskCalendarEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "user_id").Unique(),
	}
}
