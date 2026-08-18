"use client";

import { useEffect, useRef, useState } from "react";
import { Calendar as CalendarIcon, ChevronLeft, ChevronRight, X } from "lucide-react";
import clsx from "clsx";

import IconButton from "@/components/ui/IconButton";

type Props = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
};

const WEEKDAYS = ["日", "月", "火", "水", "木", "金", "土"];

function parse(value: string): { year: number; month: number; day: number } | null {
  const m = value.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!m) return null;
  return { year: Number(m[1]), month: Number(m[2]), day: Number(m[3]) };
}

function toValue(year: number, month: number, day: number): string {
  return `${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}

function formatDisplay(value: string): string {
  const d = parse(value);
  if (!d) return "";
  return `${d.year}/${String(d.month).padStart(2, "0")}/${String(d.day).padStart(2, "0")}`;
}

// カレンダーをクリックして選べる、共通の日付入力UI。ネイティブ<input type="date">は
// ブラウザ・OSごとに操作感が違い直感的でないため、アプリ全体で見た目・操作を統一する
// （タスクの期限、マイタスクの基準日等）。値は既存互換のため"YYYY-MM-DD"文字列のまま。
export default function DatePicker({ value, onChange, placeholder = "日付を選択", className }: Props) {
  const [open, setOpen] = useState(false);
  const today = new Date();
  const [viewYear, setViewYear] = useState(today.getFullYear());
  const [viewMonth, setViewMonth] = useState(today.getMonth() + 1);
  const rootRef = useRef<HTMLDivElement>(null);

  const selected = parse(value);

  function openPicker() {
    const s = parse(value);
    setViewYear(s?.year ?? today.getFullYear());
    setViewMonth(s?.month ?? today.getMonth() + 1);
    setOpen(true);
  }

  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    window.addEventListener("mousedown", handleClick);
    window.addEventListener("keydown", handleKey);
    return () => {
      window.removeEventListener("mousedown", handleClick);
      window.removeEventListener("keydown", handleKey);
    };
  }, [open]);

  function goPrevMonth() {
    if (viewMonth === 1) {
      setViewYear((y) => y - 1);
      setViewMonth(12);
    } else {
      setViewMonth((m) => m - 1);
    }
  }

  function goNextMonth() {
    if (viewMonth === 12) {
      setViewYear((y) => y + 1);
      setViewMonth(1);
    } else {
      setViewMonth((m) => m + 1);
    }
  }

  function selectDay(day: number) {
    onChange(toValue(viewYear, viewMonth, day));
    setOpen(false);
  }

  function selectToday() {
    onChange(toValue(today.getFullYear(), today.getMonth() + 1, today.getDate()));
    setOpen(false);
  }

  function clear(e?: React.MouseEvent | React.KeyboardEvent) {
    e?.stopPropagation();
    onChange("");
    setOpen(false);
  }

  // new Date(year, monthIndex, day)はローカル時刻で直接組み立てるコンストラクタ
  // （文字列パースと違いタイムゾーンでずれない）ため、月末日数・曜日計算に安全に使える。
  const firstWeekday = new Date(viewYear, viewMonth - 1, 1).getDay();
  const numDays = new Date(viewYear, viewMonth, 0).getDate();
  const cells: (number | null)[] = [];
  for (let i = 0; i < firstWeekday; i++) cells.push(null);
  for (let d = 1; d <= numDays; d++) cells.push(d);

  const isToday = (day: number) =>
    viewYear === today.getFullYear() && viewMonth === today.getMonth() + 1 && day === today.getDate();
  const isSelected = (day: number) =>
    !!selected && selected.year === viewYear && selected.month === viewMonth && selected.day === day;

  return (
    <div ref={rootRef} className={clsx("relative", className)}>
      <button
        type="button"
        onClick={() => (open ? setOpen(false) : openPicker())}
        className="flex w-full items-center gap-2 rounded-lg border border-border bg-surface px-3 py-2 text-left text-sm text-foreground outline-none transition-colors hover:bg-surface-muted focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/30"
      >
        <CalendarIcon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className={clsx("flex-1 truncate", !value && "text-muted-foreground/70")}>
          {value ? formatDisplay(value) : placeholder}
        </span>
        {value && (
          <span
            role="button"
            tabIndex={0}
            onClick={clear}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                clear(e);
              }
            }}
            title="クリア"
            className="shrink-0 text-muted-foreground hover:text-foreground"
          >
            <X className="h-3.5 w-3.5" />
          </span>
        )}
      </button>

      {open && (
        <div className="absolute z-20 mt-1 w-64 rounded-lg border border-border bg-surface p-3 shadow-lg">
          <div className="mb-2 flex items-center justify-between">
            <IconButton size="sm" type="button" onClick={goPrevMonth} title="前の月">
              <ChevronLeft className="h-4 w-4" />
            </IconButton>
            <span className="text-sm font-medium text-foreground">
              {viewYear}年{viewMonth}月
            </span>
            <IconButton size="sm" type="button" onClick={goNextMonth} title="次の月">
              <ChevronRight className="h-4 w-4" />
            </IconButton>
          </div>
          <div className="grid grid-cols-7 gap-0.5 text-center text-xs text-muted-foreground">
            {WEEKDAYS.map((w) => (
              <span key={w} className="py-1">
                {w}
              </span>
            ))}
          </div>
          <div className="grid grid-cols-7 gap-0.5">
            {cells.map((day, i) =>
              day === null ? (
                <span key={`empty-${i}`} />
              ) : (
                <button
                  key={day}
                  type="button"
                  onClick={() => selectDay(day)}
                  className={clsx(
                    "flex h-7 w-7 items-center justify-center rounded-full text-xs transition-colors",
                    isSelected(day)
                      ? "bg-indigo-600 text-white"
                      : isToday(day)
                        ? "font-semibold text-indigo-600 dark:text-indigo-400"
                        : "text-foreground hover:bg-surface-muted",
                  )}
                >
                  {day}
                </button>
              ),
            )}
          </div>
          <div className="mt-2 flex items-center justify-between border-t border-border pt-2">
            <button
              type="button"
              onClick={selectToday}
              className="text-xs text-indigo-600 hover:underline dark:text-indigo-400"
            >
              今日
            </button>
            {value && (
              <button type="button" onClick={() => clear()} className="text-xs text-muted-foreground hover:text-foreground">
                クリア
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
