import type { ButtonHTMLAttributes } from "react";
import { cn } from "@/lib/utils/cn";

type Variant = "primary" | "secondary" | "ghost" | "danger";

export function Button({ className, variant = "primary", type = "button", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return <button type={type} className={cn("btn", `btn-${variant}`, className)} {...props} />;
}
