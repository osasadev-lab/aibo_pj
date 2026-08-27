"use client";

import {
  forwardRef,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  type ForwardedRef,
  type InputHTMLAttributes,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from "react";
import clsx from "clsx";

const fieldBase =
  "w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/70 outline-none transition-colors focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/30";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className, ...props },
  ref,
) {
  return <input ref={ref} className={clsx(fieldBase, className)} {...props} />;
});

function mergeRefs<T>(...refs: Array<ForwardedRef<T> | undefined>) {
  return (node: T | null) => {
    for (const ref of refs) {
      if (!ref) continue;
      if (typeof ref === "function") ref(node);
      else (ref as React.MutableRefObject<T | null>).current = node;
    }
  };
}

// 改行で縦に伸びるテキストエリア（ユーザー要望、2026-08-27）。全画面共通のTextareaで
// 一括対応するため、rows指定は初期高さの目安としてのみ機能し、以降は入力内容の
// scrollHeightに合わせて自動で高さを調整する（スクロールバー付きの固定高にはしない）。
// 最小高はscrollHeightではなくrows×line-height＋padding／borderから算出する
// （getComputedStyleのこれらの値は非表示中でも取得できるため、デフォルト未満に
// 潰れることはない）。一方scrollHeightによる「デフォルト超過分の伸長」は要素が
// 実際に表示されていないと正しく測れない。TaskDetailPanelの説明欄・メモ欄は
// データ読み込み中`hidden`クラスで隠されたまま（保存済みの長文content入りで）
// マウントされるケースがあるため、offsetParentが付く（＝実際に表示される）まで
// requestAnimationFrameで待って再計測する。ResizeObserverはhidden祖先の表示切替に
// 対して発火が不安定だったため採用しなかった（祖先のdisplay:none解除を検知
// できないことがあり、その場合デフォルト高のまま伸びない不具合になっていた）。
export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function Textarea({ className, value, rows, ...props }, ref) {
    const innerRef = useRef<HTMLTextAreaElement | null>(null);

    const resize = useCallback(() => {
      const el = innerRef.current;
      if (!el) return;
      const computed = getComputedStyle(el);
      const lineHeight = parseFloat(computed.lineHeight) || 20;
      const paddingY = (parseFloat(computed.paddingTop) || 0) + (parseFloat(computed.paddingBottom) || 0);
      const borderY = (parseFloat(computed.borderTopWidth) || 0) + (parseFloat(computed.borderBottomWidth) || 0);
      const minHeight = (rows ?? 2) * lineHeight + paddingY + borderY;
      el.style.height = "auto";
      el.style.height = `${Math.max(el.scrollHeight, minHeight)}px`;
    }, [rows]);

    useLayoutEffect(() => {
      resize();
    }, [value, resize]);

    useEffect(() => {
      let rafId: number;
      let stopped = false;
      function waitUntilVisible() {
        if (stopped) return;
        const el = innerRef.current;
        if (el?.offsetParent) {
          resize();
          return;
        }
        rafId = requestAnimationFrame(waitUntilVisible);
      }
      rafId = requestAnimationFrame(waitUntilVisible);
      return () => {
        stopped = true;
        cancelAnimationFrame(rafId);
      };
    }, [resize]);

    return (
      <textarea
        ref={mergeRefs(innerRef, ref)}
        value={value}
        rows={rows}
        className={clsx(fieldBase, "resize-none overflow-hidden", className)}
        {...props}
      />
    );
  },
);

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(function Select(
  { className, ...props },
  ref,
) {
  return <select ref={ref} className={clsx(fieldBase, "cursor-pointer", className)} {...props} />;
});
