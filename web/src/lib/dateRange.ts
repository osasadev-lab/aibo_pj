// タスクの開始日時・期限（2026-08-27追加、日時範囲対応）を扱う純粋関数群。
// APIの値は"YYYY-MM-DD HH:MM"（時刻未設定時はnull）、日付のみの文字列（カレンダーの
// セルキー等）と混在する箇所があるため、日付部分/時刻部分の抽出をここに集約する。

// "YYYY-MM-DD HH:MM"または"YYYY-MM-DD"の先頭10文字（日付部分）を取り出す。
export function datePart(value: string): string {
  return value.slice(0, 10);
}

// "YYYY-MM-DD HH:MM"の11文字目以降（時刻部分）を取り出す。時刻を含まない場合は空文字。
export function timePart(value: string): string {
  return value.length > 11 ? value.slice(11, 16) : "";
}

// 日付と時刻を"YYYY-MM-DD HH:MM"に結合する。dateが空なら空文字（未設定）を返す。
// timeが空の場合は00:00を補う（日付だけ選ばれた状態でも常に有効な値にするため）。
export function combineDateTime(date: string, time: string): string {
  if (!date) return "";
  return `${date} ${time || "00:00"}`;
}

// start_date/due_dateの片方または両方から表示用の範囲ラベルを組み立てる
// （2026-08-27追加、プロジェクト画面のカンバンカード・ガントチャートで共用）。
// 両方未設定ならnull（呼び出し側で「表示しない」判断に使う）。
export function formatDateRangeLabel(start: string | null, due: string | null): string | null {
  if (start && due) return `${start} 〜 ${due}`;
  if (start) return `${start} 〜（期限未設定）`;
  if (due) return `〜 ${due}`;
  return null;
}

// "YYYY-MM-DD[ HH:MM]"を月内の日インデックス（1始まり）に変換する。月の範囲外なら
// 開始日は1、終了日はdaysInMonthにクリップする（バーが月をまたぐ場合の見切れ表示、
// ガントチャート用）。時刻が付いていても日付部分だけを見る。
export function clampDayIndex(
  dateStr: string,
  y: number,
  m: number,
  totalDays: number,
  isEnd: boolean,
): number {
  const [dy, dm, dd] = datePart(dateStr).split("-").map(Number);
  if (dy < y || (dy === y && dm < m)) return isEnd ? 0 : 1;
  if (dy > y || (dy === y && dm > m)) return isEnd ? totalDays : totalDays + 1;
  return dd;
}
