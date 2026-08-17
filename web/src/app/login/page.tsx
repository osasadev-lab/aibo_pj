"use client";

import { API_BASE_URL } from "@/lib/apiClient";

export default function LoginPage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-6 bg-zinc-50 font-sans dark:bg-black">
      <h1 className="text-3xl font-semibold tracking-tight text-black dark:text-zinc-50">aibo</h1>
      <a
        href={`${API_BASE_URL}/auth/google/login`}
        className="rounded-md bg-black px-6 py-3 text-sm font-medium text-white hover:bg-zinc-800 dark:bg-white dark:text-black dark:hover:bg-zinc-200"
      >
        Googleでログイン
      </a>
    </div>
  );
}
