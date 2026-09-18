"use client";

import { useState } from "react";
import type { Organization } from "@/lib/api/identity";

export function OrganizationSelect({ organizations, value, onChange, allowAll = false }: { organizations: Organization[]; value: string; onChange: (id: string) => void; allowAll?: boolean }) {
  const [search,setSearch] = useState("");
  const filtered = organizations.filter(o => o.id === value || `${o.code} ${o.name}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
  return <div className="space-y-2 rounded-xl border border-slate-200 bg-slate-50 p-3">
    <input type="search" className="field-input" aria-label="Tìm đơn vị theo mã hoặc tên" placeholder="Tìm mã hoặc tên đơn vị…" value={search} onChange={e => setSearch(e.target.value)} />
    <select className="field-select" aria-label="Đơn vị" required={!allowAll} value={value} onChange={e => onChange(e.target.value)}>
      <option value="">{allowAll ? "Tất cả đơn vị" : "Chọn đơn vị"}</option>
      {filtered.map(o => <option key={o.id} value={o.id}>{o.code} · {o.name}{o.enabled ? "" : " (Tạm khóa)"}</option>)}
    </select>
    {!filtered.length && <p className="text-xs text-slate-500">Không tìm thấy đơn vị phù hợp.</p>}
  </div>;
}
