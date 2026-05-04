import type { TableHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

export function Table({ className, ...props }: TableHTMLAttributes<HTMLTableElement>) {
  return (
    <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900/90">
      <table className={cn("min-w-full divide-y divide-slate-200 text-sm dark:divide-slate-800", className)} {...props} />
    </div>
  );
}

export function Th({ children }: { children: React.ReactNode }) {
  return <th className="whitespace-nowrap px-5 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">{children}</th>;
}

export function Td({ children, className }: { children: React.ReactNode; className?: string }) {
  return <td className={cn("whitespace-nowrap px-5 py-4 text-slate-700 dark:text-slate-300", className)}>{children}</td>;
}
