"use client";

import { X } from "lucide-react";
import clsx from "clsx";

import DatePicker from "@/components/ui/DatePicker";
import { Input } from "@/components/ui/fields";
import IconButton from "@/components/ui/IconButton";
import { combineDateTime, datePart, timePart } from "@/lib/dateRange";

type Props = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
};

// 日付＋時刻（"YYYY-MM-DD HH:MM"）を1つの値として扱う入力UI（2026-08-27追加、
// タスクの期限を日時範囲で管理する対応）。既存のDatePicker（日付のみ）はmy-tasksの
// 基準日フィルタ・Googleカレンダー手動連携の対象日と共用しているため変更せず、
// ここでは日付部分の選択にDatePickerをそのまま再利用し、時刻部分はネイティブの
// input type="time"（fields.tsxの共通スタイル）を横に並べるだけの薄いラッパーにする。
export default function DateTimeField({ value, onChange, placeholder, className }: Props) {
  const date = datePart(value);
  const time = timePart(value);

  return (
    <div className={clsx("flex items-center gap-1.5", className)}>
      <DatePicker
        value={date}
        onChange={(newDate) => onChange(combineDateTime(newDate, time))}
        placeholder={placeholder}
        className="min-w-0 flex-1"
      />
      {/* Inputの共通スタイル（fields.tsx）はw-fullを含むため、クラス名の連結だけでは
          幅を上書きできない（Tailwindのカスケード順は書いた順ではなく生成順に依存する）。
          幅固定のラッパーで囲んでw-fullをその中に収める。 */}
      <div className="w-28 shrink-0">
        <Input
          type="time"
          value={time}
          disabled={!date}
          onChange={(e) => onChange(combineDateTime(date, e.target.value))}
          className="disabled:cursor-not-allowed disabled:opacity-50"
        />
      </div>
      {value && (
        <IconButton size="sm" type="button" title="クリア" onClick={() => onChange("")}>
          <X className="h-3.5 w-3.5" />
        </IconButton>
      )}
    </div>
  );
}
