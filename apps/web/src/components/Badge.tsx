import { cn } from "@/lib/cn";

type BadgeProps = {
  children: React.ReactNode;
  tone?: "neutral" | "green" | "red" | "yellow" | "cyan" | "blue" | "purple";
  dot?: boolean;
};

const tones = {
  neutral: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300",
  green: "bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300",
  red: "bg-red-50 text-red-700 dark:bg-red-950/50 dark:text-red-300",
  yellow: "bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300",
  cyan: "bg-cyan-50 text-cyan-700 dark:bg-cyan-950/50 dark:text-cyan-300",
  blue: "bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300",
  purple: "bg-violet-50 text-violet-700 dark:bg-violet-950/50 dark:text-violet-300"
};

const dots = {
  neutral: "bg-slate-400",
  green: "bg-emerald-500",
  red: "bg-red-500",
  yellow: "bg-amber-500",
  cyan: "bg-cyan-500",
  blue: "bg-blue-500",
  purple: "bg-violet-500"
};

export function Badge({ children, tone = "neutral", dot = false }: BadgeProps) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold", tones[tone])}>
      {dot ? <span className={cn("size-1.5 rounded-full", dots[tone])} /> : null}
      {children}
    </span>
  );
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
  if (["ADMIN"].includes(status)) {
    return "purple";
  }
  if (["USER"].includes(status)) {
    return "blue";
  }
  return "neutral";
}
