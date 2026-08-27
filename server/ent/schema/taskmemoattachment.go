package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// TaskMemoAttachment holds the schema definition for the TaskMemoAttachment entity.
// M8.5（メモ機能）で追加。attachmentsと同型（task_idの代わりにtask_memo_id）。
type TaskMemoAttachment struct {
	ent.Schema
}

func (TaskMemoAttachment) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskMemoAttachment.
func (TaskMemoAttachment) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_memo_id", uuid.UUID{}),
		field.UUID("uploaded_by", uuid.UUID{}),
		field.String("file_name").NotEmpty(),
		field.String("storage_key").NotEmpty(),
		field.Int64("size_bytes"),
		field.String("content_type").NotEmpty(),
	}
}

// Edges of the TaskMemoAttachment.
func (TaskMemoAttachment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("memo", TaskMemo.Type).
			Field("task_memo_id").
			Unique().
			Required(),
		edge.To("uploader", User.Type).
			Field("uploaded_by").
			Unique().
			Required(),
	}
}
