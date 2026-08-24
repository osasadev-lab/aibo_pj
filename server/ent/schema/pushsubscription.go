package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// PushSubscription holds the schema definition for the PushSubscription entity.
// M7（通知・検索）で追加。ブラウザのPush Manager購読情報を保持し、Web Push配信に使う。
type PushSubscription struct {
	ent.Schema
}

func (PushSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the PushSubscription.
func (PushSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}),
		field.String("endpoint").
			NotEmpty().
			Unique(),
		field.String("p256dh").NotEmpty(),
		field.String("auth").NotEmpty(),
	}
}

// Edges of the PushSubscription.
func (PushSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
	}
}
