import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

import { AuthProvider } from "@/lib/auth/AuthContext";
import ServiceWorkerRegister from "@/components/ServiceWorkerRegister";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "aisu",
  description: "チーム/組織向けタスク管理サービス",
  manifest: "/manifest.json",
  icons: {
    icon: "/icons/icon-192.png",
    apple: "/icons/apple-touch-icon.png",
  },
};

export const viewport: Viewport = {
  themeColor: "#0b0b0d",
  // ページ自身がダーク/ライト両方をサポートし、CSSのcolor-schemeで実際の状態を
  // 管理していることをブラウザに明示する（globals.cssのcolor-scheme宣言と対）。
  colorScheme: "dark light",
};

// 個人設定（ダーク/ライト）はサーバー保存せずlocalStorageのみで永続化する
// （docs/aibo/m8-implementation-plan.md 設計判断4：初回ペイント前に同期的に
// 確定させないとFOUCが起きるため、認証後の非同期APIフェッチとは相性が悪い）。
// アプリの既定はダークのため、<html>には最初からdarkクラスを焼き込んでおき、
// このスクリプトは「ライトが選択されている場合のみ」外す（ダーク選択者側の
// フラッシュを避ける。ライト選択者は一瞬だけダーク→ライトに切り替わる）。
const themeInitScript = `
(function () {
  try {
    if (localStorage.getItem("aisu-theme") === "light") {
      document.documentElement.classList.remove("dark");
    }
  } catch (e) {}
})();
`;

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="ja"
      suppressHydrationWarning
      className={`${geistSans.variable} ${geistMono.variable} dark h-full antialiased`}
    >
      <head>
        <script suppressHydrationWarning dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body className="min-h-full flex flex-col">
        <AuthProvider>{children}</AuthProvider>
        <ServiceWorkerRegister />
      </body>
    </html>
  );
}
