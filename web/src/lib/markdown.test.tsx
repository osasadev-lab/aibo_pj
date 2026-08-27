import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import { renderMarkdown } from "./markdown";

// renderMarkdownはdangerouslySetInnerHTMLを使わずReact要素として構築することで
// 構造上XSSの余地を無くす設計（コメント本文のレンダリングに使われる、
// docs/aibo/m4-implementation-plan.md 5章参照）。この前提を回帰テストで守る。
describe("renderMarkdown", () => {
  it("renders bold text", () => {
    render(<div data-testid="out">{renderMarkdown("**bold**")}</div>);
    expect(screen.getByTestId("out").querySelector("strong")).toHaveTextContent("bold");
  });

  it("renders strikethrough text", () => {
    render(<div data-testid="out">{renderMarkdown("~~gone~~")}</div>);
    expect(screen.getByTestId("out").querySelector("s")).toHaveTextContent("gone");
  });

  it("renders inline code", () => {
    render(<div data-testid="out">{renderMarkdown("`code`")}</div>);
    expect(screen.getByTestId("out").querySelector("code")).toHaveTextContent("code");
  });

  it("renders italic text via underscores", () => {
    render(<div data-testid="out">{renderMarkdown("_italic_")}</div>);
    expect(screen.getByTestId("out").querySelector("em")).toHaveTextContent("italic");
  });

  it("renders a bullet list from consecutive '- ' lines", () => {
    render(<div data-testid="out">{renderMarkdown("- one\n- two\n- three")}</div>);
    const items = screen.getByTestId("out").querySelectorAll("li");
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveTextContent("one");
    expect(items[2]).toHaveTextContent("three");
  });

  it("renders an http(s) link as a clickable anchor", () => {
    render(<div data-testid="out">{renderMarkdown("[aisu](https://example.com)")}</div>);
    const link = screen.getByTestId("out").querySelector("a");
    expect(link).toHaveAttribute("href", "https://example.com");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noreferrer");
    expect(link).toHaveTextContent("aisu");
  });

  it("does not render a javascript: URL as a clickable link (XSS guard)", () => {
    render(<div data-testid="out">{renderMarkdown("[click me](javascript:alert(1))")}</div>);
    const container = screen.getByTestId("out");
    expect(container.querySelector("a")).toBeNull();
    // isSafeURL()がhttp/https以外を弾いた場合、リンク記法全体をそのままテキストとして
    // 出力する（現状の実装仕様）。少なくとも実際にクリック可能なリンクにはならないこと、
    // つまりhrefが有効なDOM要素として存在しないことが安全性の核心。
    expect(container.textContent).toContain("click me");
  });

  it("escapes plain text safely (no raw HTML injection)", () => {
    render(<div data-testid="out">{renderMarkdown("<img src=x onerror=alert(1)>")}</div>);
    const container = screen.getByTestId("out");
    // Reactの子要素としてテキストがそのまま描画されるため、<img>タグは実DOM要素として
    // 生成されない（テキストノードとして "<img ...>" という文字列がそのまま表示される）。
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
  });

  it("preserves plain text with no markdown syntax", () => {
    render(<div data-testid="out">{renderMarkdown("hello world")}</div>);
    expect(screen.getByTestId("out")).toHaveTextContent("hello world");
  });
});
