package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Feedback holds the schema definition for the Feedback entity.
// M8（PWA・UI仕上げ）で追加。左サイドバー「フィードバック」からの投稿を記録する
// （一次記録。通知は別途feedbackmailパッケージがコミット後にベストエフォートでメール送信する）。
type Feedback struct {
	ent.Schema
}

func (Feedback) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the Feedback.
func (Feedback) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("workspace_id", uuid.UUID{}).Optional().Nillable(),
		field.Text("body").NotEmpty(),
		field.String("page_path").Optional().Nillable(),
	}
}

// Edges of the Feedback.
func (Feedback) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
	}
}
