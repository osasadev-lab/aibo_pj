"use client";

import { LogIn } from "lucide-react";

import { API_BASE_URL } from "@/lib/apiClient";

export default function LoginPage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-8 bg-background px-4">
      <div className="flex flex-col items-center gap-3">
        <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-indigo-600 text-2xl font-bold text-white shadow-lg shadow-indigo-600/30">
          a
        </span>
        <h1 className="text-3xl font-semibold tracking-tight text-foreground">aibo</h1>
        <p className="text-sm text-muted-foreground">チームのタスクを、シンプルに。</p>
      </div>

      <a
        href={`${API_BASE_URL}/auth/google/login`}
        className="inline-flex items-center gap-2 rounded-lg bg-indigo-600 px-6 py-3 text-sm font-medium text-white shadow-sm shadow-indigo-600/20 transition-colors hover:bg-indigo-500 dark:bg-indigo-500 dark:hover:bg-indigo-400"
      >
        <LogIn className="h-4 w-4" />
        Googleでログイン
      </a>
    </div>
  );
}
