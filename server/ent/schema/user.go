package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{BaseMixin{}}
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("google_sub").
			Unique().
			NotEmpty(),
		field.String("email").
			Unique().
			NotEmpty(),
		field.String("name").
			NotEmpty(),
		field.String("avatar_url").
			Optional().
			Nillable(),
		field.String("google_refresh_token").
			Optional().
			Nillable().
			Sensitive(),
		field.Bool("calendar_sync_enabled").
			Default(false),
		field.Enum("calendar_sync_mode").
			Values("auto", "manual").
			Optional().
			Nillable(),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("workspace_members", WorkspaceMember.Type).Ref("user"),
		edge.From("project_members", ProjectMember.Type).Ref("user"),
		edge.From("created_projects", Project.Type).Ref("creator"),
		edge.From("created_tasks", Task.Type).Ref("creator"),
		edge.From("task_assignees", TaskAssignee.Type).Ref("user"),
		edge.From("calendar_events", TaskCalendarEvent.Type).Ref("user"),
		edge.From("comments", Comment.Type).Ref("user"),
		edge.From("comment_mentions", CommentMention.Type).Ref("mentioned_user"),
		edge.From("attachments", Attachment.Type).Ref("uploader"),
		edge.From("activity_logs", ActivityLog.Type).Ref("actor"),
		edge.From("notifications", Notification.Type).Ref("user"),
	}
}
