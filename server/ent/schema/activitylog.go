package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ActivityLog holds the schema definition for the ActivityLog entity.
// フェーズ2のAI進捗サマリー機能（spec.md 9章）のデータソースとなる。
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
		// M5のハイライト機能（GET /workspaces/:id/activity）のactor絞り込み用。
		index.Fields("workspace_id", "actor_id"),
	}
}
