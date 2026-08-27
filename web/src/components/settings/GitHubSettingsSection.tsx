"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";

import { apiFetch } from "@/lib/apiClient";
import Button from "@/components/ui/Button";
import { Input } from "@/components/ui/fields";

type GitHubSettings = { connected: boolean };

// GitHub Issue連携設定（ワークスペース共通、M8.5後追加）。Personal Access Tokenは
// AES-256-GCMで暗号化してworkspaces.github_tokenに保存し、フロントには
// {connected: boolean}のみを返す（生の値は二度と表示しない）。設定・解除は
// ワークスペースメンバーなら誰でも可（当初Owner限定だったが、限られすぎるとの
// ユーザーフィードバックにより変更、2026-08-27）。
export default function GitHubSettingsSection() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;

  const [settings, setSettings] = useState<GitHubSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [token, setToken] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    apiFetch<GitHubSettings>(`/workspaces/${workspaceId}/github-settings`)
      .then(setSettings)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [workspaceId]);

  async function handleSave() {
    if (!token.trim()) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await apiFetch<GitHubSettings>(`/workspaces/${workspaceId}/github-settings`, {
        method: "PATCH",
        body: JSON.stringify({ token: token.trim() }),
      });
      setSettings(updated);
      setToken("");
      setEditing(false);
    } catch {
      setError("GitHub連携設定の更新に失敗しました");
    } finally {
      setSaving(false);
    }
  }

  async function handleDisconnect() {
    if (!window.confirm("GitHub連携を解除しますか？")) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await apiFetch<GitHubSettings>(`/workspaces/${workspaceId}/github-settings`, {
        method: "PATCH",
        body: JSON.stringify({ token: "" }),
      });
      setSettings(updated);
    } catch {
      setError("GitHub連携の解除に失敗しました");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-1 text-sm font-semibold text-foreground">GitHub Issue連携</h2>
      <p className="mb-3 text-xs text-muted-foreground">
        タスク詳細から、リンクしたGitHub Issueへコメントを投稿・更新できるようにします
        （ワークスペース共通の設定、メンバーなら誰でも変更できます）。
      </p>
      {error && <p className="mb-2 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-foreground">{settings?.connected ? "連携済み" : "未連携"}</p>

          {editing ? (
              <div className="flex flex-col gap-2">
                <Input
                  type="password"
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  placeholder="GitHub Personal Access Token"
                  className="text-sm"
                  autoComplete="off"
                />
                <div className="flex gap-2">
                  <Button variant="primary" size="sm" disabled={!token.trim() || saving} onClick={handleSave}>
                    保存
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => {
                      setEditing(false);
                      setToken("");
                    }}
                  >
                    キャンセル
                  </Button>
                </div>
              </div>
            ) : (
              <div className="flex gap-2">
                <Button variant="secondary" size="sm" onClick={() => setEditing(true)}>
                  {settings?.connected ? "トークンを再設定" : "トークンを設定"}
                </Button>
                {settings?.connected && (
                  <Button variant="ghost" size="sm" disabled={saving} onClick={handleDisconnect}>
                    連携を解除
                  </Button>
                )}
              </div>
            )}
        </div>
      )}
    </section>
  );
}
