"use client";

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";

// 右サイドバーからスライドインするパネル（メンバー・通知・ハイライト・ピン留め・
// フィードバック・プロジェクト参画メンバー）はすべてSidePanel（同じz-indexの
// 固定オーバーレイ）を使っている。従来は各パネルの開閉状態を個別のboolean
// （showMembers/showNotifications等、CurrentProjectContextのeditingMembers）で
// 別々に持っていたため、複数を同時にtrueにできてしまい、後からマウントされた
// 方がDOM順で上に重なって手前のパネルを覆い隠す不具合があった（例：ハイライトを
// 開いた状態で通知を開くと、通知パネルはマウントされるがハイライトの裏に隠れて
// 見えない）。「開いているのは常に1つだけ」を保証するため、右パネルの開閉状態を
// この1つのContextに一本化する（ユーザー報告により2026-08-27追加）。
export type RightPanelId =
  | "members"
  | "notifications"
  | "activity"
  | "pinned"
  | "feedback"
  | "project-members"
  | "memos"
  | "guide";

type RightPanelContextValue = {
  active: RightPanelId | null;
  open: (id: RightPanelId) => void;
  close: () => void;
  isOpen: (id: RightPanelId) => boolean;
};

const RightPanelContext = createContext<RightPanelContextValue | null>(null);

export function RightPanelProvider({ children }: { children: ReactNode }) {
  const [active, setActive] = useState<RightPanelId | null>(null);

  const open = useCallback((id: RightPanelId) => setActive(id), []);
  const close = useCallback(() => setActive(null), []);
  const isOpen = useCallback((id: RightPanelId) => active === id, [active]);

  const value = useMemo(() => ({ active, open, close, isOpen }), [active, open, close, isOpen]);

  return <RightPanelContext.Provider value={value}>{children}</RightPanelContext.Provider>;
}

export function useRightPanel(): RightPanelContextValue {
  const ctx = useContext(RightPanelContext);
  if (!ctx) {
    throw new Error("useRightPanel must be used within a RightPanelProvider");
  }
  return ctx;
}
