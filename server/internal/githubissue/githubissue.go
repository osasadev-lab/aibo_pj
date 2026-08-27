// Package githubissue は、aisu側のタスク詳細を要約してリンク済みのGitHub Issueへ
// コメントとして投稿・更新する薄いAPIクライアント（M8.5後の追加要望）。
//
// GitHub Appの登録やOAuthフローは行わず、Personal Access Token（`GITHUB_TOKEN`環境変数、
// feedbackmailと同じ「未設定でもサーバー起動は妨げない」方針）でGitHub REST APIを
// 直接呼ぶ。ユーザーが明示的に押すボタン起点の同期呼び出しのため、結果を
// そのまま呼び出し元へ返せるよう非同期化はしない（api-spec.md「手動モード」と同じ判断）。
package githubissue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// Config はGitHub API呼び出しに必要な設定一式。
type Config struct {
	Token string
}

// Configured はGitHub連携に必要な設定が揃っているかどうか。
func (c Config) Configured() bool {
	return c.Token != ""
}

// IssueRef はGitHub Issue URLから解決した参照先。
type IssueRef struct {
	Owner  string
	Repo   string
	Number int
}

// ErrInvalidIssueURL はgithub_issue_urlがGitHub IssueのURL形式でない場合に返す。
var ErrInvalidIssueURL = errors.New("invalid github issue url")

var issueURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/issues/(\d+)`)

// ParseIssueURL は`https://github.com/{owner}/{repo}/issues/{number}`形式のURLを解析する。
func ParseIssueURL(raw string) (IssueRef, error) {
	m := issueURLPattern.FindStringSubmatch(raw)
	if m == nil {
		return IssueRef{}, ErrInvalidIssueURL
	}
	num, err := strconv.Atoi(m[3])
	if err != nil {
		return IssueRef{}, ErrInvalidIssueURL
	}
	return IssueRef{Owner: m[1], Repo: m[2], Number: num}, nil
}

const apiTimeout = 15 * time.Second

type commentResponse struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
}

// CreateComment はIssueへ新規コメントを投稿する。
func CreateComment(ctx context.Context, cfg Config, ref IssueRef, body string) (commentID int64, htmlURL string, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", ref.Owner, ref.Repo, ref.Number)
	return doCommentRequest(ctx, cfg, http.MethodPost, url, body)
}

// UpdateComment は既存コメントの本文を置き換える（タスク編集内容の反映用）。
func UpdateComment(ctx context.Context, cfg Config, ref IssueRef, commentID int64, body string) (int64, string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments/%d", ref.Owner, ref.Repo, commentID)
	return doCommentRequest(ctx, cfg, http.MethodPatch, url, body)
}

// DeleteComment はコメントを削除する（タスク削除時のクリーンアップ用）。
// 404（コメントが既に存在しない）はエラー扱いしない。
func DeleteComment(ctx context.Context, cfg Config, ref IssueRef, commentID int64) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments/%d", ref.Owner, ref.Repo, commentID)
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	setHeaders(req, cfg)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api error (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func doCommentRequest(ctx context.Context, cfg Config, method, url, body string) (int64, string, error) {
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return 0, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	setHeaders(req, cfg)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return 0, "", fmt.Errorf("github api error (%d): %s", resp.StatusCode, string(b))
	}

	var out commentResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, "", err
	}
	return out.ID, out.HTMLURL, nil
}

func setHeaders(req *http.Request, cfg Config) {
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}
