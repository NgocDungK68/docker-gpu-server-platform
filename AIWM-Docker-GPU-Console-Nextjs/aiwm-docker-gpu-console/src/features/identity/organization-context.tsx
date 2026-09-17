"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useSession } from "@/features/identity/session";
import { OrganizationSelect } from "@/features/identity/organization-select";
import { identityApi } from "@/lib/api/identity";
import { api } from "@/lib/api/api-client";

export function OrganizationContext() {
  const { user,organization,organizationFilter,setOrganizationFilter } = useSession();
  const admin = user.role === "ADMIN";
  const organizations = useQuery({ queryKey: ["organizations"], queryFn: identityApi.organizations, enabled: admin });
  const summary = useQuery({ queryKey: ["summary",organizationFilter], queryFn: () => api.getSummary(organizationFilter) });
  const s = summary.data;
  return <section className="mb-6 rounded-2xl border border-slate-200 border-t-red-500 bg-white p-4 sm:p-5">
    <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
      <div><p className="text-xs font-bold uppercase tracking-wider text-red-600">Đơn vị của tài khoản · {organization.code}</p><h2 className="mt-1 font-bold text-slate-900">{organization.name}</h2><p className="mt-1 text-xs text-slate-500">{user.username} · {user.role}</p>
        {admin && <p className="mt-2 text-xs text-slate-500">Bộ lọc chỉ áp dụng khi xem tài nguyên. Workload mới thuộc {organization.code}.</p>}
        {admin && <Link href="/organizations" className="mt-2 inline-block text-sm font-semibold text-red-700">Quản lý đơn vị và tài khoản →</Link>}
      </div>
      {admin && <div className="w-full lg:w-80">{organizations.isPending ? <p role="status">Đang tải đơn vị…</p> : organizations.isError ? <button className="btn btn-secondary" onClick={() => void organizations.refetch()}>Thử tải lại đơn vị</button> : <OrganizationSelect organizations={organizations.data ?? []} value={organizationFilter} onChange={setOrganizationFilter} allowAll />}</div>}
    </div>
    {s && <div className="mt-4 grid grid-cols-3 gap-2 border-t border-slate-100 pt-4 lg:grid-cols-9">{[
      ["Servers",s.serversTotal],["ONLINE",s.serversOnline],["OFFLINE",s.serversOffline],
      ["GPUs",s.gpusTotal],["FREE",s.gpusFree],["RESERVED",s.gpusReserved],["ALLOCATED",s.gpusAllocated],
      ["LEGACY / UNKNOWN",`${s.gpusOccupiedLegacy} / ${s.gpusOccupiedUnknown}`],["UNHEALTHY",s.gpusUnhealthy],
    ].map(([label,value]) => <div key={label} className="rounded-xl bg-slate-50 p-2.5"><p className="text-[10px] font-semibold text-slate-500">{label}</p><p className="mt-1 text-lg font-bold text-slate-900">{value}</p></div>)}</div>}
    {summary.isError && <p className="mt-3 text-sm text-red-700" role="alert">Chưa tải được số liệu tài nguyên.</p>}
  </section>;
}
