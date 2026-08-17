"use client";

import { createClient, type SupabaseClient } from "@supabase/supabase-js";

import { apiFetch } from "@/lib/apiClient";

let cachedClient: SupabaseClient | null = null;
let tokenCache: { token: string; expiresAt: number } | null = null;

async function getSupabaseToken(): Promise<string | null> {
  if (tokenCache && Date.now() < tokenCache.expiresAt) {
    return tokenCache.token;
  }
  try {
    const { token } = await apiFetch<{ token: string }>("/me/supabase-token");
    // トークンのTTLは1時間（server/internal/auth/supabase.go）。50分でリフレッシュする。
    tokenCache = { token, expiresAt: Date.now() + 50 * 60 * 1000 };
    return token;
  } catch {
    return null;
  }
}

// Supabase RealtimeでRLSを通すための、アプリ自前のBearerトークン（localStorage）とは
// 別系統の短命トークン（/me/supabase-token）で認証したクライアントを返す。
// docs/aibo/m3-implementation-plan.mdのSupabase JWTブリッジ設計参照。
export function getSupabaseClient(): SupabaseClient {
  if (cachedClient) return cachedClient;
  cachedClient = createClient(
    process.env.NEXT_PUBLIC_SUPABASE_URL!,
    process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!,
    { accessToken: getSupabaseToken },
  );
  return cachedClient;
}
