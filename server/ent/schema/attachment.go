package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Attachment holds the schema definition for the Attachment entity.
type Attachment struct {
	ent.Schema
}

func (Attachment) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the Attachment.
func (Attachment) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("uploaded_by", uuid.UUID{}),
		field.String("file_name").NotEmpty(),
		field.String("storage_key").NotEmpty(),
		field.Int64("size_bytes"),
		field.String("content_type").NotEmpty(),
	}
}

// Edges of the Attachment.
func (Attachment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).
			Field("task_id").
			Unique().
			Required(),
		edge.To("uploader", User.Type).
			Field("uploaded_by").
			Unique().
			Required(),
	}
}
