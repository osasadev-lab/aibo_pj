package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// DMMessage holds the schema definition for the DMMessage entity.
// M8.5（DM機能）で追加。書き込みはGo API経由のみ、即時反映はSupabase Realtimeの
// 直接購読で行う（commentsと同じ方式、docs/aibo/m8.5-implementation-plan.md参照）。
type DMMessage struct {
	ent.Schema
}

func (DMMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the DMMessage.
func (DMMessage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("channel_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		// commentsと異なりOptional：添付ファイルのみのメッセージを許容するため。
		field.Text("body").Optional().Nillable(),
	}
}

// Edges of the DMMessage.
func (DMMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("channel", DMChannel.Type).
			Field("channel_id").
			Unique().
			Required(),
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
		edge.From("attachments", DMAttachment.Type).Ref("message"),
	}
}
