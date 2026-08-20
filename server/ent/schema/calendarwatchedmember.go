package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CalendarWatchedMember holds the schema definition for the CalendarWatchedMember entity.
// カレンダー画面の個人設定（M5追加）。本人（user_id）が追加した他メンバー
// （watched_user_id）のタスクも、本人のカレンダーに重ねて表示するための設定。
// ワークスペースごとに独立して保持する。
type CalendarWatchedMember struct {
	ent.Schema
}

func (CalendarWatchedMember) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the CalendarWatchedMember.
func (CalendarWatchedMember) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("workspace_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),         // 閲覧者本人
		field.UUID("watched_user_id", uuid.UUID{}), // カレンダーに追加表示する対象メンバー
	}
}

// Edges of the CalendarWatchedMember.
func (CalendarWatchedMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("workspace", Workspace.Type).
			Field("workspace_id").
			Unique().
			Required(),
		edge.To("user", User.Type).
			Field("user_id").
			Unique().
			Required(),
		edge.To("watched_user", User.Type).
			Field("watched_user_id").
			Unique().
			Required(),
	}
}

// Indexes of the CalendarWatchedMember.
func (CalendarWatchedMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "user_id", "watched_user_id").Unique(),
	}
}
