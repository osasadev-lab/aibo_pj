"use client";

import { useEffect, useState, type ReactNode } from "react";
import { X } from "lucide-react";
import clsx from "clsx";

import IconButton from "@/components/ui/IconButton";

type Props = {
  title: string;
  onClose: () => void;
  children: ReactNode;
};

// 右サイドバーからスライドインする共通パネル（タスク詳細・参画メンバー・メンバー・
// 通知など、右側から出す表示はすべてこれで統一する）。
export default function SidePanel({ title, onClose, children }: Props) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const id = setTimeout(() => setVisible(true), 10);
    return () => clearTimeout(id);
  }, []);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-40">
      <div
        className={clsx(
          "absolute inset-0 bg-black/40 transition-opacity",
          visible ? "opacity-100" : "opacity-0",
        )}
        onClick={onClose}
      />
      <div
        className={clsx(
          "absolute right-0 top-0 flex h-full w-full max-w-xl flex-col border-l border-border bg-surface shadow-2xl transition-transform duration-200",
          visible ? "translate-x-0" : "translate-x-full",
        )}
      >
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{title}</h2>
          <IconButton size="sm" onClick={onClose} title="閉じる">
            <X className="h-4 w-4" />
          </IconButton>
        </div>
        <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
      </div>
    </div>
  );
}
