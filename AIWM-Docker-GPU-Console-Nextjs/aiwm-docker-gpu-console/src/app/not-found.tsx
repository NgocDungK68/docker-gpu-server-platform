import Link from "next/link";

export default function NotFound() {
  return <main className="grid min-h-screen place-items-center bg-slate-50 p-6"><div className="max-w-md text-center"><p className="text-sm font-bold uppercase tracking-[0.18em] text-[var(--brand-red-dark)]">404</p><h1 className="mt-3 text-3xl font-bold text-slate-950">Không tìm thấy màn hình</h1><p className="mt-3 text-slate-600">Đường dẫn không tồn tại hoặc tài nguyên đã được đổi.</p><Link href="/" className="btn btn-primary mt-6">Về tổng quan</Link></div></main>;
}
