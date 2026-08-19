package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Project holds the schema definition for the Project entity.
type Project struct {
	ent.Schema
}

func (Project) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the Project.
func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		field.String("name").NotEmpty(),
		field.String("description").Optional().Nillable(),
		field.Enum("visibility").
			Values("public", "private"),
		field.UUID("created_by", uuid.UUID{}),
	}
}

// Edges of the Project.
func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("creator", User.Type).
			Field("created_by").
			Unique().
			Required(),
		edge.From("members", ProjectMember.Type).Ref("project"),
		edge.From("status_columns", ProjectStatusColumn.Type).Ref("project"),
		edge.From("sections", Section.Type).Ref("project"),
		edge.From("tasks", Task.Type).Ref("project"),
		edge.From("tags", Tag.Type).Ref("project"),
	}
}
