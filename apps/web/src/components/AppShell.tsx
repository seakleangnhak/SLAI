"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import { api, type User } from "@/lib/api";
import { cn } from "@/lib/cn";
import { userInitial } from "@/lib/auth";

import { Button } from "./Button";

type AppShellProps = {
  children: React.ReactNode;
  section?: "dashboard" | "admin" | "public";
  user?: User | null;
  logoutRedirect?: string;
};

const navItems = {
  dashboard: [
    { href: "/dashboard", label: "Overview" },
    { href: "/dashboard/api-key", label: "API key" },
    { href: "/dashboard/usage", label: "Usage" },
    { href: "/dashboard/billing", label: "Billing" },
    { href: "/dashboard/settings", label: "Settings" }
  ],
  admin: [
    { href: "/admin", label: "Overview" },
    { href: "/admin/users", label: "Users" },
    { href: "/admin/packages", label: "Packages" },
    { href: "/admin/usage", label: "Usage" },
    { href: "/admin/sync", label: "Sync" },
    { href: "/admin/audit", label: "Audit Logs" }
  ],
  public: [
    { href: "/packages", label: "Packages" },
    { href: "/login", label: "Login" },
    { href: "/signup", label: "Sign up" }
  ]
};

export function AppShell({ children, section = "public", user, logoutRedirect }: AppShellProps) {
  const pathname = usePathname();
  const router = useRouter();
  const items = navItems[section];

  async function logout() {
    await api.auth.logout();
    router.push(logoutRedirect ?? (section === "admin" ? "/admin/login" : "/login"));
    router.refresh();
  }

  return (
    <div className="min-h-screen bg-[#f7f8fb] text-slate-950">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex min-h-16 max-w-7xl flex-col gap-3 px-4 py-3 sm:px-6 md:flex-row md:items-center md:justify-between lg:px-8">
          <Link href="/" className="flex items-center gap-3">
            <span className="grid size-8 place-items-center rounded-md bg-slate-950 text-sm font-semibold text-white">S</span>
            <span className="text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
          </Link>
          <nav className="flex flex-wrap items-center gap-1">
            {items.map((item) => (
              <Link
                href={item.href}
                key={item.label}
                className={cn(
                  "rounded-md px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-950",
                  pathname === item.href && "bg-slate-100 text-slate-950"
                )}
              >
                {item.label}
              </Link>
            ))}
            {section === "dashboard" && user?.role === "ADMIN" ? (
              <Link className="rounded-md px-3 py-2 text-sm font-medium text-cyan-700 hover:bg-cyan-50" href="/admin">
                Admin
              </Link>
            ) : null}
          </nav>
          <div className="flex items-center gap-3">
            {user ? (
              <>
                <div className="hidden items-center gap-2 text-sm text-slate-600 sm:flex">
                  <span className="grid size-7 place-items-center rounded-md bg-slate-100 font-semibold text-slate-800">
                    {userInitial(user)}
                  </span>
                  <span className="max-w-48 truncate">{user.email}</span>
                </div>
                <Button type="button" variant="secondary" onClick={logout}>
                  Logout
                </Button>
              </>
            ) : (
              <Link className="rounded-md bg-slate-950 px-3 py-2 text-sm font-semibold text-white hover:bg-slate-800" href="/login">
                Login
              </Link>
            )}
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">{children}</main>
    </div>
  );
}
