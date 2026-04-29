import { cn } from "@/lib/cn";

type BadgeProps = {
  children: React.ReactNode;
  tone?: "neutral" | "green" | "red" | "yellow" | "cyan";
};

const tones = {
  neutral: "bg-slate-100 text-slate-700",
  green: "bg-emerald-50 text-emerald-700",
  red: "bg-red-50 text-red-700",
  yellow: "bg-amber-50 text-amber-700",
  cyan: "bg-cyan-50 text-cyan-700"
};

export function Badge({ children, tone = "neutral" }: BadgeProps) {
  return <span className={cn("inline-flex rounded-md px-2.5 py-1 text-xs font-semibold", tones[tone])}>{children}</span>;
}

export function statusTone(status?: string): BadgeProps["tone"] {
  if (!status) {
    return "neutral";
  }
  if (["ACTIVE", "billed", "ready"].includes(status)) {
    return "green";
  }
  if (["SUSPENDED", "pending", "duplicate"].includes(status)) {
    return "yellow";
  }
  if (["REVOKED", "failed"].includes(status)) {
    return "red";
  }
  return "neutral";
}
