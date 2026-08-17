package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// WorkspaceInvitation holds the schema definition for the WorkspaceInvitation entity.
// 行の存在が「招待中(pending)」を表す。招待されたメールアドレスのユーザーが
// 初めてGoogleログインした時点でworkspace_memberへ変換し、この行は削除する。
type WorkspaceInvitation struct {
	ent.Schema
}

func (WorkspaceInvitation) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the WorkspaceInvitation.
func (WorkspaceInvitation) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		field.String("email").
			NotEmpty(),
		field.UUID("invited_by", uuid.UUID{}),
	}
}

// Edges of the WorkspaceInvitation.
func (WorkspaceInvitation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("inviter", User.Type).
			Field("invited_by").
			Unique().
			Required(),
	}
}

// Indexes of the WorkspaceInvitation.
func (WorkspaceInvitation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "email").Unique(),
	}
}
