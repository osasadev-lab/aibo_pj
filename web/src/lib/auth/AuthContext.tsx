"use client";

import { createContext, useCallback, useEffect, useState, type ReactNode } from "react";

import { apiFetch, clearToken, setToken as saveToken } from "@/lib/apiClient";

export type User = {
  id: string;
  email: string;
  name: string;
  avatar_url: string | null;
  // R2（添付ファイル）が未設定（ローカル開発でR2を無効化中等）かどうか。
  // falseの場合、フロントは添付ファイルUI自体を出さずエラーを起こさせない。
  storage_enabled: boolean;
};

type AuthContextValue = {
  user: User | null;
  loading: boolean;
  setToken: (token: string) => Promise<void>;
  logout: () => void;
  // プロフィール更新（表示名変更等）後、再フェッチ無しでローカルのuser表示
  // （サイドバーのアバター名等）に即座に反映するための部分更新（M8追加）。
  updateUser: (patch: Partial<User>) => void;
};

export const AuthContext = createContext<AuthContextValue | null>(null);

function hasStoredToken(): boolean {
  return typeof window !== "undefined" && Boolean(window.localStorage.getItem("aibo_token"));
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(hasStoredToken);

  useEffect(() => {
    if (!hasStoredToken()) return;
    let ignore = false;
    apiFetch<User>("/auth/me")
      .then((me) => {
        if (!ignore) setUser(me);
      })
      .catch(() => {
        if (!ignore) setUser(null);
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, []);

  const setToken = useCallback(async (token: string) => {
    saveToken(token);
    setLoading(true);
    try {
      const me = await apiFetch<User>("/auth/me");
      setUser(me);
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  const logout = useCallback(() => {
    clearToken();
    setUser(null);
  }, []);

  const updateUser = useCallback((patch: Partial<User>) => {
    setUser((prev) => (prev ? { ...prev, ...patch } : prev));
  }, []);

  return (
    <AuthContext.Provider value={{ user, loading, setToken, logout, updateUser }}>
      {children}
    </AuthContext.Provider>
  );
}
