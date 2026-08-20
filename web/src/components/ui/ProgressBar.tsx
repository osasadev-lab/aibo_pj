import clsx from "clsx";

// ステータス別に色分けしたセグメント（横幅%指定）。外部チャートライブラリを
// 追加せず、DatePicker/markdownレンダラーと同じ方針で自前実装する。
export type ProgressSegment = {
  key: string;
  value: number;
  className: string;
  label: string;
};

export default function ProgressBar({
  segments,
  total,
  className,
}: {
  segments: ProgressSegment[];
  total: number;
  className?: string;
}) {
  return (
    <div
      className={clsx(
        "flex h-3 w-full overflow-hidden rounded-full bg-surface-muted",
        className,
      )}
    >
      {total > 0 &&
        segments
          .filter((s) => s.value > 0)
          .map((s) => (
            <div
              key={s.key}
              title={`${s.label}: ${s.value}`}
              className={clsx("h-full", s.className)}
              style={{ width: `${(s.value / total) * 100}%` }}
            />
          ))}
    </div>
  );
}

// シンプルな単一値バー（担当者別の完了率等、2値の比較に使う）。
export function SingleBar({
  value,
  total,
  className,
}: {
  value: number;
  total: number;
  className?: string;
}) {
  const pct = total > 0 ? (value / total) * 100 : 0;
  return (
    <div className={clsx("h-2.5 w-full overflow-hidden rounded-full bg-surface-muted", className)}>
      <div className="h-full rounded-full bg-indigo-500 transition-all dark:bg-indigo-400" style={{ width: `${pct}%` }} />
    </div>
  );
}
