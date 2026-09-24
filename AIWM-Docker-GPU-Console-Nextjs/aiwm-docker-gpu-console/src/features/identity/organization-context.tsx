"use client";

import { useQuery } from "@tanstack/react-query";
import { usePathname } from "next/navigation";
import { useSession } from "@/features/identity/session";
import { OrganizationSelect } from "@/features/identity/organization-select";
import { identityApi } from "@/lib/api/identity";

export function OrganizationContext() {
  const { user, organization, organizationFilter, setOrganizationFilter } = useSession();
  const pathname = usePathname();
  const admin = user.role === "ADMIN";
  const organizations = useQuery({ queryKey: ["organizations"], queryFn: identityApi.organizations, enabled: admin });
  // Resource filters do not change the identity used to submit a workload.
  const filterable = ["/", "/servers", "/gpus", "/containers", "/workloads", "/queue"].includes(pathname);
  if (admin && !filterable) return null;
  return <section className="mb-5 flex flex-wrap items-center justify-between gap-3">
    <p className="text-sm font-semibold text-slate-600">{admin ? (organizationFilter ? "Tài nguyên theo đơn vị" : "Tài nguyên toàn hệ thống") : organization.name}</p>
    {admin && <div className="w-full sm:w-72">{organizations.isPending ? <p role="status">Đang tải đơn vị…</p> : organizations.isError ? <button className="btn btn-secondary" onClick={() => void organizations.refetch()}>Thử tải lại đơn vị</button> : <OrganizationSelect organizations={organizations.data ?? []} value={organizationFilter} onChange={setOrganizationFilter} allowAll />}</div>}
  </section>;
}
