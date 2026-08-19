import type { ReactNode } from "react";

// Slack風の最小限のMarkdown記法（太字/斜体/取り消し線/インラインコード/リンク/箇条書き）を
// 安全にReact要素へ変換する。dangerouslySetInnerHTMLを一切使わず常にReact要素として
// 構築するため、構造上XSSの余地が無い（外部のMarkdownパーサー+サニタイザは導入しない、
// docs/aibo/m4-implementation-plan.md 5章参照）。

const INLINE_REGEX =
  /(\*\*(.+?)\*\*)|(~~(.+?)~~)|(`([^`]+?)`)|(_(.+?)_)|(\[([^\]]+)\]\(([^)]+)\))/g;

function isSafeURL(url: string): boolean {
  return /^https?:\/\//i.test(url);
}

function renderInline(text: string, keyPrefix: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let key = 0;
  INLINE_REGEX.lastIndex = 0;
  while ((match = INLINE_REGEX.exec(text)) !== null) {
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }
    if (match[1]) {
      nodes.push(<strong key={`${keyPrefix}-${key++}`}>{match[2]}</strong>);
    } else if (match[3]) {
      nodes.push(<s key={`${keyPrefix}-${key++}`}>{match[4]}</s>);
    } else if (match[5]) {
      nodes.push(
        <code key={`${keyPrefix}-${key++}`} className="rounded bg-surface-muted px-1 py-0.5 text-xs">
          {match[6]}
        </code>,
      );
    } else if (match[7]) {
      nodes.push(<em key={`${keyPrefix}-${key++}`}>{match[8]}</em>);
    } else if (match[9]) {
      const linkText = match[10];
      const url = match[11];
      if (isSafeURL(url)) {
        nodes.push(
          <a
            key={`${keyPrefix}-${key++}`}
            href={url}
            target="_blank"
            rel="noreferrer"
            className="text-indigo-600 underline dark:text-indigo-400"
          >
            {linkText}
          </a>,
        );
      } else {
        nodes.push(match[9]);
      }
    }
    lastIndex = INLINE_REGEX.lastIndex;
  }
  if (lastIndex < text.length) nodes.push(text.slice(lastIndex));
  return nodes;
}

export function renderMarkdown(text: string): ReactNode {
  const lines = text.split("\n");
  const nodes: ReactNode[] = [];
  let listBuffer: string[] = [];
  let key = 0;

  function flushList() {
    if (listBuffer.length === 0) return;
    const items = listBuffer;
    nodes.push(
      <ul key={`ul-${key++}`} className="list-disc pl-5">
        {items.map((item, i) => (
          <li key={i}>{renderInline(item, `li-${key}-${i}`)}</li>
        ))}
      </ul>,
    );
    listBuffer = [];
  }

  lines.forEach((line, i) => {
    if (line.startsWith("- ")) {
      listBuffer.push(line.slice(2));
      return;
    }
    flushList();
    nodes.push(<span key={`line-${key++}`}>{renderInline(line, `line-${key}`)}</span>);
    if (i < lines.length - 1) nodes.push(<br key={`br-${key++}`} />);
  });
  flushList();

  return <>{nodes}</>;
}
