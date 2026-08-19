package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Tag holds the schema definition for the Tag entity.
type Tag struct {
	ent.Schema
}

func (Tag) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the Tag.
func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		// nilはワークスペース共通タグ（project非依存、単体タスクや全プロジェクトのタスクから使える）。
		// 非nilはそのプロジェクト専用タグ（そのプロジェクトのタスクからのみ使える）。
		field.UUID("project_id", uuid.UUID{}).Optional().Nillable(),
		field.String("name").NotEmpty(),
		field.String("color").NotEmpty(),
	}
}

// Edges of the Tag.
func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("project", Project.Type).
			Field("project_id").
			Unique(),
		edge.From("tasks", TaskTag.Type).Ref("tag"),
	}
}

// Indexes of the Tag.
func (Tag) Indexes() []ent.Index {
	return []ent.Index{
		// PostgresのUNIQUE INDEXはNULLを区別しないため、project_idがnull同士（共通タグ）の
		// 重複名はこの制約では検出できない。共通タグの重複チェックはアプリ層で行う。
		index.Fields("workspace_id", "project_id", "name").Unique(),
	}
}
