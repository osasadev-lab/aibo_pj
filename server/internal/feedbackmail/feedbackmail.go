// Package feedbackmail はユーザーからのフィードバック投稿を、コミット後に
// ベストエフォートでosasadev@gmail.com宛にメール通知する（M8）。
//
// pushdelivery/calendarsyncと同じ「DBトランザクションの外・コミット後にgoroutineで
// 実行する」方針（docs/aibo/m8-implementation-plan.md スコープ追加D）。新規サービス
// 契約を避けるため、Go標準ライブラリnet/smtpからGmailのSMTPリレー
// （smtp.gmail.com:587、STARTTLS）を直接呼ぶ。認証にはGoogleアカウントの
// 「アプリパスワード」を使う。
package feedbackmail

import (
	"encoding/base64"
	"fmt"
	"log"
	"mime"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config はSMTP接続情報一式。Hostが空の場合はConfigured()がfalseを返し、
// Async()は何もせずスキップする（未設定でもフィードバック自体のDB保存は成功させるため）。
type Config struct {
	Host        string
	Port        string
	Username    string
	Password    string
	NotifyEmail string
}

// Configured はメール送信に必要な設定が揃っているかどうか。
func (c Config) Configured() bool {
	return c.Host != "" && c.Username != "" && c.Password != "" && c.NotifyEmail != ""
}

// Item は1件のフィードバック投稿。
type Item struct {
	SenderName  string
	SenderEmail string
	WorkspaceID *uuid.UUID
	PagePath    string
	Body        string
	CreatedAt   time.Time
}

// Async はitemをgoroutine内でベストエフォート送信する（呼び出し元はDBコミット後に呼ぶこと）。
// 送信失敗はログ出力のみでエラーを呼び出し元に返さない（feedbacksテーブルへの保存は
// 既に成功しているため、UI上は送信成功として扱う）。
func Async(cfg Config, item Item) {
	if !cfg.Configured() {
		log.Printf("feedbackmail: SMTP not configured, skipping notification email")
		return
	}
	go func() {
		if err := send(cfg, item); err != nil {
			log.Printf("feedbackmail: failed to send notification: %v", err)
			return
		}
		log.Printf("feedbackmail: notification email sent to %s", cfg.NotifyEmail)
	}()
}

func send(cfg Config, item Item) error {
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)

	// 件名に日本語を含むため、生のUTF-8バイト列をそのままヘッダーに置くと
	// 一部のメールクライアント/中継サーバーで文字化けする。RFC 2047に従い
	// mime.QEncoding等でエンコードする（mime.BEncoding/mime.WordEncoderが
	// 自動でMIME encoded-word形式 =?UTF-8?...?= を組み立てる）。
	subject := mime.QEncoding.Encode("UTF-8", "[aisu] フィードバックが届きました")
	// Content-Transfer-Encoding: 8bitは、送信経路の全区間が8bit透過であることを
	// 前提にするが、net/smtp.SendMailはESMTPの8BITMIME拡張（MAIL FROM時のBODY=8BITMIME
	// パラメータ）を明示的に要求しないため、途中経路で8bit文字が壊れるケースがあった
	// （文字化け報告により判明）。base64はASCII文字のみで転送されるため、経路の
	// 8bit対応状況に依存せず確実に届く（RFC 2045、76文字ごとに改行が必要）。
	body := encodeBase64Body(buildBody(item))
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s",
		cfg.Username, cfg.NotifyEmail, subject, body,
	)

	return smtp.SendMail(addr, auth, cfg.Username, []string{cfg.NotifyEmail}, []byte(msg))
}

// encodeBase64Body はRFC 2045に従い、base64文字列を76文字ごとにCRLFで折り返す。
func encodeBase64Body(s string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(s))
	var sb strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		sb.WriteString(encoded[i:end])
		sb.WriteString("\r\n")
	}
	return sb.String()
}

func buildBody(item Item) string {
	workspaceLine := "（ワークスペース外）"
	if item.WorkspaceID != nil {
		workspaceLine = item.WorkspaceID.String()
	}
	pagePath := item.PagePath
	if pagePath == "" {
		pagePath = "（不明）"
	}
	return fmt.Sprintf(
		"投稿者: %s <%s>\n送信日時: %s\nワークスペースID: %s\nページ: %s\n\n%s\n",
		item.SenderName, item.SenderEmail,
		item.CreatedAt.Format("2006-01-02 15:04:05"),
		workspaceLine, pagePath, item.Body,
	)
}
