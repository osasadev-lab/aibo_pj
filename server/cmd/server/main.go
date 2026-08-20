package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/osasadev-lab/aibo_pj/server/internal/config"
	"github.com/osasadev-lab/aibo_pj/server/internal/db"
	"github.com/osasadev-lab/aibo_pj/server/internal/handler"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
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

	router := gin.Default()

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

	authHandler := handler.NewAuthHandler(client, cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret, cfg.GoogleOAuthRedirectURL, cfg.JWTSecret, cfg.SupabaseJWTSecret, cfg.FrontendURL, cookieSecure, r2Client)
	workspaceHandler := handler.NewWorkspaceHandler(client, r2Client)
	memberHandler := handler.NewMemberHandler(client)
	projectHandler := handler.NewProjectHandler(client, r2Client)
	taskHandler := handler.NewTaskHandler(client, r2Client)
	commentHandler := handler.NewCommentHandler(client)
	notificationHandler := handler.NewNotificationHandler(client)
	tagHandler := handler.NewTagHandler(client)
	attachmentHandler := handler.NewAttachmentHandler(client, r2Client)
	calendarHandler := handler.NewCalendarHandler(client)
	progressHandler := handler.NewProgressHandler(client)
	activityHandler := handler.NewActivityHandler(client)

	requireAuth := middleware.RequireAuth(client, cfg.JWTSecret)
	requireWorkspaceMember := middleware.RequireWorkspaceMember(client)
	requireOwner := middleware.RequireOwner()
	requireProjectAccess := middleware.RequireProjectAccess(client)
	requireProjectManager := middleware.RequireProjectManager(client)
	requireTaskAccess := middleware.RequireTaskAccess(client)

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

				withMember.GET("/members", memberHandler.List)
				withMember.POST("/members/invite", requireOwner, memberHandler.Invite)
				withMember.PATCH("/members/:member_id", requireOwner, memberHandler.ChangeRole)
				withMember.DELETE("/members/:member_id", memberHandler.Remove)

				withMember.GET("/projects", projectHandler.List)
				withMember.POST("/projects", projectHandler.Create)
				withMember.GET("/tasks", taskHandler.Search)
				withMember.POST("/tasks", taskHandler.Create)
				withMember.GET("/my-tasks", taskHandler.MyTasks)

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
			}
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

				withTask.GET("/dependencies", taskHandler.ListDependencies)
				withTask.POST("/dependencies", taskHandler.CreateDependency)
				withTask.DELETE("/dependencies/:dependency_id", taskHandler.DeleteDependency)

				withTask.GET("/attachments", attachmentHandler.ListAttachments)
				withTask.POST("/attachments", attachmentHandler.CreateAttachment)

				withTask.GET("/mentionable-members", commentHandler.MentionableMembers)
				withTask.POST("/comments", commentHandler.CreateComment)
				withTask.GET("/comments", commentHandler.ListComments)
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
			me.GET("/supabase-token", authHandler.SupabaseToken)
			me.GET("/hover-settings", authHandler.GetHoverSettings)
			me.PATCH("/hover-settings", authHandler.UpdateHoverSettings)
		}

		// 通知系エンドポイント
		notifications := api.Group("/notifications", requireAuth)
		{
			notifications.GET("", notificationHandler.List)
			notifications.PATCH("/:notification_id/read", notificationHandler.MarkRead)
		}
	}

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
