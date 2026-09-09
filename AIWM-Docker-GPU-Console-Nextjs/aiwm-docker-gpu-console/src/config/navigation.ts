import {
  Boxes,
  Container,
  Cpu,
  Gauge,
  ListOrdered,
  Network,
  PlusSquare,
  Server,
  Settings,
  SlidersHorizontal,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

export interface NavigationItem {
  label: string;
  href: string;
  icon: LucideIcon;
  planned?: boolean;
}

interface NavigationGroup {
  label: string;
  items: NavigationItem[];
}

export const navigation: NavigationGroup[] = [
  {
    label: "Vận hành",
    items: [
      { label: "Tổng quan", href: "/", icon: Gauge },
      { label: "Máy chủ", href: "/servers", icon: Server },
      { label: "GPU inventory", href: "/gpus", icon: Cpu },
      { label: "Containers", href: "/containers", icon: Container },
    ],
  },
  {
    label: "Điều phối",
    items: [
      { label: "Workloads", href: "/workloads", icon: Boxes },
      { label: "Tạo workload", href: "/workloads/new", icon: PlusSquare },
      { label: "Hàng đợi", href: "/queue", icon: ListOrdered },
      { label: "Scheduler Lab", href: "/scheduler", icon: SlidersHorizontal },
    ],
  },
  {
    label: "Quản trị",
    items: [
      { label: "Onboard Agent", href: "/onboarding", icon: Network },
      { label: "Cấu hình", href: "/settings", icon: Settings },
    ],
  },
];

export const flatNavigation = navigation.flatMap((group) => group.items);
