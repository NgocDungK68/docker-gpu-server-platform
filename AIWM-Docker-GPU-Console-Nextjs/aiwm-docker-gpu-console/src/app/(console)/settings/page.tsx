"use client";

import { ConnectionStatus } from "@/components/layout/connection-status";
import { Card, CardHeader } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page";
import { useSession } from "@/features/identity/session";
import Link from "next/link";

export default function SettingsPage() {
  const { user } = useSession();
  return <><PageHeader title="Hệ thống" /><Card><CardHeader title="Trạng thái kết nối" action={<ConnectionStatus />} />
    <div className="flex flex-wrap gap-3 p-5">
      <Link className="btn btn-secondary" href="/onboarding">Kết nối máy chủ</Link>
      {user.role === "ADMIN" && <Link className="btn btn-secondary" href="/organizations">Đơn vị và tài khoản</Link>}
    </div>
  </Card></>;
}
