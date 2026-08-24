package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ReminderSend holds the schema definition for the ReminderSend entity.
// M7（通知・検索）で追加。「今日期限」「期限超過」リマインダーの1ユーザー1日1回の
// 送信済み記録（task_calendar_eventsと同種の冪等性テーブル）。
type ReminderSend struct {
	ent.Schema
}

func (ReminderSend) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the ReminderSend.
func (ReminderSend) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.UUID{}),
		field.Enum("type").
			Values("due_today", "overdue"),
		field.Time("sent_date").
			SchemaType(map[string]string{"postgres": "date"}),
	}
}

// Edges of the ReminderSend.
func (ReminderSend) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
	}
}

// Indexes of the ReminderSend.
func (ReminderSend) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "type", "sent_date").Unique(),
	}
}
