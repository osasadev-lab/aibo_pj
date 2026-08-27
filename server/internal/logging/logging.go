// Package logging は「誰が・いつ・どこで・何の処理を行ったか」を追えるようにする
// ための構造化ログ（JSON、log/slog）をまとめる（2026-08-27追加）。
//
// 2段構えにしている：
//  1. RequestLogger（ginミドルウェア）：全リクエストについて、認証済みなら
//     user_idを含めてmethod/path/status/所要時間を1行のJSONで記録する。
//     これだけで「誰が・いつ・どのエンドポイントに」の大部分をカバーできる。
//  2. Action（ハンドラから明示的に呼ぶ）：ワークスペース/プロジェクト/タスクの
//     作成・更新・削除等、特に重要な操作について「誰が・何を・対象は」を
//     アクセスログとは別の1行として記録する（Cloud Logging等でのアラート・
//     検索対象にしやすくする目的。activity_logsへのDB記録とは別物で、
//     こちらはアプリのプロセスログとして残す）。
package logging

import (
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// Logger はアプリケーション全体で共有する構造化ロガー。標準出力へJSON形式で出す
// （Cloud Runはstdout/stderrをそのままCloud Loggingに取り込むため、フォーマット
// 変換の追加設定は不要）。
var Logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

// RequestLogger はリクエスト1件ごとに1行のJSONログを出力するginミドルウェア。
// gin.Default()付属のテキストロガーを置き換える想定（main.goでgin.New()+これ+
// gin.Recovery()を使う）。user_idはハンドラ実行後（c.Next()の後）に
// middleware.CurrentUserで取得するため、RequireAuthより前に登録しても後でも動く
// （c.Next()がチェーン全体を実行し終えてから戻ってくるため）。
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", fullPath(c)),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
		}
		if u := middleware.CurrentUser(c); u != nil {
			attrs = append(attrs, slog.String("user_id", u.ID.String()), slog.String("user_email", u.Email))
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("errors", c.Errors.String()))
		}

		level := slog.LevelInfo
		switch {
		case c.Writer.Status() >= 500:
			level = slog.LevelError
		case c.Writer.Status() >= 400:
			level = slog.LevelWarn
		}
		Logger.LogAttrs(c.Request.Context(), level, "http_request", attrs...)
	}
}

func fullPath(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	return c.Request.URL.Path
}

// Action は主要な状態変更操作（作成・更新・削除等）を「誰が・何を」の形で記録する。
// ハンドラが処理成功を確定させた直後（DBコミット後）に呼ぶこと。detailsには
// 対象のID・名前等、後から追跡する際に必要になる情報を入れる。
func Action(c *gin.Context, action string, details map[string]any) {
	attrs := make([]slog.Attr, 0, len(details)+3)
	attrs = append(attrs, slog.String("action", action))
	if u := middleware.CurrentUser(c); u != nil {
		attrs = append(attrs, slog.String("user_id", u.ID.String()), slog.String("user_name", u.Name))
	}
	for k, v := range details {
		attrs = append(attrs, slog.Any(k, v))
	}
	Logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "action", attrs...)
}
