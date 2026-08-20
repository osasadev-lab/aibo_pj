// アプリ全体で共用する型。API直下の形をコンポーネントごとに個別定義すると、
// バックエンドのレスポンス形が変わった際に一部の画面だけ追従し忘れて型エラーに
// 気づけない（＝実行時までエラーが出ない）リスクがあるため、ここに集約する。

// ワークスペースメンバーの最小限の表示情報（氏名・メール等）。
// メンバー選択UI（MemberPicker）やメンション候補、担当者候補等、
// ロール情報を必要としない箇所で使う。
export type MemberSummary = {
  id: string;
  user_id: string;
  name: string;
  email: string;
};

// ワークスペースメンバー一覧（メンバー管理画面）用。ロール・アイコンを含む。
export type WorkspaceMember = MemberSummary & {
  avatar_url: string | null;
  role: "owner" | "member";
};

// プロジェクト参画メンバー用。責任者(manager)/スタッフ(staff)のロールを含む。
export type ProjectMemberSummary = MemberSummary & {
  role: "manager" | "staff";
};

export type Workspace = {
  id: string;
  name: string;
  role: "owner" | "member";
};

// タグ。project_idがnullならワークスペース共通タグ（Owner限定管理、単体タスク・
// 全プロジェクトのタスクから使える）、非nullならそのプロジェクト専用タグ。
export type Tag = {
  id: string;
  project_id: string | null;
  name: string;
  color: string;
};

// ハイライト機能（GET /workspaces/:id/activity、M5）の1行。
// project.deleted/task.deletedはproject_id/task_idがnullになる
// （対象が既に存在しないため）。その場合はpayload内の情報で表示を補う。
export type ActivityLogEntry = {
  id: string;
  action_type: string;
  actor_id: string;
  actor_name?: string;
  project_id: string | null;
  project_name?: string;
  task_id: string | null;
  task_title?: string;
  payload: Record<string, unknown> | null;
  created_at: string;
};
