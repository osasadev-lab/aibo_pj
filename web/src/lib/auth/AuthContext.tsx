"use client";

import { createContext, useCallback, useEffect, useState, type ReactNode } from "react";

import { apiFetch, clearToken, setToken as saveToken } from "@/lib/apiClient";

export type User = {
  id: string;
  email: string;
  name: string;
  avatar_url: string | null;
};

type AuthContextValue = {
  user: User | null;
  loading: boolean;
  setToken: (token: string) => Promise<void>;
  logout: () => void;
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

  return (
    <AuthContext.Provider value={{ user, loading, setToken, logout }}>
      {children}
    </AuthContext.Provider>
  );
}
