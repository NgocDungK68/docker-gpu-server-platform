"use client";

import { createContext, useContext, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { identityApi, type Session } from "@/lib/api/identity";

type SessionContextValue = Session & { organizationFilter: string; setOrganizationFilter: (id: string) => void };
const SessionContext = createContext<SessionContextValue | null>(null);

export function SessionGate({ children }: { children: React.ReactNode }) {
  const [organizationFilter, setOrganizationFilter] = useState("");
  const session = useQuery({ queryKey: ["session"], queryFn: identityApi.me, retry: false, refetchInterval: 30000 });
  if (session.isPending) return <div className="grid min-h-screen place-items-center text-slate-500" role="status">Đang xác thực phiên đăng nhập…</div>;
  if (!session.data || session.isError) return <div className="grid min-h-screen place-items-center"><div className="rounded-2xl border bg-white p-8 text-center"><p className="text-red-700">Không xác thực được phiên đăng nhập.</p><button className="btn btn-primary mt-4" onClick={() => void session.refetch()}>Thử lại</button></div></div>;
  return <SessionContext.Provider value={{ ...session.data, organizationFilter: session.data.user.role === "ADMIN" ? organizationFilter : "", setOrganizationFilter }}>{children}</SessionContext.Provider>;
}

export function useSession() {
  const session = useContext(SessionContext);
  if (!session) throw new Error("SessionGate required");
  return session;
}

export function AccountBadge() {
  const session = useSession();
  const client = useQueryClient();
  return <div className="flex flex-wrap items-center justify-end gap-2 text-xs">
    <span aria-label="Tài khoản hiện tại" className="rounded-full bg-red-50 px-3 py-1.5 font-bold text-red-700" title={session.user.role === "ADMIN" ? session.user.username : session.organization.name}>{session.user.role === "ADMIN" ? session.user.username : session.organization.code}</span>
    <button className="rounded-lg border px-2 py-1.5 hover:bg-slate-50" onClick={async () => { try { await identityApi.logout(); await client.cancelQueries(); client.clear(); window.location.replace("/login"); } catch { window.alert("Chưa đăng xuất được. Vui lòng thử lại khi kết nối phục hồi."); } }}>Đăng xuất</button>
  </div>;
}
