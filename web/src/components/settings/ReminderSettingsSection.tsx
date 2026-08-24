"use client";

import { useEffect, useState } from "react";

import { apiFetch } from "@/lib/apiClient";
import { Select } from "@/components/ui/fields";

type ReminderSettings = {
  due_today_enabled: boolean;
  due_today_time: string | null;
  overdue_enabled: boolean;
  overdue_time: string | null;
};

// 15分刻みの時刻選択肢（"00:00"〜"23:45"、M7設計判断11）。
const TIME_SLOTS = Array.from({ length: 96 }, (_, i) => {
  const h = Math.floor(i / 4);
  const m = (i % 4) * 15;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
});

const DEFAULT_SETTINGS: ReminderSettings = {
  due_today_enabled: false,
  due_today_time: null,
  overdue_enabled: false,
  overdue_time: null,
};

// リマインダー個人設定（M7追加）。「今日期限のタスク」「期限を過ぎているタスク」を
// それぞれ独立してON/OFF・時刻設定できる（両方同時にONにできる）。デフォルトはOFF。
export default function ReminderSettingsSection() {
  const [settings, setSettings] = useState<ReminderSettings>(DEFAULT_SETTINGS);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    apiFetch<ReminderSettings>("/me/reminder-settings")
      .then(setSettings)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  async function save(next: ReminderSettings) {
    const prev = settings;
    setSettings(next);
    setError(null);
    try {
      const updated = await apiFetch<ReminderSettings>("/me/reminder-settings", {
        method: "PATCH",
        body: JSON.stringify(next),
      });
      setSettings(updated);
    } catch {
      setSettings(prev);
      setError("リマインダー設定の更新に失敗しました");
    }
  }

  function handleToggle(key: "due_today" | "overdue", enabled: boolean) {
    const timeKey = `${key}_time` as const;
    const enabledKey = `${key}_enabled` as const;
    save({
      ...settings,
      [enabledKey]: enabled,
      [timeKey]: enabled ? (settings[timeKey] ?? "09:00") : settings[timeKey],
    });
  }

  function handleTimeChange(key: "due_today" | "overdue", time: string) {
    const timeKey = `${key}_time` as const;
    save({ ...settings, [timeKey]: time });
  }

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-1 text-sm font-semibold text-foreground">リマインダー通知</h2>
      <p className="mb-3 text-xs text-muted-foreground">
        指定した時刻に、対象タスクの件数をまとめた通知が届きます（1日1回）。
      </p>
      {error && <p className="mb-2 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : (
        <div className="flex flex-col gap-4">
          <ReminderRow
            label="今日期限のタスクを通知"
            enabled={settings.due_today_enabled}
            time={settings.due_today_time}
            onToggle={(v) => handleToggle("due_today", v)}
            onTimeChange={(v) => handleTimeChange("due_today", v)}
          />
          <ReminderRow
            label="期限を過ぎているタスクを通知"
            enabled={settings.overdue_enabled}
            time={settings.overdue_time}
            onToggle={(v) => handleToggle("overdue", v)}
            onTimeChange={(v) => handleTimeChange("overdue", v)}
          />
        </div>
      )}
    </section>
  );
}

function ReminderRow({
  label,
  enabled,
  time,
  onToggle,
  onTimeChange,
}: {
  label: string;
  enabled: boolean;
  time: string | null;
  onToggle: (enabled: boolean) => void;
  onTimeChange: (time: string) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
        <input type="checkbox" checked={enabled} onChange={(e) => onToggle(e.target.checked)} />
        {label}
      </label>
      {enabled && (
        <Select
          value={time ?? "09:00"}
          onChange={(e) => onTimeChange(e.target.value)}
          className="w-auto max-w-[7rem]"
        >
          {TIME_SLOTS.map((slot) => (
            <option key={slot} value={slot}>
              {slot}
            </option>
          ))}
        </Select>
      )}
    </div>
  );
}
