import { describe, expect, it } from "vitest";

import { clampDayIndex, combineDateTime, datePart, formatDateRangeLabel, timePart } from "./dateRange";

describe("datePart", () => {
  it("extracts the date portion from a datetime string", () => {
    expect(datePart("2026-08-23 14:30")).toBe("2026-08-23");
  });

  it("returns the value unchanged when already date-only", () => {
    expect(datePart("2026-08-23")).toBe("2026-08-23");
  });
});

describe("timePart", () => {
  it("extracts the time portion from a datetime string", () => {
    expect(timePart("2026-08-23 14:30")).toBe("14:30");
  });

  it("returns empty string when there is no time portion", () => {
    expect(timePart("2026-08-23")).toBe("");
    expect(timePart("")).toBe("");
  });
});

describe("combineDateTime", () => {
  it("combines a date and time into the API format", () => {
    expect(combineDateTime("2026-08-23", "14:30")).toBe("2026-08-23 14:30");
  });

  it("defaults the time to 00:00 when omitted", () => {
    expect(combineDateTime("2026-08-23", "")).toBe("2026-08-23 00:00");
  });

  it("returns empty string when date is empty (unset)", () => {
    expect(combineDateTime("", "14:30")).toBe("");
    expect(combineDateTime("", "")).toBe("");
  });
});

describe("formatDateRangeLabel", () => {
  it("shows both sides when both are set", () => {
    expect(formatDateRangeLabel("2026-08-23 09:00", "2026-08-23 18:30")).toBe(
      "2026-08-23 09:00 〜 2026-08-23 18:30",
    );
  });

  it("notes the missing due date when only start is set", () => {
    expect(formatDateRangeLabel("2026-08-23 09:00", null)).toBe("2026-08-23 09:00 〜（期限未設定）");
  });

  it("shows only the due date when start is unset", () => {
    expect(formatDateRangeLabel(null, "2026-08-23 18:30")).toBe("〜 2026-08-23 18:30");
  });

  it("returns null when neither is set", () => {
    expect(formatDateRangeLabel(null, null)).toBeNull();
  });
});

describe("clampDayIndex", () => {
  it("returns the day-of-month for a date within the target month", () => {
    expect(clampDayIndex("2026-08-23", 2026, 8, 31, false)).toBe(23);
    expect(clampDayIndex("2026-08-23", 2026, 8, 31, true)).toBe(23);
  });

  it("does not break on a datetime string with a time suffix (regression: previously split('-') on the time part produced NaN)", () => {
    expect(clampDayIndex("2026-08-23 14:30", 2026, 8, 31, false)).toBe(23);
    expect(clampDayIndex("2026-08-23 14:30", 2026, 8, 31, true)).toBe(23);
  });

  it("clamps a date before the target month to day 1 (start) or 0 (end)", () => {
    expect(clampDayIndex("2026-07-31 09:00", 2026, 8, 31, false)).toBe(1);
    expect(clampDayIndex("2026-07-31 09:00", 2026, 8, 31, true)).toBe(0);
  });

  it("clamps a date after the target month to totalDays (start) or totalDays+1 (end)", () => {
    expect(clampDayIndex("2026-09-01 09:00", 2026, 8, 31, false)).toBe(32);
    expect(clampDayIndex("2026-09-01 09:00", 2026, 8, 31, true)).toBe(31);
  });
});
