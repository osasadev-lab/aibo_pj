package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CommentMention holds the schema definition for the CommentMention entity.
type CommentMention struct {
	ent.Schema
}

func (CommentMention) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the CommentMention.
func (CommentMention) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("comment_id", uuid.UUID{}),
		field.UUID("mentioned_user_id", uuid.UUID{}),
	}
}

// Edges of the CommentMention.
func (CommentMention) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("comment", Comment.Type).
			Field("comment_id").
			Unique().
			Required(),
		edge.To("mentioned_user", User.Type).
			Field("mentioned_user_id").
			Unique().
			Required(),
	}
}

// Indexes of the CommentMention.
func (CommentMention) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("comment_id", "mentioned_user_id").Unique(),
	}
}
