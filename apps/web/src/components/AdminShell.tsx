"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { api, readableError, type SyncStatus, type User } from "@/lib/api";
import { isAdmin, userInitial } from "@/lib/auth";
import { cn } from "@/lib/cn";

import { AppShell } from "./AppShell";
import { Button } from "./Button";
import { Card, CardDescription, CardHeader, CardTitle } from "./Card";
import { ErrorState } from "./ErrorState";

type AdminShellProps = {
  children: React.ReactNode;
};

type AdminNavIconName = "overview" | "users" | "packages" | "payments" | "usage" | "sync" | "audit" | "settings";

const navItems: { href: string; label: string; icon: AdminNavIconName }[] = [
  { href: "/admin", label: "Overview", icon: "overview" },
  { href: "/admin/users", label: "Users", icon: "users" },
  { href: "/admin/packages", label: "Packages", icon: "packages" },
  { href: "/admin/payments", label: "Payments", icon: "payments" },
  { href: "/admin/settings/payments", label: "Payment Settings", icon: "settings" },
  { href: "/admin/usage", label: "Usage", icon: "usage" },
  { href: "/admin/sync", label: "Sync", icon: "sync" },
  { href: "/admin/audit", label: "Audit Logs", icon: "audit" }
];

let cachedAdminUser: User | null = null;
let cachedSyncStatus: SyncStatus | null = null;

