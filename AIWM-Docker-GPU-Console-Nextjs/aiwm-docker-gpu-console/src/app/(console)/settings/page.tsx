"use client";

import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Code2, Database, Palette, ServerCog } from "lucide-react";
import { Card, CardHeader } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page";
import { ConnectionStatus } from "@/components/layout/connection-status";
import { appConfig } from "@/config/app";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";

export default function SettingsPage() {
  const health = useQuery({ queryKey: queryKeys.health, queryFn: api.health });
  return <><PageHeader eyebrow="Developer settings" title="Cấu hình frontend" description="Các điểm chỉnh sửa tập trung để người mới không phải tìm rải rác trong source code." />
    <div className="grid gap-5 lg:grid-cols-2"><Card><CardHeader title="Kết nối Control Plane" action={<ConnectionStatus />} /><div className="space-y-4 p-5"><Setting icon={<ServerCog className="size-5" />} title="Backend URL" value={health.data?.backendUrl ?? "AIWM_API_BASE_URL"} note="Sửa trong .env.local; chỉ Next.js server đọc giá trị này." /><Setting icon={<Code2 className="size-5" />} title="Browser API path" value={appConfig.apiProxyBasePath} note="Frontend gọi same-origin proxy, không hard-code localhost trong component." /><Setting icon={<Database className="size-5" />} title="Chế độ dữ liệu" value={appConfig.dataMode} note="Dữ liệu inventory và jobs được đọc từ Control Plane." /></div></Card><Card><CardHeader title="Design tokens" /><div className="space-y-4 p-5"><Setting icon={<Palette className="size-5" />} title="Font giao diện" value="--font-ui" note="Sửa một biến ở src/app/globals.css để đổi font toàn hệ thống." /><Setting icon={<Palette className="size-5 text-[var(--brand-red)]" />} title="Màu Viettel" value="#EE0033" note="--brand-red, --brand-red-dark và --brand-red-soft." /><Setting icon={<CheckCircle2 className="size-5" />} title="Refresh interval" value={`${appConfig.refreshIntervalMs} ms`} note="Sửa NEXT_PUBLIC_REFRESH_INTERVAL_MS trong .env.local." /></div></Card><Card className="lg:col-span-2"><CardHeader title="Các file cần biết" /><div className="grid gap-3 p-5 sm:grid-cols-2 lg:grid-cols-4">{[["API endpoint", "src/lib/api/endpoints.ts"], ["Kiểu dữ liệu", "src/lib/api/types.ts"], ["Cấu hình proxy", "src/config/server.ts"], ["Menu", "src/config/navigation.ts"]].map(([label, path]) => <div key={label} className="rounded-xl border border-slate-200 bg-slate-50 p-4"><p className="text-xs font-semibold text-slate-500">{label}</p><code className="mt-2 block break-all text-xs font-bold text-slate-800">{path}</code></div>)}</div></Card></div>
  </>;
}

function Setting({ icon, title, value, note }: { icon: React.ReactNode; title: string; value: string; note: string }) { return <div className="flex gap-3 rounded-xl border border-slate-200 p-4"><div className="mt-0.5 text-slate-500">{icon}</div><div className="min-w-0"><p className="text-sm font-bold text-slate-950">{title}</p><code className="mt-1 block break-all text-xs text-[var(--brand-red-dark)]">{value}</code><p className="mt-1.5 text-xs leading-5 text-slate-500">{note}</p></div></div>; }
