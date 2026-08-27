package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Reaction holds the schema definition for the Reaction entity.
// タスク説明・コメント・DMメッセージへの絵文字リアクション（2026-08-28追加）。
// target_type/target_idで対象を多態的に指す。task_id/dm_channel_idは対象を含む
// タスク／DMチャンネル単位でSupabase Realtimeを一括購読するための非正規化列
// （comments/dm_messagesと同じ理由。コメント一覧・DMメッセージ一覧のたびに
// 多数のリアクションチャンネルを個別購読しなくて済むようにするため）。
type Reaction struct {
	ent.Schema
}

func (Reaction) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the Reaction.
func (Reaction) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("target_type").Values("task", "comment", "dm_message"),
		field.UUID("target_id", uuid.UUID{}),
		field.UUID("task_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("dm_channel_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("user_id", uuid.UUID{}),
		field.String("emoji"),
	}
}

// Edges of the Reaction.
func (Reaction) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
	}
}

// Indexes of the Reaction.
func (Reaction) Indexes() []ent.Index {
	return []ent.Index{
		// 1人1対象につきリアクションは1つだけ（ユーザー要望、2026-08-28）。
		// 既に付けている絵文字を再度選ぶと解除、別の絵文字を選ぶと変更（emoji列を
		// 更新）になる。そのためunique制約からemojiを外し、target_type/target_id/
		// user_idの組で一意にする。
		index.Fields("target_type", "target_id", "user_id").Unique(),
		index.Fields("target_type", "target_id"),
		index.Fields("task_id"),
		index.Fields("dm_channel_id"),
	}
}
