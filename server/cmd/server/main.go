package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	internalauth "github.com/osasadev-lab/aibo_pj/server/internal/auth"
	"github.com/osasadev-lab/aibo_pj/server/internal/config"
	"github.com/osasadev-lab/aibo_pj/server/internal/db"
	"github.com/osasadev-lab/aibo_pj/server/internal/feedbackmail"
	"github.com/osasadev-lab/aibo_pj/server/internal/handler"
	"github.com/osasadev-lab/aibo_pj/server/internal/logging"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"
)

func main() {
	// ローカル開発用。.envが無い場合（Cloud Run等）はエラーを無視して
	// プラットフォームが注入した環境変数をそのまま使う。
	_ = godotenv.Load()

	cfg := config.Load()

	client, err := db.NewEntClient(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer client.Close()

	// gin.Default()（テキストログのみ、user_idを含まない）の代わりに、
	// 「誰が・いつ・どこに・何を」を1行のJSONで残すRequestLoggerを使う
	// （2026-08-27追加。docs/aibo/project-audit.md「エラートラッキング・
	// 構造化ログの不在」対応）。
	router := gin.New()
	router.Use(logging.RequestLogger(), gin.Recovery())

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{cfg.FrontendURL},
		AllowMethods: []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))

	// 注意: "/healthz" はCloud Run側（Googleフロントエンド層）の予約パスと
	// 衝突し、コンテナに届く前に404を返すことを実機検証で確認したため、
	// 別のパスを使う。
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ローカルのhttp開発ではSecure Cookie（state用）を付けない。
	cookieSecure := strings.HasPrefix(cfg.GoogleOAuthRedirectURL, "https://")

	r2Client := storage.NewR2Client(cfg.R2AccountID, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2BucketName)

	// M6（Googleカレンダー連携）。encKeyはusers.google_refresh_tokenの暗号化に、
	// calendarOAuthConfigはcalendar.eventsスコープの同意フロー・Calendar API呼び出しに使う。
	encKey, err := internalauth.DecodeEncryptionKey(cfg.TokenEncryptionKey)
	if err != nil {
		log.Fatalf("invalid TOKEN_ENCRYPTION_KEY: %v", err)
	}
	calendarOAuthConfig := internalauth.NewGoogleCalendarOAuthConfig(cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret, cfg.GoogleCalendarRedirectURL)

	// M7（通知・検索）。pushCfgはWeb Push配信（VAPID）用。
	pushCfg := pushdelivery.Config{
		VAPIDPublicKey:  cfg.VAPIDPublicKey,
		VAPIDPrivateKey: cfg.VAPIDPrivateKey,
		VAPIDSubject:    cfg.VAPIDSubject,
	}

	// M8（フィードバック機能）。SMTPHostが空なら未設定として送信をスキップする。
	mailCfg := feedbackmail.Config{
		Host:        cfg.SMTPHost,
		Port:        cfg.SMTPPort,
		Username:    cfg.SMTPUsername,
		Password:    cfg.SMTPPassword,
		NotifyEmail: cfg.FeedbackNotifyEmail,
	}

	authHandler := handler.NewAuthHandler(client, cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret, cfg.GoogleOAuthRedirectURL, cfg.JWTSecret, cfg.SupabaseJWTSecret, cfg.FrontendURL, cookieSecure, r2Client, calendarOAuthConfig, encKey)
	calendarConnectHandler := handler.NewCalendarConnectHandler(client, calendarOAuthConfig, cfg.JWTSecret, encKey, cfg.FrontendURL)
	workspaceHandler := handler.NewWorkspaceHandler(client, r2Client, calendarOAuthConfig, encKey)
	memberHandler := handler.NewMemberHandler(client)
	projectHandler := handler.NewProjectHandler(client, r2Client, calendarOAuthConfig, encKey, pushCfg, cfg.FrontendURL)
	taskHandler := handler.NewTaskHandler(client, r2Client, calendarOAuthConfig, encKey, cfg.FrontendURL, pushCfg)
	commentHandler := handler.NewCommentHandler(client, pushCfg, cfg.FrontendURL)
	reactionHandler := handler.NewReactionHandler(client)
	notificationHandler := handler.NewNotificationHandler(client, pushCfg)
	tagHandler := handler.NewTagHandler(client)
	attachmentHandler := handler.NewAttachmentHandler(client, r2Client)
	calendarHandler := handler.NewCalendarHandler(client)
	progressHandler := handler.NewProgressHandler(client)
	activityHandler := handler.NewActivityHandler(client)
	cronHandler := handler.NewCronHandler(client, pushCfg, cfg.FrontendURL)
	feedbackHandler := handler.NewFeedbackHandler(client, mailCfg)
	dmHandler := handler.NewDMHandler(client, r2Client, pushCfg, cfg.FrontendURL)
	taskMemoHandler := handler.NewTaskMemoHandler(client, r2Client)

	requireAuth := middleware.RequireAuth(client, cfg.JWTSecret)
	requireWorkspaceMember := middleware.RequireWorkspaceMember(client)
	requireOwner := middleware.RequireOwner()
	requireProjectAccess := middleware.RequireProjectAccess(client)
	requireProjectManager := middleware.RequireProjectManager(client)
	requireTaskAccess := middleware.RequireTaskAccess(client)
	requireInternalCronSecret := middleware.RequireInternalCronSecret(cfg.InternalCronSecret)
	requireDMChannelAccess := middleware.RequireDMChannelAccess(client)

	api := router.Group("/api/v1")
	{
		// 簡易疎通確認用。Cloud Runのヘルスチェックではなく、アプリケーションの疎通確認用。
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
		})

		// 認証系エンドポイント
		authGroup := api.Group("/auth")
		{
			authGroup.GET("/google/login", authHandler.GoogleLogin)
			authGroup.GET("/google/callback", authHandler.GoogleCallback)
			authGroup.POST("/logout", requireAuth, authHandler.Logout)
			authGroup.GET("/me", requireAuth, authHandler.Me)

			// Googleカレンダー連携の同意フロー（M6）。tokenをクエリパラメータで受け取る
			// 専用経路のためrequireAuthミドルウェアは使わない（docs/aibo/m6-implementation-plan.md参照）。
			authGroup.GET("/google/calendar/connect", calendarConnectHandler.Connect)
			authGroup.GET("/google/calendar/callback", calendarConnectHandler.Callback)
		}

		// ワークスペース系エンドポイント
		workspaces := api.Group("/workspaces", requireAuth)
		{
			workspaces.GET("", workspaceHandler.List)
			workspaces.POST("", workspaceHandler.Create)

			withMember := workspaces.Group("/:workspace_id", requireWorkspaceMember)
			{
				withMember.GET("", workspaceHandler.Get)
				withMember.PATCH("", requireOwner, workspaceHandler.Update)
				withMember.DELETE("", requireOwner, workspaceHandler.Delete)
				// M8追加：左サイドバーのプロジェクト一覧の個人ごとの並び順。誰でも自分の
				// 表示順は変えられるためrequireOwner等の追加権限チェックは不要。
				withMember.PATCH("/project-order", workspaceHandler.UpdateProjectOrder)

				withMember.GET("/members", memberHandler.List)
				withMember.POST("/members/invite", requireOwner, memberHandler.Invite)
				withMember.PATCH("/members/:member_id", requireOwner, memberHandler.ChangeRole)
				withMember.DELETE("/members/:member_id", memberHandler.Remove)

				withMember.GET("/projects", projectHandler.List)
				withMember.POST("/projects", projectHandler.Create)
				withMember.GET("/tasks", taskHandler.Search)
				withMember.POST("/tasks", taskHandler.Create)
				withMember.GET("/my-tasks", taskHandler.MyTasks)
				withMember.GET("/search", taskHandler.FullTextSearch)
				withMember.GET("/pinned-tasks", taskHandler.ListPinned)

				withMember.GET("/calendar", calendarHandler.GetCalendar)
				withMember.GET("/calendar-watched-users", calendarHandler.GetWatchedMembers)
				withMember.PUT("/calendar-watched-users", calendarHandler.PutWatchedMembers)
				withMember.GET("/progress", progressHandler.GetProgress)
				withMember.GET("/activity", activityHandler.List)
				withMember.GET("/members/:member_id/tasks", memberHandler.MemberTasks)

				withMember.GET("/common-tags", tagHandler.ListCommonTags)
				withMember.POST("/common-tags", requireOwner, tagHandler.CreateCommonTag)
				withMember.PATCH("/common-tags/:tag_id", requireOwner, tagHandler.UpdateCommonTag)
				withMember.DELETE("/common-tags/:tag_id", requireOwner, tagHandler.DeleteCommonTag)

				// DM（M8.5、メンバー選択は既存の/membersを再利用）
				withMember.GET("/dm/channels", dmHandler.ListChannels)
				withMember.POST("/dm/channels", dmHandler.CreateChannel)

				// メモ（M8.5）
				withMember.GET("/task-memos", taskMemoHandler.ListMine)

				// GitHub Issue連携設定（M8.5後追加）。閲覧・設定ともワークスペースメンバー
				// なら誰でも可（ユーザー確認済み、2026-08-27。Owner限定にすると設定できる人が
				// 限られすぎるとのフィードバックにより変更）。
				withMember.GET("/github-settings", workspaceHandler.GetGitHubSettings)
				withMember.PATCH("/github-settings", workspaceHandler.UpdateGitHubSettings)
			}
		}

		// DMチャンネル系エンドポイント（:workspace_idを含まないパス、M8.5）
		dmChannels := api.Group("/dm/channels", requireAuth)
		{
			withDMChannel := dmChannels.Group("/:channel_id", requireDMChannelAccess)
			{
				withDMChannel.GET("", dmHandler.GetChannel)
				withDMChannel.PATCH("", dmHandler.RenameChannel)
				withDMChannel.POST("/members", dmHandler.AddMembers)
				withDMChannel.DELETE("/members/me", dmHandler.LeaveChannel)
				withDMChannel.GET("/messages", dmHandler.ListMessages)
				withDMChannel.POST("/messages", dmHandler.CreateMessage)
				withDMChannel.PATCH("/messages/:message_id", dmHandler.UpdateMessage)
				withDMChannel.DELETE("/messages/:message_id", dmHandler.DeleteMessage)
				withDMChannel.POST("/messages/:message_id/reactions", reactionHandler.ToggleDMReaction)
				withDMChannel.PATCH("/read", dmHandler.MarkRead)
			}
		}

		// DMメッセージの添付ファイル（:channel_idを含まないパス、M8.5）
		dmMessages := api.Group("/dm/messages", requireAuth)
		{
			dmMessages.POST("/:message_id/attachments", dmHandler.CreateAttachment)
		}
		dmAttachments := api.Group("/dm-attachments", requireAuth)
		{
			dmAttachments.DELETE("/:attachment_id", dmHandler.DeleteAttachment)
		}

		// メモ添付ファイル（:task_idを含まないパス、M8.5）
		memoAttachments := api.Group("/memo-attachments", requireAuth)
		{
			memoAttachments.DELETE("/:attachment_id", taskMemoHandler.DeleteAttachment)
		}

		// プロジェクト系エンドポイント（:workspace_idを含まないパス）
		projects := api.Group("/projects", requireAuth)
		{
			withProject := projects.Group("/:project_id", requireProjectAccess)
			{
				withProject.GET("", projectHandler.Get)
				withProject.PATCH("", requireProjectManager, projectHandler.Update)
				withProject.DELETE("", requireProjectManager, projectHandler.Delete)

				withProject.GET("/members", projectHandler.ListMembers)
				withProject.PUT("/members", requireProjectManager, projectHandler.PutMembers)
				withProject.PATCH("/members/:member_id", requireProjectManager, projectHandler.ChangeMemberRole)
				withProject.PUT("/managers", requireProjectManager, projectHandler.PutManagers)

				withProject.GET("/status-columns", projectHandler.ListStatusColumns)
				withProject.POST("/status-columns", requireProjectManager, projectHandler.CreateStatusColumn)
				withProject.PATCH("/status-columns/:column_id", requireProjectManager, projectHandler.UpdateStatusColumn)
				withProject.DELETE("/status-columns/:column_id", requireProjectManager, projectHandler.DeleteStatusColumn)

				withProject.GET("/tags", tagHandler.ListProjectTags)
				withProject.POST("/tags", requireProjectManager, tagHandler.CreateProjectTag)
				withProject.PATCH("/tags/:tag_id", requireProjectManager, tagHandler.UpdateProjectTag)
				withProject.DELETE("/tags/:tag_id", requireProjectManager, tagHandler.DeleteProjectTag)
			}
		}

		// タスク系エンドポイント（:workspace_idを含まないパス）
		tasks := api.Group("/tasks", requireAuth)
		{
			withTask := tasks.Group("/:task_id", requireTaskAccess)
			{
				withTask.GET("", taskHandler.Get)
				withTask.PATCH("", taskHandler.Update)
				withTask.DELETE("", taskHandler.Delete)

				withTask.POST("/subtasks", taskHandler.CreateSubtask)
				withTask.GET("/subtasks", taskHandler.ListSubtasks)
				withTask.PUT("/assignees", taskHandler.PutAssignees)
				withTask.PUT("/tags", taskHandler.PutTags)
				withTask.GET("/assignable-tags", taskHandler.ListAssignableTags)

				withTask.POST("/pin", taskHandler.Pin)
				withTask.DELETE("/pin", taskHandler.Unpin)

				withTask.GET("/dependencies", taskHandler.ListDependencies)
				withTask.POST("/dependencies", taskHandler.CreateDependency)
				withTask.DELETE("/dependencies/:dependency_id", taskHandler.DeleteDependency)

				withTask.GET("/attachments", attachmentHandler.ListAttachments)
				withTask.POST("/attachments", attachmentHandler.CreateAttachment)

				withTask.GET("/mentionable-members", commentHandler.MentionableMembers)
				withTask.POST("/comments", commentHandler.CreateComment)
				withTask.POST("/github-issue/comment", taskHandler.PostGitHubIssueComment)
				withTask.GET("/comments", commentHandler.ListComments)
				withTask.PATCH("/comments/:comment_id", commentHandler.UpdateComment)
				withTask.DELETE("/comments/:comment_id", commentHandler.DeleteComment)
				withTask.POST("/comments/:comment_id/reactions", reactionHandler.ToggleCommentReaction)

				// リアクション（説明欄、タスク単位、2026-08-28追加）
				withTask.POST("/reactions", reactionHandler.ToggleTaskReaction)

				// メモ（M8.5、本人にのみ表示される個人データ）
				withTask.GET("/memo", taskMemoHandler.Get)
				withTask.PUT("/memo", taskMemoHandler.Put)
				withTask.DELETE("/memo", taskMemoHandler.Delete)
				withTask.POST("/memo/attachments", taskMemoHandler.CreateAttachment)
			}
		}

		// 添付ファイル系エンドポイント（:task_idを含まないパス）
		attachments := api.Group("/attachments", requireAuth)
		{
			attachments.DELETE("/:attachment_id", attachmentHandler.DeleteAttachment)
		}

		// 自分自身に関するエンドポイント
		me := api.Group("/me", requireAuth)
		{
			me.PATCH("/profile", authHandler.UpdateProfile)
			me.GET("/supabase-token", authHandler.SupabaseToken)
			me.GET("/hover-settings", authHandler.GetHoverSettings)
			me.PATCH("/hover-settings", authHandler.UpdateHoverSettings)
			me.GET("/calendar-settings", authHandler.GetCalendarSettings)
			me.PATCH("/calendar-settings", authHandler.UpdateCalendarSettings)
			me.POST("/calendar-sync", authHandler.ManualCalendarSync)
			me.GET("/reminder-settings", authHandler.GetReminderSettings)
			me.PATCH("/reminder-settings", authHandler.UpdateReminderSettings)
		}

		// 通知系エンドポイント
		notifications := api.Group("/notifications", requireAuth)
		{
			notifications.GET("", notificationHandler.List)
			notifications.PATCH("/:notification_id/read", notificationHandler.MarkRead)
			notifications.GET("/push-public-key", notificationHandler.PushPublicKey)
			notifications.POST("/subscribe", notificationHandler.Subscribe)
			notifications.DELETE("/subscribe", notificationHandler.Unsubscribe)
		}

		// フィードバック（M8。ワークスペースメンバーシップは問わないためrequireAuthのみ）
		feedback := api.Group("/feedback", requireAuth)
		{
			feedback.POST("", feedbackHandler.Create)
		}

		// 内部cronエンドポイント（Cloud Schedulerからのみ、requireAuthを経由しない。M7）
		internalCron := api.Group("/internal/cron", requireInternalCronSecret)
		{
			internalCron.POST("/reminders", cronHandler.Reminders)
		}
	}

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
