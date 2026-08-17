package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// ProjectStatusColumn holds the schema definition for the ProjectStatusColumn entity.
type ProjectStatusColumn struct {
	ent.Schema
}

func (ProjectStatusColumn) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the ProjectStatusColumn.
func (ProjectStatusColumn) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("project_id", uuid.UUID{}),
		field.String("name").NotEmpty(),
		field.Int("position"),
		field.Enum("maps_to_status").
			Values("not_started", "in_progress", "done", "on_hold"),
		// プロジェクト作成時に自動投入される既定4列かどうか。既定列はUIから削除できない。
		field.Bool("is_default").Default(false),
	}
}

// Edges of the ProjectStatusColumn.
func (ProjectStatusColumn) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("project", Project.Type).
			Field("project_id").
			Unique().
			Required(),
		edge.From("tasks", Task.Type).Ref("status_column"),
	}
}
