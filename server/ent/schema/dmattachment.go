package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// DMAttachment holds the schema definition for the DMAttachment entity.
// M8.5（DM機能）で追加。attachmentsと同型（task_idの代わりにdm_message_id）。
type DMAttachment struct {
	ent.Schema
}

func (DMAttachment) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the DMAttachment.
func (DMAttachment) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("dm_message_id", uuid.UUID{}),
		field.UUID("uploaded_by", uuid.UUID{}),
		field.String("file_name").NotEmpty(),
		field.String("storage_key").NotEmpty(),
		field.Int64("size_bytes"),
		field.String("content_type").NotEmpty(),
	}
}

// Edges of the DMAttachment.
func (DMAttachment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("message", DMMessage.Type).
			Field("dm_message_id").
			Unique().
			Required(),
		edge.To("uploader", User.Type).
			Field("uploaded_by").
			Unique().
			Required(),
	}
}
