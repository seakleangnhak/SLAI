"use client";

import Link from "next/link";
import { useState } from "react";
import { usePathname, useRouter } from "next/navigation";

import { api, type User } from "@/lib/api";
import { cn } from "@/lib/cn";
import { isAdmin, userInitial } from "@/lib/auth";

import { Button } from "./Button";
import { ThemeToggle } from "./ThemeToggle";

type AppShellProps = {
  children: React.ReactNode;
  section?: "dashboard" | "admin" | "public";
  user?: User | null;
  logoutRedirect?: string;
};

type DashboardNavIconName = "overview" | "key" | "usage" | "billing" | "settings" | "admin";

const navItems = {
  dashboard: [
    { href: "/dashboard", label: "Overview", icon: "overview" },
    { href: "/dashboard/api-key", label: "API key", icon: "key" },
    { href: "/dashboard/usage", label: "Usage", icon: "usage" },
    { href: "/dashboard/billing", label: "Billing", icon: "billing" },
    { href: "/dashboard/settings", label: "Settings", icon: "settings" }
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
    { href: "/#how-it-works", label: "How it works" },
    { href: "/dashboard", label: "Dashboard" }
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

  if (section === "dashboard") {
    return (
      <DashboardFrame user={user} pathname={pathname} onLogout={logout}>
        {children}
      </DashboardFrame>
    );
  }

  return (
    <div className="min-h-screen bg-[#f8fafc] text-slate-950">
      <header className="sticky top-0 z-40 border-b border-slate-200 bg-white/85 backdrop-blur-md">
        <div className="mx-auto flex min-h-16 max-w-7xl flex-col gap-3 px-4 py-3 sm:px-6 md:flex-row md:items-center md:justify-between lg:px-8">
          <Link href="/" className="flex items-center gap-3">
            <span className="grid size-9 place-items-center rounded-xl bg-slate-950 text-sm font-bold text-white shadow-sm">S</span>
            <span>
              <span className="block text-sm font-semibold text-slate-950">SLAI</span>
              <span className="block text-xs text-slate-500">Prepaid AI credits</span>
            </span>
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
              <>
                <Link className="rounded-md px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-100 hover:text-slate-950" href="/login">
                  Sign in
                </Link>
                <Link className="rounded-md bg-slate-950 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-slate-800" href="/signup">
                  Create account
                </Link>
              </>
            )}
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">{children}</main>
      {section === "public" ? (
        <footer className="border-t border-slate-200 bg-white/80">
          <div className="mx-auto flex max-w-7xl flex-col gap-4 px-4 py-8 text-sm text-slate-500 sm:px-6 md:flex-row md:items-center md:justify-between lg:px-8">
            <div>
              <p className="font-semibold text-slate-950">SLAI</p>
              <p className="mt-1">Prepaid AI API credits for developers.</p>
            </div>
            <div className="flex flex-wrap gap-3">
              <Link className="hover:text-slate-950" href="/packages">Packages</Link>
              {user ? (
                <>
                  <Link className="hover:text-slate-950" href="/dashboard">Dashboard</Link>
                  <Link className="hover:text-slate-950" href="/dashboard/billing">Billing</Link>
                </>
              ) : (
                <>
                  <Link className="hover:text-slate-950" href="/login">Sign in</Link>
                  <Link className="hover:text-slate-950" href="/signup">Create account</Link>
                </>
              )}
            </div>
          </div>
        </footer>
      ) : null}
    </div>
  );
}

