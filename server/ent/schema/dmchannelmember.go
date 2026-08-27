package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// DMChannelMember holds the schema definition for the DMChannelMember entity.
// M8.5（DM機能）で追加。チャンネルの参加者。
type DMChannelMember struct {
	ent.Schema
}

func (DMChannelMember) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the DMChannelMember.
func (DMChannelMember) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("channel_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		// 未読バッジ算出用。このチャンネルを開いた時刻を都度更新する。
		field.Time("last_read_at").Optional().Nillable(),
	}
}

// Edges of the DMChannelMember.
func (DMChannelMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("channel", DMChannel.Type).
			Field("channel_id").
			Unique().
			Required(),
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
	}
}

// Indexes of the DMChannelMember.
func (DMChannelMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_id", "user_id").Unique(),
	}
}
