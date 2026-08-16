package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Workspace holds the schema definition for the Workspace entity.
type Workspace struct {
	ent.Schema
}

func (Workspace) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the Workspace.
func (Workspace) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
	}
}

// Edges of the Workspace.
func (Workspace) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("members", WorkspaceMember.Type).Ref("workspace"),
		edge.From("projects", Project.Type).Ref("workspace"),
		edge.From("tasks", Task.Type).Ref("workspace"),
		edge.From("tags", Tag.Type).Ref("workspace"),
		edge.From("activity_logs", ActivityLog.Type).Ref("workspace"),
	}
}
