package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// WorkspaceMember holds the schema definition for the WorkspaceMember entity.
type WorkspaceMember struct {
	ent.Schema
}

func (WorkspaceMember) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the WorkspaceMember.
func (WorkspaceMember) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		field.Enum("role").
			Values("owner", "member"),
		// M8追加。左サイドバーのプロジェクト一覧の個人ごとの並び順（本人のみが変更可能、
		// 他メンバーの表示には影響しない）。配列に含まれないプロジェクトはcreated_at順で
		// 末尾に表示する（フロント側で補完）。
		field.JSON("project_order", []uuid.UUID{}).Optional(),
	}
}

// Edges of the WorkspaceMember.
func (WorkspaceMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
	}
}

// Indexes of the WorkspaceMember.
func (WorkspaceMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "user_id").Unique(),
	}
}
