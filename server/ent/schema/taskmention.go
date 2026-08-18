package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TaskMention holds the schema definition for the TaskMention entity.
// CommentMentionと異なり、タスク説明文は編集で内容が変わるため追記型ログではなく
// TaskAssigneeと同じ「現在の状態を置き換える」テーブルとして扱う。
type TaskMention struct {
	ent.Schema
}

func (TaskMention) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the TaskMention.
func (TaskMention) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("mentioned_user_id", uuid.UUID{}),
	}
}

// Edges of the TaskMention.
func (TaskMention) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("task", Task.Type).
			Field("task_id").
			Unique().
			Required(),
		edge.To("mentioned_user", User.Type).
			Field("mentioned_user_id").
			Unique().
			Required(),
	}
}

// Indexes of the TaskMention.
func (TaskMention) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "mentioned_user_id").Unique(),
	}
}
