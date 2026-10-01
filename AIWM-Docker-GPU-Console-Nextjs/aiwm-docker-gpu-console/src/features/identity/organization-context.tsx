"use client";

import { useQuery } from "@tanstack/react-query";
import { usePathname } from "next/navigation";
import { useSession } from "@/features/identity/session";
import { OrganizationSelect } from "@/features/identity/organization-select";
import { identityApi } from "@/lib/api/identity";

export function OrganizationContext() {
  const { user } = useSession();
  const pathname = usePathname();
  const filterable = ["/servers", "/gpus", "/containers", "/workloads", "/queue"].includes(pathname);
  if (user.role !== "ADMIN" || !filterable) return null;
  return <div className="mb-4 flex justify-end"><OrganizationFilter /></div>;
}

export function OrganizationFilter() {
  const { user, organizationFilter, setOrganizationFilter } = useSession();
  const admin = user.role === "ADMIN";
  const organizations = useQuery({ queryKey: ["organizations"], queryFn: identityApi.organizations, enabled: admin });
  if (!admin) return null;
  return <div className="w-full sm:w-56">{organizations.isPending ? <p role="status" className="text-sm text-slate-500">Đang tải đơn vị…</p> : organizations.isError ? <button className="btn btn-secondary" onClick={() => void organizations.refetch()}>Thử tải lại đơn vị</button> : <OrganizationSelect organizations={organizations.data ?? []} value={organizationFilter} onChange={setOrganizationFilter} allowAll compact />}</div>;
}
