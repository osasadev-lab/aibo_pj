"use client";

import type { RefObject } from "react";
import { Bold, Code, Italic, Link as LinkIcon, List, Strikethrough } from "lucide-react";

import IconButton from "@/components/ui/IconButton";

type Props = {
  textareaRef: RefObject<HTMLTextAreaElement | null>;
  value: string;
  onChange: (value: string) => void;
};

// 選択範囲の前後にbefore/afterを挿入する（選択が無い場合はplaceholderを挿入して選択状態にする）。
function wrapSelection(
  textarea: HTMLTextAreaElement,
  value: string,
  onChange: (v: string) => void,
  before: string,
  after: string,
  placeholder: string,
) {
  const start = textarea.selectionStart ?? value.length;
  const end = textarea.selectionEnd ?? value.length;
  const selected = value.slice(start, end) || placeholder;
  const next = value.slice(0, start) + before + selected + after + value.slice(end);
  onChange(next);
  requestAnimationFrame(() => {
    textarea.focus();
    const from = start + before.length;
    textarea.setSelectionRange(from, from + selected.length);
  });
}

// 選択範囲が含まれる行（複数行可）それぞれの先頭にprefixを付ける（箇条書き用）。
function prefixLines(textarea: HTMLTextAreaElement, value: string, onChange: (v: string) => void, prefix: string) {
  const start = textarea.selectionStart ?? value.length;
  const end = textarea.selectionEnd ?? value.length;
  const lineStart = value.lastIndexOf("\n", start - 1) + 1;
  const nextBreak = value.indexOf("\n", end);
  const lineEnd = nextBreak === -1 ? value.length : nextBreak;
  const target = value.slice(lineStart, lineEnd);
  const replaced = target
    .split("\n")
    .map((line) => (line.startsWith(prefix) ? line : prefix + line))
    .join("\n");
  const next = value.slice(0, lineStart) + replaced + value.slice(lineEnd);
  onChange(next);
  requestAnimationFrame(() => textarea.focus());
}

// Slack風の最小限のMarkdownツールバー。選択したtextareaのテキストを囲む/行頭に
// 記法を挿入するだけの軽量実装（本格的なWYSIWYGではない、ユーザー確認済み）。
export default function MarkdownToolbar({ textareaRef, value, onChange }: Props) {
  function run(fn: (t: HTMLTextAreaElement) => void) {
    const el = textareaRef.current;
    if (!el) return;
    fn(el);
  }

  return (
    <div className="flex items-center gap-0.5 rounded-t-lg border border-b-0 border-border bg-surface-muted px-1 py-0.5">
      <IconButton
        size="sm"
        type="button"
        title="太字"
        onClick={() => run((t) => wrapSelection(t, value, onChange, "**", "**", "太字"))}
      >
        <Bold className="h-3.5 w-3.5" />
      </IconButton>
      <IconButton
        size="sm"
        type="button"
        title="斜体"
        onClick={() => run((t) => wrapSelection(t, value, onChange, "_", "_", "斜体"))}
      >
        <Italic className="h-3.5 w-3.5" />
      </IconButton>
      <IconButton
        size="sm"
        type="button"
        title="取り消し線"
        onClick={() => run((t) => wrapSelection(t, value, onChange, "~~", "~~", "取り消し線"))}
      >
        <Strikethrough className="h-3.5 w-3.5" />
      </IconButton>
      <IconButton
        size="sm"
        type="button"
        title="コード"
        onClick={() => run((t) => wrapSelection(t, value, onChange, "`", "`", "code"))}
      >
        <Code className="h-3.5 w-3.5" />
      </IconButton>
      <IconButton
        size="sm"
        type="button"
        title="リンク"
        onClick={() => run((t) => wrapSelection(t, value, onChange, "[", "](https://)", "リンク文字列"))}
      >
        <LinkIcon className="h-3.5 w-3.5" />
      </IconButton>
      <IconButton
        size="sm"
        type="button"
        title="箇条書き"
        onClick={() => run((t) => prefixLines(t, value, onChange, "- "))}
      >
        <List className="h-3.5 w-3.5" />
      </IconButton>
    </div>
  );
}
