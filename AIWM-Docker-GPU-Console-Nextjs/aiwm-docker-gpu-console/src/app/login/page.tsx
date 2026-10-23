"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { identityApi } from "@/lib/api/identity";

export default function LoginPage() {
  const [username,setUsername] = useState("");
  const [password,setPassword] = useState("");
  const client = useQueryClient();
  const login = useMutation({ mutationFn: () => identityApi.login(username.trim(),password), onSuccess: async () => { await client.cancelQueries(); client.clear(); window.location.replace("/"); } });
  return <main className="grid min-h-screen place-items-center bg-slate-50 p-5"><div className="w-full max-w-md overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-xl shadow-slate-200/50">
    <div className="border-t-4 border-red-600 px-8 pb-5 pt-9"><p className="text-xs font-bold uppercase tracking-widest text-red-600">AIWM · GPU Resource Pool</p><h1 className="mt-3 text-3xl font-bold text-slate-900">Đăng nhập nội bộ</h1><p className="mt-2 text-sm leading-6 text-slate-500">Truy cập tài nguyên GPU thuộc đơn vị của bạn.</p></div>
    <form className="space-y-5 px-8 pb-8" onSubmit={e => { e.preventDefault(); login.mutate(); }}>
      <label className="block text-sm font-medium text-slate-700">Tên đăng nhập hoặc email<input className="field-input mt-2" autoComplete="username" required maxLength={200} value={username} onChange={e => setUsername(e.target.value)} /></label>
      <label className="block text-sm font-medium text-slate-700">Mật khẩu<input className="field-input mt-2" type="password" autoComplete="current-password" required maxLength={256} value={password} onChange={e => setPassword(e.target.value)} /></label>
      {login.isError && <p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-700">Không đăng nhập được. Kiểm tra tài khoản, mật khẩu và trạng thái kết nối.</p>}
      <button className="btn btn-primary w-full" disabled={login.isPending}>{login.isPending ? "Đang đăng nhập…" : "Đăng nhập"}</button>
      <p className="text-center text-xs leading-5 text-slate-400">Tài khoản do ADMIN cấp. Liên hệ quản trị viên đơn vị nếu cần hỗ trợ.</p>
    </form>
  </div></main>;
}
