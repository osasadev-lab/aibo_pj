package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ActivityLog holds the schema definition for the ActivityLog entity.
// Serves as the data source for the phase-2 AI progress-summary features
// (spec.md 9章).
type ActivityLog struct {
	ent.Schema
}

func (ActivityLog) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the ActivityLog.
func (ActivityLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		field.UUID("task_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("project_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("actor_id", uuid.UUID{}),
		field.String("action_type").NotEmpty(),
		field.JSON("payload", map[string]any{}).Optional(),
	}
}

// Edges of the ActivityLog.
func (ActivityLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("task", Task.Type).
			Field("task_id").
			Unique(),
		edge.To("project", Project.Type).
			Field("project_id").
			Unique(),
		edge.To("actor", User.Type).
			Field("actor_id").
			Unique().
			Required(),
	}
}

// Indexes of the ActivityLog.
func (ActivityLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "created_at"),
		index.Fields("task_id"),
	}
}