export function AdminShell({ children }: AdminShellProps) {
  const router = useRouter();
  const pathname = usePathname();
  const [user, setUser] = useState<User | null>(() => cachedAdminUser);
  const [forbiddenUser, setForbiddenUser] = useState<User | null>(null);
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(() => cachedSyncStatus);
  const [loading, setLoading] = useState(() => !cachedAdminUser);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    api.auth
      .me()
      .then((response) => {
        if (cancelled) {
          return;
        }
        if (!isAdmin(response.user)) {
          cachedAdminUser = null;
          cachedSyncStatus = null;
          setUser(null);
          setForbiddenUser(response.user);
          return;
        }
        cachedAdminUser = response.user;
        setUser(response.user);
        setForbiddenUser(null);
        api.admin.usage
          .syncStatus()
          .then((statusResponse) => {
            if (!cancelled) {
              cachedSyncStatus = statusResponse.sync_status;
              setSyncStatus(statusResponse.sync_status);
            }
          })
          .catch(() => {
            if (!cancelled) {
              cachedSyncStatus = null;
              setSyncStatus(null);
            }
          });
      })
      .catch((err) => {
        if (!cancelled) {
          if (err?.status === 401) {
            cachedAdminUser = null;
            cachedSyncStatus = null;
            setUser(null);
            router.replace("/admin/login?next=" + encodeURIComponent(pathname));
            return;
          }
          setError(readableError(err));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [pathname, router]);

  if (loading) {
    return <AdminLoadingState />;
  }

  if (error) {
    return (
      <AppShell section="public">
        <ErrorState message={error} />
      </AppShell>
    );
  }

  if (forbiddenUser) {
    return <AdminForbiddenState user={forbiddenUser} />;
  }

  if (!user) {
    return null;
  }

  return <AdminFrame user={user} syncStatus={syncStatus}>{children}</AdminFrame>;
}

function AdminFrame({ children, user, syncStatus }: { children: React.ReactNode; user: User; syncStatus: SyncStatus | null }) {
  const router = useRouter();
  const pathname = usePathname();
  const [loggingOut, setLoggingOut] = useState(false);

  async function logout() {
    setLoggingOut(true);
    cachedAdminUser = null;
    cachedSyncStatus = null;
    await api.auth.logout();
    router.push("/admin/login");
    router.refresh();
  }

  return (
    <div className="min-h-screen bg-slate-50 text-slate-950">
      <aside className="fixed left-0 top-0 z-40 hidden h-screen w-64 flex-col border-r border-slate-200 bg-slate-50 lg:flex">
        <div className="border-b border-slate-200 px-6 py-5">
          <Link href="/admin" className="flex items-center gap-3">
            <span className="grid size-9 place-items-center rounded-lg bg-blue-600 text-sm font-bold text-white shadow-sm">S</span>
            <span>
              <span className="block text-base font-semibold leading-5 text-slate-950">SLAI</span>
              <span className="block text-xs text-slate-500">Prepaid AI admin</span>
            </span>
          </Link>
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto px-2 py-4 text-[13px] font-medium tracking-wide">
          {navItems.map((item) => <AdminNavItem item={item} key={item.href} pathname={pathname} />)}
        </nav>

        <div className="border-t border-slate-200 p-4">
          <div className="mb-3 flex items-center gap-3 rounded-lg bg-white px-3 py-2 shadow-sm ring-1 ring-slate-200">
            <span className="grid size-8 place-items-center rounded-md bg-slate-100 text-sm font-semibold text-slate-700">{userInitial(user)}</span>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-slate-900">{user.email}</p>
              <p className="text-xs text-slate-500">Administrator</p>
            </div>
          </div>
          <Button className="w-full justify-center" type="button" variant="secondary" onClick={logout} disabled={loggingOut}>
            {loggingOut ? "Signing out" : "Sign out"}
          </Button>
        </div>
      </aside>

      <div className="lg:pl-64">
        <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/80 backdrop-blur-md">
          <div className="flex min-h-14 flex-col gap-3 px-4 py-3 sm:px-6 xl:flex-row xl:items-center xl:justify-between">
            <div className="flex items-center gap-3 lg:hidden">
              <Link href="/admin" className="flex items-center gap-2">
                <span className="grid size-8 place-items-center rounded-md bg-blue-600 text-sm font-bold text-white">S</span>
                <span className="text-sm font-semibold tracking-[0.16em] text-slate-900">SLAI ADMIN</span>
              </Link>
            </div>
            <div className="relative w-full max-w-xl">
              <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-slate-400">/</span>
              <input
                className="h-9 w-full rounded-md border border-slate-200 bg-slate-100 pl-9 pr-20 text-sm text-slate-900 outline-none ring-blue-600/20 transition placeholder:text-slate-500 focus:border-blue-600 focus:bg-white focus:ring-4"
                placeholder="Search users, keys, requests..."
                type="search"
              />
              <span className="pointer-events-none absolute right-3 top-1/2 hidden -translate-y-1/2 rounded border border-slate-300 px-1.5 py-0.5 font-mono text-[10px] text-slate-500 sm:inline">⌘K</span>
            </div>
            <div className="flex items-center justify-between gap-4 xl:justify-end">
              <Link className="rounded-md px-2 py-1.5 text-sm font-medium text-slate-500 hover:bg-slate-100 hover:text-slate-950" href="/">
                Docs
              </Link>
              <SyncDot status={syncStatus} />
              <div className="grid size-8 place-items-center rounded-full border border-slate-200 bg-white text-sm font-semibold text-slate-700 shadow-sm">
                {userInitial(user)}
              </div>
            </div>
          </div>
          <nav className="flex gap-1 overflow-x-auto border-t border-slate-200 px-3 py-2 text-sm font-medium lg:hidden">
            {navItems.map((item) => <MobileAdminNavItem item={item} key={item.href} pathname={pathname} />)}
          </nav>
        </header>

        <main className="px-4 py-6 sm:px-6 lg:px-8">{children}</main>
      </div>
    </div>
  );
}

function AdminNavItem({ item, pathname }: { item: (typeof navItems)[number]; pathname: string }) {
  const active = isActiveAdminPath(pathname, item.href);
  return (
    <Link
      href={item.href}
      className={cn(
        "group flex items-center gap-3 rounded-md border-l-2 px-4 py-2.5 transition-colors",
        active
          ? "border-blue-600 bg-white text-blue-600 shadow-sm ring-1 ring-slate-200/70"
          : "border-transparent text-slate-600 hover:bg-slate-200/60 hover:text-slate-950"
      )}
    >
      <span className={cn("grid size-6 place-items-center rounded", active ? "bg-blue-50 text-blue-600" : "bg-slate-100 text-slate-500")}>
        <AdminNavIcon name={item.icon} />
      </span>
      {item.label}
    </Link>
  );
}

function AdminNavIcon({ name }: { name: AdminNavIconName }) {
  const paths: Record<AdminNavIconName, React.ReactNode> = {
    overview: (
      <>
        <path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h1A2.5 2.5 0 0 1 10 6.5v1A2.5 2.5 0 0 1 7.5 10h-1A2.5 2.5 0 0 1 4 7.5v-1Z" />
        <path d="M14 4h3.5A2.5 2.5 0 0 1 20 6.5v1a2.5 2.5 0 0 1-2.5 2.5H14V4Z" />
        <path d="M4 14h6v6H6.5A2.5 2.5 0 0 1 4 17.5V14Z" />
        <path d="M14 14h3.5a2.5 2.5 0 0 1 2.5 2.5v1a2.5 2.5 0 0 1-2.5 2.5H14v-6Z" />
      </>
    ),
    users: (
      <>
        <path d="M16 20v-1.5A3.5 3.5 0 0 0 12.5 15h-5A3.5 3.5 0 0 0 4 18.5V20" />
        <path d="M10 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" />
        <path d="M20 20v-1.5a3.5 3.5 0 0 0-2.5-3.35" />
        <path d="M15.5 4.2a3.5 3.5 0 0 1 0 6.6" />
      </>
    ),
    packages: (
      <>
        <path d="M4 8.5 12 4l8 4.5-8 4.5-8-4.5Z" />
        <path d="m4 8.5 8 4.5v7l-8-4.5v-7Z" />
        <path d="m20 8.5-8 4.5v7l8-4.5v-7Z" />
        <path d="M8 6.25 16 10.75" />
      </>
    ),
    payments: (
      <>
        <path d="M4 7h16" />
        <path d="M6 7V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v2" />
        <path d="M6 7v12a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V7" />
        <path d="M9 12h6" />
        <path d="M9 16h4" />
      </>
    ),
    settings: (
      <>
        <path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" />
        <path d="M19.4 15a1.8 1.8 0 0 0 .36 1.98l.05.05a2 2 0 0 1-2.83 2.83l-.05-.05a1.8 1.8 0 0 0-1.98-.36 1.8 1.8 0 0 0-1.1 1.66V21a2 2 0 0 1-4 0v-.09a1.8 1.8 0 0 0-1.1-1.66 1.8 1.8 0 0 0-1.98.36l-.05.05a2 2 0 0 1-2.83-2.83l.05-.05A1.8 1.8 0 0 0 4.6 15a1.8 1.8 0 0 0-1.66-1.1H3a2 2 0 0 1 0-4h.09A1.8 1.8 0 0 0 4.75 8.8a1.8 1.8 0 0 0-.36-1.98l-.05-.05A2 2 0 1 1 7.17 3.94l.05.05a1.8 1.8 0 0 0 1.98.36h.01A1.8 1.8 0 0 0 10.3 2.7V2a2 2 0 0 1 4 0v.09a1.8 1.8 0 0 0 1.1 1.66 1.8 1.8 0 0 0 1.98-.36l.05-.05a2 2 0 1 1 2.83 2.83l-.05.05a1.8 1.8 0 0 0-.36 1.98v.01a1.8 1.8 0 0 0 1.66 1.09H21a2 2 0 0 1 0 4h-.09A1.8 1.8 0 0 0 19.4 15Z" />
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
    sync: (
      <>
        <path d="M20 7h-5a5 5 0 0 0-8.66-2.5" />
        <path d="m17 4 3 3-3 3" />
        <path d="M4 17h5a5 5 0 0 0 8.66 2.5" />
        <path d="m7 20-3-3 3-3" />
      </>
    ),
    audit: (
      <>
        <path d="M7 4h10a2 2 0 0 1 2 2v14l-3-2-3 2-3-2-3 2-3-2V6a2 2 0 0 1 2-2Z" />
        <path d="M8 9h8" />
        <path d="M8 13h6" />
      </>
    )
  };

  return (
    <svg aria-hidden="true" className="size-4" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" viewBox="0 0 24 24">
      {paths[name]}
    </svg>
  );
}

function MobileAdminNavItem({ item, pathname }: { item: (typeof navItems)[number]; pathname: string }) {
  const active = isActiveAdminPath(pathname, item.href);
  return (
    <Link className={cn("whitespace-nowrap rounded-md px-3 py-1.5", active ? "bg-blue-50 text-blue-700" : "text-slate-600")} href={item.href}>
      {item.label}
    </Link>
  );
}

function isActiveAdminPath(pathname: string, href: string) {
  if (href === "/admin") {
    return pathname === href;
  }
  return pathname === href || pathname.startsWith(href + "/");
}

function SyncDot({ status }: { status: SyncStatus | null }) {
  const tone = status?.last_error ? "bg-red-500" : status?.currently_running ? "bg-blue-500" : status?.worker_enabled ? "bg-emerald-500" : "bg-slate-300";
  const label = status?.last_error ? "Sync issue" : status?.currently_running ? "Sync running" : status?.worker_enabled ? "Sync OK" : "Sync off";

  return (
    <div className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium text-slate-600">
      <span className={cn("size-2 rounded-full", tone, status?.currently_running && "animate-pulse")} />
      <span>{label}</span>
    </div>
  );
}

function AdminLoadingState() {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-950">
      <aside className="fixed left-0 top-0 z-40 hidden h-screen w-64 flex-col border-r border-slate-200 bg-slate-50 lg:flex">
        <div className="border-b border-slate-200 px-6 py-5">
          <div className="flex items-center gap-3">
            <span className="grid size-9 place-items-center rounded-lg bg-blue-600 text-sm font-bold text-white shadow-sm">S</span>
            <span>
              <span className="block h-4 w-16 rounded bg-slate-200" />
              <span className="mt-1 block h-3 w-28 rounded bg-slate-100" />
            </span>
          </div>
        </div>
        <div className="space-y-2 px-3 py-4">
          {navItems.map((item) => (
            <div className="flex items-center gap-3 rounded-md px-3 py-2.5" key={item.href}>
              <span className="size-6 rounded bg-slate-200" />
              <span className="h-3 w-24 rounded bg-slate-200" />
            </div>
          ))}
        </div>
      </aside>

      <div className="lg:pl-64">
        <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/80 backdrop-blur-md">
          <div className="flex min-h-14 items-center justify-between gap-4 px-4 py-3 sm:px-6">
            <div className="h-9 w-full max-w-xl rounded-md border border-slate-200 bg-slate-100" />
            <div className="hidden items-center gap-3 sm:flex">
              <span className="h-3 w-20 rounded bg-slate-200" />
              <span className="size-8 rounded-full border border-slate-200 bg-white shadow-sm" />
            </div>
          </div>
        </header>

        <main className="px-4 py-6 sm:px-6 lg:px-8">
          <div className="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
            <div className="flex items-center gap-3 text-sm font-medium text-slate-600">
              <span className="size-2 rounded-full bg-blue-600" />
              Checking admin access
            </div>
          </div>
        </main>
      </div>
    </div>
  );
}

function AdminForbiddenState({ user }: { user: User }) {
  const router = useRouter();
  const [loggingOut, setLoggingOut] = useState(false);

  async function logout() {
    setLoggingOut(true);
    cachedAdminUser = null;
    cachedSyncStatus = null;
    await api.auth.logout();
    router.push("/admin/login");
    router.refresh();
  }

  return (
    <AppShell section="public" user={user} logoutRedirect="/admin/login">
      <Card className="mx-auto max-w-2xl">
        <CardHeader>
          <div>
            <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin Console</p>
            <CardTitle className="mt-2 text-2xl">Admin access required</CardTitle>
            <CardDescription>
              Your current account does not have permission to access the SLAI admin console.
            </CardDescription>
          </div>
        </CardHeader>
        <div className="flex flex-wrap gap-3">
          <Link className="inline-flex min-h-10 items-center justify-center rounded-md bg-slate-950 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800" href="/dashboard">
            Go to Dashboard
          </Link>
          <Button type="button" variant="secondary" onClick={logout} disabled={loggingOut}>
            {loggingOut ? "Logging out" : "Log out"}
          </Button>
        </div>
      </Card>
    </AppShell>
  );
}