function DashboardFrame({
  children,
  user,
  pathname,
  onLogout
}: {
  children: React.ReactNode;
  user?: User | null;
  pathname: string;
  onLogout: () => Promise<void>;
}) {
  const [loggingOut, setLoggingOut] = useState(false);
  const dashboardItems = navItems.dashboard;
  const showAdminLink = isAdmin(user);

  async function logout() {
    setLoggingOut(true);
    await onLogout();
  }

  return (
    <div className="min-h-screen bg-slate-50 text-slate-950 dark:bg-slate-950 dark:text-slate-100">
      <aside className="fixed left-0 top-0 z-40 hidden h-screen w-64 flex-col border-r border-slate-200 bg-slate-50 dark:border-slate-800 dark:bg-slate-950 lg:flex">
        <div className="border-b border-slate-200 px-6 py-5">
          <Link href="/dashboard" className="flex items-center gap-3">
            <span className="grid size-9 place-items-center rounded-lg bg-slate-950 text-sm font-bold text-white shadow-sm">S</span>
            <span>
              <span className="block text-base font-semibold leading-5 text-slate-950">SLAI</span>
              <span className="block text-xs text-slate-500">Developer console</span>
            </span>
          </Link>
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto px-2 py-4 text-[13px] font-medium tracking-wide">
          {dashboardItems.map((item) => <DashboardNavItem item={item} key={item.href} pathname={pathname} />)}
          {showAdminLink ? <DashboardNavItem item={{ href: "/admin", label: "Admin", icon: "admin" }} pathname={pathname} /> : null}
        </nav>

        <div className="border-t border-slate-200 p-4">
          {user ? (
            <>
              <div className="mb-3 flex items-center gap-3 rounded-lg bg-white px-3 py-2 shadow-sm ring-1 ring-slate-200">
                <span className="grid size-8 place-items-center rounded-md bg-slate-100 text-sm font-semibold text-slate-700">{userInitial(user)}</span>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-slate-900">{user.email}</p>
                  <p className="text-xs text-slate-500">Developer account</p>
                </div>
              </div>
              <ThemeToggle className="mb-2 w-full" />
              <Button className="w-full justify-center" type="button" variant="secondary" onClick={logout} disabled={loggingOut}>
                {loggingOut ? "Signing out" : "Sign out"}
              </Button>
            </>
          ) : (
            <div className="rounded-lg border border-slate-200 bg-white p-3 text-sm text-slate-500 shadow-sm">Checking session</div>
          )}
        </div>
      </aside>

      <div className="lg:pl-64">
        <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/80 backdrop-blur-md dark:border-slate-800 dark:bg-slate-950/85">
          <div className="flex min-h-14 flex-col gap-3 px-4 py-3 sm:px-6 xl:flex-row xl:items-center xl:justify-between">
            <div className="flex items-center justify-between gap-3 lg:hidden">
              <Link href="/dashboard" className="flex items-center gap-2">
                <span className="grid size-8 place-items-center rounded-md bg-slate-950 text-sm font-bold text-white">S</span>
                <span className="text-sm font-semibold tracking-[0.16em] text-slate-900">SLAI</span>
              </Link>
              <div className="flex items-center gap-2">
                <ThemeToggle compact />
                {user ? (
                  <div className="grid size-8 place-items-center rounded-full border border-slate-200 bg-white text-sm font-semibold text-slate-700 shadow-sm dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
                    {userInitial(user)}
                  </div>
                ) : null}
              </div>
            </div>
            <div className="relative w-full max-w-xl">
              <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-slate-400">/</span>
              <input
                className="h-9 w-full rounded-md border border-slate-200 bg-slate-100 pl-9 pr-4 text-sm text-slate-900 outline-none ring-blue-600/20 transition placeholder:text-slate-500 focus:border-blue-600 focus:bg-white focus:ring-4"
                placeholder="Search usage, ledger, keys..."
                type="search"
              />
            </div>
            <div className="hidden items-center justify-end gap-4 xl:flex">
              <ThemeToggle />
              {showAdminLink ? (
                <Link className="rounded-md px-2 py-1.5 text-sm font-medium text-blue-700 hover:bg-blue-50 dark:hover:bg-blue-950/40" href="/admin">
                  Admin
                </Link>
              ) : null}
              {user ? (
                <div className="grid size-8 place-items-center rounded-full border border-slate-200 bg-white text-sm font-semibold text-slate-700 shadow-sm">
                  {userInitial(user)}
                </div>
              ) : null}
            </div>
          </div>
          <nav className="flex gap-1 overflow-x-auto border-t border-slate-200 px-3 py-2 text-sm font-medium lg:hidden">
            {dashboardItems.map((item) => <MobileDashboardNavItem item={item} key={item.href} pathname={pathname} />)}
            {showAdminLink ? <MobileDashboardNavItem item={{ href: "/admin", label: "Admin", icon: "admin" }} pathname={pathname} /> : null}
          </nav>
        </header>

        <main className="px-4 py-6 sm:px-6 lg:px-8">{children}</main>
      </div>
    </div>
  );
}

