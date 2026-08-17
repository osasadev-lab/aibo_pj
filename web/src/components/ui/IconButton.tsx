"use client";

import { forwardRef, type ButtonHTMLAttributes } from "react";
import clsx from "clsx";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  size?: "sm" | "md";
};

// アイコンのみの正方形ボタン（閉じる・削除アイコン等）。
const IconButton = forwardRef<HTMLButtonElement, Props>(function IconButton(
  { className, size = "md", type = "button", ...props },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={clsx(
        "inline-flex shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-surface-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500/50 disabled:opacity-50",
        size === "sm" ? "h-7 w-7" : "h-9 w-9",
        className,
      )}
      {...props}
    />
  );
});

export default IconButton;
