package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// DMChannel holds the schema definition for the DMChannel entity.
// M8.5（DM機能）で追加。ワークスペース内メンバーによる1:1またはグループチャットの
// チャンネル（docs/aibo/m8.5-implementation-plan.md参照）。
type DMChannel struct {
	ent.Schema
}

func (DMChannel) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the DMChannel.
func (DMChannel) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		field.Bool("is_group").Default(false),
		field.String("name").Optional().Nillable(),
		// 1:1チャンネル（is_group=false）のみ設定する正規化キー（ソートした2人分の
		// user_idを連結した文字列）。同じ2人の組み合わせで重複作成しないための
		// find-or-createキー。グループは常にnull（PostgresのUNIQUE制約はNULL同士を
		// 区別しないため、複数グループでnullが重複しても衝突しない＝同じメンバー
		// 構成でも複数のグループを作成できる）。
		field.String("dm_key").Optional().Nillable().Unique(),
		field.UUID("created_by", uuid.UUID{}),
	}
}

// Edges of the DMChannel.
func (DMChannel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("creator", User.Type).
			Field("created_by").
			Unique().
			Required(),
		edge.From("members", DMChannelMember.Type).Ref("channel"),
		edge.From("messages", DMMessage.Type).Ref("channel"),
	}
}