function DashboardNavItem({ item, pathname }: { item: (typeof navItems.dashboard)[number] | { href: string; label: string; icon: DashboardNavIconName }; pathname: string }) {
  const active = isActiveDashboardPath(pathname, item.href);
  return (
    <Link
      href={item.href}
      className={cn(
        "group flex items-center gap-3 rounded-md border-l-2 px-4 py-2.5 transition-colors",
        active
          ? "border-blue-600 bg-white text-blue-600 shadow-sm ring-1 ring-slate-200/70 dark:bg-slate-900 dark:text-blue-300 dark:ring-slate-800/80"
          : "border-transparent text-slate-600 hover:bg-slate-200/60 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-slate-800/80 dark:hover:text-slate-100"
      )}
    >
      <span className={cn("grid size-6 place-items-center rounded", active ? "bg-blue-50 text-blue-600 dark:bg-blue-950/50 dark:text-blue-300" : "bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400")}>
        <DashboardNavIcon name={item.icon as DashboardNavIconName} />
      </span>
      {item.label}
    </Link>
  );
}

function MobileDashboardNavItem({ item, pathname }: { item: (typeof navItems.dashboard)[number] | { href: string; label: string; icon: DashboardNavIconName }; pathname: string }) {
  const active = isActiveDashboardPath(pathname, item.href);
  return (
    <Link className={cn("whitespace-nowrap rounded-md px-3 py-1.5", active ? "bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300" : "text-slate-600 dark:text-slate-400")} href={item.href}>
      {item.label}
    </Link>
  );
}

function isActiveDashboardPath(pathname: string, href: string) {
  if (href === "/dashboard") {
    return pathname === href;
  }
  return pathname === href || pathname.startsWith(href + "/");
}

function DashboardNavIcon({ name }: { name: DashboardNavIconName }) {
  const paths: Record<DashboardNavIconName, React.ReactNode> = {
    overview: (
      <>
        <path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h1A2.5 2.5 0 0 1 10 6.5v1A2.5 2.5 0 0 1 7.5 10h-1A2.5 2.5 0 0 1 4 7.5v-1Z" />
        <path d="M14 4h3.5A2.5 2.5 0 0 1 20 6.5v1a2.5 2.5 0 0 1-2.5 2.5H14V4Z" />
        <path d="M4 14h6v6H6.5A2.5 2.5 0 0 1 4 17.5V14Z" />
        <path d="M14 14h3.5a2.5 2.5 0 0 1 2.5 2.5v1a2.5 2.5 0 0 1-2.5 2.5H14v-6Z" />
      </>
    ),
    key: (
      <>
        <path d="M15.5 7.5a4 4 0 1 1-2.3 7.28L8.5 19.5H6v-2.5H3.5v-2.5l4.72-4.7A4 4 0 0 1 15.5 7.5Z" />
        <path d="M16.5 8.5h.01" />
      </>
    ),
    usage: (
      <>
        <path d="M4 19h16" />
        <path d="M7 16V9" />
        <path d="M12 16V5" />
        <path d="M17 16v-4" />
      </>
    ),
    billing: (
      <>
        <path d="M5 7h14a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2Z" />
        <path d="M3 11h18" />
        <path d="M7 15h3" />
      </>
    ),
    settings: (
      <>
        <path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" />
        <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.04.04a2 2 0 0 1-2.83 2.83l-.04-.04A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6l-.06.08a2 2 0 0 1-3.88 0L10 20a1.7 1.7 0 0 0-1-.6 1.7 1.7 0 0 0-1.88.34l-.04.04a2 2 0 1 1-2.83-2.83l.04-.04A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1l-.08-.06a2 2 0 0 1 0-3.88L4 10a1.7 1.7 0 0 0 .6-1 1.7 1.7 0 0 0-.34-1.88l-.04-.04a2 2 0 0 1 2.83-2.83l.04.04A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6l.06-.08a2 2 0 0 1 3.88 0L14 4a1.7 1.7 0 0 0 1 .6 1.7 1.7 0 0 0 1.88-.34l.04-.04a2 2 0 1 1 2.83 2.83l-.04.04A1.7 1.7 0 0 0 19.4 9c.1.38.32.72.6 1l.08.06a2 2 0 0 1 0 3.88L20 14c-.28.28-.5.62-.6 1Z" />
      </>
    ),
    admin: (
      <>
        <path d="M12 3 19 6v5c0 4.4-2.9 8.4-7 10-4.1-1.6-7-5.6-7-10V6l7-3Z" />
        <path d="M9.5 12.5 11 14l3.5-4" />
      </>
    )
  };

  return (
    <svg aria-hidden="true" className="size-4" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" viewBox="0 0 24 24">
      {paths[name]}
    </svg>
  );
}
