"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "@/features/identity/session";
import { OrganizationSelect } from "@/features/identity/organization-select";
import { identityApi, type UserInput } from "@/lib/api/identity";

const emptyOrganization = { code: "", name: "", enabled: true };
const emptyUser: UserInput = { username: "", password: "", role: "ORGANIZATION_USER", organizationId: "", enabled: true };

export default function OrganizationsPage() {
  const { user } = useSession();
  const admin = user.role === "ADMIN";
  const client = useQueryClient();
  const organizations = useQuery({ queryKey: ["organizations"], queryFn: identityApi.organizations, enabled: admin });
  const users = useQuery({ queryKey: ["users"], queryFn: identityApi.users, enabled: admin });
  const [organizationId,setOrganizationId] = useState("");
  const [organization,setOrganization] = useState(emptyOrganization);
  const [userId,setUserId] = useState("");
  const [account,setAccount] = useState(emptyUser);
  const [message,setMessage] = useState("");
  const saveOrganization = useMutation({ mutationFn: () => identityApi.saveOrganization(organization,organizationId || undefined), onSuccess: () => { void client.invalidateQueries({ queryKey: ["organizations"] }); setOrganization(emptyOrganization); setOrganizationId(""); setMessage("Đã lưu đơn vị."); } });
  const saveUser = useMutation({ mutationFn: () => identityApi.saveUser(account,userId || undefined), onSuccess: () => { void client.invalidateQueries({ queryKey: ["users"] }); setAccount(emptyUser); setUserId(""); setMessage("Đã lưu tài khoản và thu hồi các phiên cũ."); } });
  if (!admin) return <p className="rounded-xl bg-red-50 p-5 text-red-700">Chức năng này dành cho ADMIN.</p>;
  return <div className="space-y-6">
    <div><h1 className="text-2xl font-bold text-slate-900">Đơn vị và tài khoản</h1><p className="mt-2 text-sm text-slate-500">Quản lý đơn vị và tài khoản nội bộ.</p></div>
    {message && <p role="status" className="rounded-xl bg-emerald-50 p-3 text-sm text-emerald-800">{message}</p>}
    {(organizations.isError || users.isError) && <p role="alert" className="rounded-xl bg-red-50 p-3 text-red-700">Không tải được dữ liệu. <button onClick={() => { void organizations.refetch(); void users.refetch(); }}>Thử lại</button></p>}
    <section className="grid gap-5 xl:grid-cols-[1.4fr_1fr]">
      <div className="surface-card p-5"><h2 className="mb-4 font-bold">Danh sách đơn vị</h2>{organizations.isPending ? <p role="status">Đang tải…</p> : <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b text-slate-500"><th className="p-3">Mã</th><th>Tên đơn vị</th><th>Trạng thái</th><th /></tr></thead><tbody>{organizations.data?.map(o => <tr key={o.id} className="border-b border-slate-100"><td className="p-3 font-bold text-red-700">{o.code}</td><td>{o.name}</td><td><span className={o.enabled ? "badge badge-success" : "badge badge-neutral"}>{o.enabled ? "Hoạt động" : "Tạm khóa"}</span></td><td><button className="btn btn-ghost" onClick={() => { setOrganizationId(o.id); setOrganization({code:o.code,name:o.name,enabled:o.enabled}); }}>Sửa</button></td></tr>)}</tbody></table>{!organizations.data?.length && <p className="p-5 text-slate-500">Chưa có đơn vị.</p>}</div>}</div>
      <form className="surface-card space-y-4 p-5" onSubmit={e => { e.preventDefault(); if (!organization.enabled && !window.confirm("Tạm khóa đơn vị sẽ chặn đăng nhập và cấp phát mới. Container hiện có tiếp tục chạy. Tiếp tục?")) return; saveOrganization.mutate(); }}>
        <h2 className="font-bold">{organizationId ? "Chỉnh sửa đơn vị" : "Thêm đơn vị"}</h2>
        <label className="block"><span className="field-label">Mã đơn vị</span><input className="field-input" required maxLength={32} value={organization.code} onChange={e => setOrganization({...organization,code:e.target.value})} /></label>
        <label className="block"><span className="field-label">Tên đơn vị</span><input className="field-input" required maxLength={200} value={organization.name} onChange={e => setOrganization({...organization,name:e.target.value})} /></label>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={organization.enabled} onChange={e => setOrganization({...organization,enabled:e.target.checked})} />Hoạt động</label>
        {saveOrganization.isError && <p role="alert" className="text-sm text-red-700">Không lưu được. Kiểm tra mã trùng và quyền truy cập.</p>}
        <div className="flex gap-2"><button className="btn btn-primary" disabled={saveOrganization.isPending}>Lưu đơn vị</button><button className="btn btn-secondary" type="button" onClick={() => { setOrganization(emptyOrganization); setOrganizationId(""); }}>Nhập mới</button></div>
      </form>
    </section>
    <section className="grid gap-5 xl:grid-cols-[1.4fr_1fr]">
      <div className="surface-card p-5"><h2 className="mb-4 font-bold">Tài khoản nội bộ</h2>{users.isPending ? <p role="status">Đang tải…</p> : <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b text-slate-500"><th className="p-3">Tài khoản</th><th>Đơn vị / quyền</th><th>Trạng thái</th><th /></tr></thead><tbody>{users.data?.map(u => <tr key={u.id} className="border-b border-slate-100"><td className="p-3 font-semibold">{u.username}</td><td><p>{organizations.data?.find(o => o.id===u.organizationId)?.code ?? "—"}</p><p className="text-xs text-slate-500">{u.role === "ADMIN" ? "Quản trị viên" : "Người dùng đơn vị"}</p></td><td><span className={u.enabled ? "badge badge-success" : "badge badge-neutral"}>{u.enabled ? "Hoạt động" : "Tạm khóa"}</span></td><td><button className="btn btn-ghost" onClick={() => { setUserId(u.id); setAccount({username:u.username,password:"",role:u.role,organizationId:u.organizationId,enabled:u.enabled}); }}>Sửa</button></td></tr>)}</tbody></table>{!users.data?.length && <p className="p-5 text-slate-500">Chưa có tài khoản.</p>}</div>}</div>
      <form className="surface-card space-y-4 p-5" onSubmit={e => { e.preventDefault(); if (userId && !window.confirm("Lưu thay đổi và thu hồi các phiên đăng nhập cũ của tài khoản?")) return; saveUser.mutate(); }}>
        <h2 className="font-bold">{userId ? "Chỉnh sửa tài khoản" : "Cấp tài khoản"}</h2>
        <label className="block"><span className="field-label">Tên đăng nhập / email</span><input className="field-input" required maxLength={200} value={account.username} onChange={e => setAccount({...account,username:e.target.value})} /></label>
        <label className="block"><span className="field-label">Mật khẩu {userId ? "mới (để trống nếu giữ nguyên)" : "(12–256 ký tự)"}</span><input className="field-input" type="password" autoComplete="new-password" required={!userId} minLength={12} maxLength={256} value={account.password} onChange={e => setAccount({...account,password:e.target.value})} /></label>
        <label className="block"><span className="field-label">Quyền</span><select className="field-select" value={account.role} onChange={e => setAccount({...account,role:e.target.value as UserInput["role"]})}><option value="ORGANIZATION_USER">Người dùng đơn vị</option><option value="ADMIN">Quản trị viên</option></select></label>
        <div><p className="field-label">Đơn vị</p>{userId ? <p className="rounded-lg bg-slate-50 p-3 text-sm">{organizations.data?.find(o => o.id===account.organizationId)?.name}</p> : <OrganizationSelect organizations={(organizations.data ?? []).filter(o => o.enabled)} value={account.organizationId} onChange={id => setAccount({...account,organizationId:id})} />}</div>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={account.enabled} onChange={e => setAccount({...account,enabled:e.target.checked})} />Cho phép đăng nhập</label>
        {saveUser.isError && <p role="alert" className="text-sm text-red-700">Không lưu được. Kiểm tra tài khoản trùng, mật khẩu và đơn vị.</p>}
        <div className="flex gap-2"><button className="btn btn-primary" disabled={saveUser.isPending}>Lưu tài khoản</button><button type="button" className="btn btn-secondary" onClick={() => { setUserId(""); setAccount(emptyUser); }}>Nhập mới</button></div>
      </form>
    </section>
  </div>;
}
