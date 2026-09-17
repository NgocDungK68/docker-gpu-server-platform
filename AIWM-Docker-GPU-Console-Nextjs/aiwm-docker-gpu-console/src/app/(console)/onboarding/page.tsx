"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { OrganizationSelect } from "@/features/identity/organization-select";
import { useSession } from "@/features/identity/session";
import { identityApi, type EnrollmentResult } from "@/lib/api/identity";
import { useServers } from "@/features/inventory/use-inventory";
import { Badge } from "@/components/ui/badge";

function parseLabels(text: string) {
  const labels: Record<string,string> = {};
  for (const line of text.split("\n").map(v => v.trim()).filter(Boolean)) {
    const at = line.indexOf("=");
    if (at <= 0) throw new Error("Mỗi label cần có dạng key=value.");
    labels[line.slice(0,at).trim()] = line.slice(at+1).trim();
  }
  return labels;
}

export default function OnboardingPage() {
  const { user,organization } = useSession();
  const admin = user.role === "ADMIN";
  const client = useQueryClient();
  const organizations = useQuery({ queryKey:["organizations"],queryFn:identityApi.organizations,enabled:admin });
  const enrollments = useQuery({ queryKey:["enrollments"],queryFn:identityApi.enrollments });
  const servers = useServers();
  const [displayName,setDisplayName] = useState("");
  const [organizationId,setOrganizationId] = useState(organization.id);
  const [labels,setLabels] = useState("");
  const [result,setResult] = useState<EnrollmentResult | null>(null);
  const [copied,setCopied] = useState(false);
  const create = useMutation({ mutationFn: () => identityApi.createEnrollment({ displayName,labels:parseLabels(labels),...(admin ? {organizationId} : {}) }),onSuccess: data => { setResult(data); setCopied(false); void client.invalidateQueries({queryKey:["enrollments"]}); } });
  const revoke = useMutation({ mutationFn:identityApi.revokeEnrollment,onSuccess:() => { setResult(null); void client.invalidateQueries({queryKey:["enrollments"]}); } });
  const config = result ? "AIWM_CONTROL_PLANE_URL=http://CONTROL_PLANE_HOST:8080\nAIWM_ENROLLMENT_TOKEN=" + result.enrollmentToken + "\nAIWM_AGENT_STATE_FILE=/var/lib/aiwm-agent/state.json" : "";
  return <div className="space-y-6"><div><h1 className="text-2xl font-bold text-slate-900">Kết nối GPU Server</h1><p className="mt-2 text-sm text-slate-500">Enrollment xác lập ownership. Agent + NVML tự phát hiện GPU trên Linux server.</p></div>
    <div className="grid gap-5 xl:grid-cols-2">
      <form className="surface-card space-y-5 p-6" onSubmit={e => { e.preventDefault(); create.mutate(); }}>
        <h2 className="font-bold">1. Tạo Server Enrollment</h2>
        <label className="block"><span className="field-label">Tên hiển thị server</span><input className="field-input" required maxLength={200} value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder="GPU server phòng lab" /></label>
        <div><p className="field-label">Đơn vị sở hữu</p>{admin ? organizations.isError ? <p role="alert" className="text-sm text-red-700">Không tải được đơn vị. <button type="button" onClick={() => void organizations.refetch()}>Thử lại</button></p> : <OrganizationSelect organizations={(organizations.data ?? []).filter(o => o.enabled)} value={organizationId} onChange={setOrganizationId} /> : <p className="rounded-xl border border-red-100 bg-red-50 p-3 text-sm font-semibold text-red-700">{organization.code} · {organization.name}</p>}</div>
        <label className="block"><span className="field-label">Labels (tùy chọn, mỗi dòng key=value)</span><textarea className="field-textarea" rows={3} value={labels} onChange={e => setLabels(e.target.value)} placeholder="location=lab" /></label>
        {create.isError && <p role="alert" className="text-sm text-red-700">Không tạo được enrollment. {create.error.message}</p>}
        <button className="btn btn-primary" disabled={create.isPending || (admin && organizations.isPending)}>{create.isPending ? "Đang tạo…" : "Tạo enrollment token"}</button>
      </form>
      <section className="surface-card space-y-4 p-6"><h2 className="font-bold">2. Cấu hình và chạy Agent</h2><p className="text-sm leading-6 text-slate-600">Linux server cần Docker Engine, NVIDIA Driver/NVML, NVIDIA Container Toolkit và quyền truy cập Docker socket. Agent dùng kết nối outbound tới Control Plane.</p>
        {result ? <><p className="rounded-lg bg-amber-50 p-3 text-sm text-amber-800">Token chỉ hiển thị khi tạo. Lưu vào .env.agent với quyền hạn chế; thay CONTROL_PLANE_HOST bằng địa chỉ thực. Hạn bind lần đầu: {new Date(result.expiresAt).toLocaleString("vi-VN")}.</p><pre className="overflow-auto rounded-xl bg-slate-950 p-4 text-xs leading-6 text-slate-100">{config}</pre><button className="btn btn-secondary" onClick={async () => { try { await navigator.clipboard.writeText(config); setCopied(true); } catch { setCopied(false); } }}>{copied ? "Đã sao chép" : "Sao chép cấu hình"}</button></> : <p className="rounded-xl border border-dashed p-5 text-sm text-slate-500">Tạo enrollment để nhận cấu hình Agent.</p>}
        <p className="text-sm text-slate-600">Trong thư mục backend Go trên server, sau khi cấu hình .env.agent:</p><pre className="overflow-auto rounded-xl bg-slate-950 p-4 text-xs leading-6 text-slate-100">{"CGO_ENABLED=1 go build -o bin/aiwm-agent ./cmd/aiwm-agent\nsudo ./bin/aiwm-agent --check\nsudo ./bin/aiwm-agent"}</pre><p className="text-xs leading-5 text-slate-500">Mỗi host dùng MachineID và state file riêng. Xem docs/TESTING_RUNBOOK.md cho fake GPU, GPU thật và thử mất kết nối.</p>
      </section>
    </div>
    <section className="surface-card p-5"><h2 className="mb-4 font-bold">Enrollment đã tạo</h2>{enrollments.isPending ? <p role="status">Đang tải…</p> : enrollments.isError ? <p role="alert" className="text-red-700">Không tải được enrollment.</p> : <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b text-slate-500"><th className="p-3">Server</th><th>Đơn vị</th><th>Trạng thái</th><th /></tr></thead><tbody>{enrollments.data?.map(e => <tr key={e.id} className="border-b border-slate-100"><td className="p-3 font-semibold">{e.displayName}</td><td>{organizations.data?.find(o => o.id===e.organizationId)?.code ?? organization.code}</td><td><span className={e.revoked ? "badge badge-neutral" : e.machineId ? "badge badge-success" : "badge badge-warning"}>{e.revoked ? "Đã thu hồi" : e.machineId ? "Đã bind machine" : "Chờ Agent (xem hạn dùng)"}</span><p className="mt-1 text-xs text-slate-400">{new Date(e.expiresAt).toLocaleString("vi-VN")}</p></td><td><button className="btn btn-ghost text-red-700" disabled={e.revoked || revoke.isPending} onClick={() => { if (window.confirm("Thu hồi enrollment? Chặn đăng ký lại; không dừng container hoặc thu hồi Agent token hiện có.")) revoke.mutate(e.id); }}>Thu hồi</button></td></tr>)}</tbody></table>{!enrollments.data?.length && <p className="p-5 text-slate-500">Chưa có enrollment.</p>}</div>}{revoke.isError && <p role="alert" className="mt-3 text-red-700">Chưa thu hồi được token.</p>}</section>
    <section className="surface-card p-5"><h2 className="mb-4 font-bold">Agent đã kết nối</h2>{servers.isError ? <p role="alert" className="text-red-700">Không tải được server.</p> : servers.isPending ? <p role="status">Đang tải…</p> : <div className="grid gap-3 md:grid-cols-2">{servers.data?.map(s => <div className="flex items-center justify-between rounded-xl border p-4" key={s.id}><div><p className="font-semibold">{s.name}</p><p className="mt-1 text-xs text-slate-500">{s.gpus.length} GPU · {s.schedulingReason}</p></div><Badge value={s.status} /></div>)}{!servers.data?.length && <p className="text-sm text-slate-500">Chưa có Agent đăng ký trong phạm vi đang xem.</p>}</div>}</section>
  </div>;
}
