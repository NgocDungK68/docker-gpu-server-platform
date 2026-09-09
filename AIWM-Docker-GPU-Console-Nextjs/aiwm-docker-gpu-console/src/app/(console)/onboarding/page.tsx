"use client";

import { CheckCircle2, Clipboard, Network, Server, TerminalSquare } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { PageHeader, TableSkeleton } from "@/components/ui/page";
import { useToast } from "@/components/ui/toast";
import { useServers } from "@/features/inventory/use-inventory";
import { relativeTime } from "@/lib/utils/format";

const commands = [
  { title: "Build Agent trên Linux server", command: "go build -trimpath -o bin/aiwm-agent ./cmd/aiwm-agent" },
  { title: "Kiểm tra Docker và NVIDIA NVML", command: "sudo ./bin/aiwm-agent --check" },
  { title: "Chạy thử foreground", command: "sudo ./bin/aiwm-agent" },
];

export default function OnboardingPage() {
  const servers = useServers();
  const { pushToast } = useToast();
  const copy = async (value: string) => { await navigator.clipboard.writeText(value); pushToast({ tone: "success", title: "Đã sao chép lệnh" }); };
  return <><PageHeader eyebrow="Gradual adoption" title="Onboard Docker GPU Agent" description="Cài Agent dần trên từng máy chủ; không restart workload, không cài lại OS và không đưa máy vào Kubernetes." />
    <div className="grid gap-5 xl:grid-cols-[1fr_.75fr]"><div className="space-y-5"><Card><CardHeader title="Quy trình triển khai" description="Lặp lại độc lập cho từng GPU server" /><div className="space-y-0 p-5">{[{ icon: TerminalSquare, title: "Chuẩn bị server", text: "Docker Engine, NVIDIA driver, Container Toolkit và quyền truy cập /var/run/docker.sock." }, { icon: Network, title: "Cấu hình outbound", text: "Đặt AIWM_CONTROL_PLANE_URL, enrollment token, machine ID, tên và placement labels." }, { icon: CheckCircle2, title: "Preflight inventory", text: "Chạy --check, đối chiếu toàn bộ GPU/container cũ trước khi đưa GPU trống vào pool." }, { icon: Server, title: "Bật systemd", text: "Agent chủ động register, heartbeat, report inventory và poll command." }].map(({ icon: Icon, title, text }, index) => <div key={title} className="relative flex gap-4 pb-7 last:pb-0"><div className="relative z-10 grid size-9 shrink-0 place-items-center rounded-full bg-slate-950 text-white"><Icon className="size-4" /></div>{index < 3 && <div className="absolute bottom-0 left-[17px] top-9 w-px bg-slate-200" />}<div><p className="font-bold text-slate-950">{index + 1}. {title}</p><p className="mt-1 text-sm leading-6 text-slate-600">{text}</p></div></div>)}</div></Card><Card><CardHeader title="Lệnh lab" description="Chạy trong repository backend Go, không phải frontend" /><div className="space-y-3 p-4">{commands.map((item) => <div key={item.title} className="rounded-xl border border-slate-200 bg-slate-950 p-4 text-white"><div className="mb-2 flex items-center justify-between gap-3"><p className="text-xs font-semibold text-slate-400">{item.title}</p><button onClick={() => copy(item.command)} className="rounded-md p-1.5 text-slate-400 hover:bg-white/10 hover:text-white" aria-label="Sao chép"><Clipboard className="size-4" /></button></div><code className="block overflow-x-auto text-xs text-slate-200">{item.command}</code></div>)}</div></Card></div>
    <div className="space-y-5"><Card><CardHeader title="Agent đã kết nối" description="Đọc từ GET /api/v1/servers" />{servers.isLoading ? <TableSkeleton rows={4} /> : <div className="divide-y divide-slate-100">{(servers.data ?? []).map((server) => <div key={server.id} className="flex items-center gap-3 p-4"><div className="grid size-9 place-items-center rounded-xl bg-slate-100"><Server className="size-4 text-slate-600" /></div><div className="min-w-0 flex-1"><p className="truncate text-sm font-bold text-slate-950">{server.name}</p><p className="mt-0.5 text-xs text-slate-400">v{server.agentVersion || "—"} · heartbeat {relativeTime(server.lastHeartbeatAt)}</p></div><Badge value={server.status} /></div>)}{!servers.isLoading && !(servers.data?.length) && <div className="p-8 text-center text-sm text-slate-500">Chưa có Agent đăng ký.</div>}</div>}</Card><div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm leading-6 text-amber-900"><b>Lưu ý quyền:</b> truy cập Docker socket tương đương quyền root. Dùng tài khoản service riêng, bảo vệ token và triển khai TLS theo tài liệu backend.</div><Button variant="secondary" className="w-full" onClick={() => copy("docs/agent-deployment.md")}>Mở hướng dẫn: docs/agent-deployment.md</Button></div></div>
  </>;
}
