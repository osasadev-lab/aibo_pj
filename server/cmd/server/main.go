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

	authHandler := handler.NewAuthHandler(client, cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret, cfg.GoogleOAuthRedirectURL, cfg.JWTSecret, cfg.SupabaseJWTSecret, cfg.FrontendURL, cookieSecure)
	workspaceHandler := handler.NewWorkspaceHandler(client)
	memberHandler := handler.NewMemberHandler(client)
	projectHandler := handler.NewProjectHandler(client)
	taskHandler := handler.NewTaskHandler(client)
	commentHandler := handler.NewCommentHandler(client)
	notificationHandler := handler.NewNotificationHandler(client)

	requireAuth := middleware.RequireAuth(client, cfg.JWTSecret)
	requireWorkspaceMember := middleware.RequireWorkspaceMember(client)
	requireOwner := middleware.RequireOwner()
	requireProjectAccess := middleware.RequireProjectAccess(client)
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

				withMember.GET("/members", memberHandler.List)
				withMember.POST("/members/invite", requireOwner, memberHandler.Invite)
				withMember.PATCH("/members/:member_id", requireOwner, memberHandler.ChangeRole)
				withMember.DELETE("/members/:member_id", memberHandler.Remove)

				withMember.GET("/projects", projectHandler.List)
				withMember.POST("/projects", projectHandler.Create)
				withMember.GET("/tasks", taskHandler.Search)
				withMember.POST("/tasks", taskHandler.Create)
				withMember.GET("/my-tasks", taskHandler.MyTasks)
			}
		}

		// プロジェクト系エンドポイント（:workspace_idを含まないパス）
		projects := api.Group("/projects", requireAuth)
		{
			withProject := projects.Group("/:project_id", requireProjectAccess)
			{
				withProject.GET("", projectHandler.Get)
				withProject.PATCH("", projectHandler.Update)
				withProject.DELETE("", middleware.RequireProjectOwnerOrCreator(), projectHandler.Delete)

				withProject.GET("/members", projectHandler.ListMembers)
				withProject.PUT("/members", middleware.RequireProjectOwnerOrCreator(), projectHandler.PutMembers)

				withProject.GET("/status-columns", projectHandler.ListStatusColumns)
				withProject.POST("/status-columns", projectHandler.CreateStatusColumn)
				withProject.PATCH("/status-columns/:column_id", projectHandler.UpdateStatusColumn)
				withProject.DELETE("/status-columns/:column_id", projectHandler.DeleteStatusColumn)
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

				withTask.GET("/mentionable-members", commentHandler.MentionableMembers)
				withTask.POST("/comments", commentHandler.CreateComment)
				withTask.GET("/comments", commentHandler.ListComments)
			}
		}

		// 自分自身に関するエンドポイント
		me := api.Group("/me", requireAuth)
		{
			me.GET("/supabase-token", authHandler.SupabaseToken)
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
