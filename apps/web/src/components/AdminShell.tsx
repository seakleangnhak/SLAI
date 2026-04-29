"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { api, readableError, type User } from "@/lib/api";
import { isAdmin } from "@/lib/auth";

import { AppShell } from "./AppShell";
import { Button } from "./Button";
import { Card, CardDescription, CardHeader, CardTitle } from "./Card";
import { ErrorState } from "./ErrorState";
import { LoadingState } from "./LoadingState";

type AdminShellProps = {
  children: React.ReactNode;
};

export function AdminShell({ children }: AdminShellProps) {
  const router = useRouter();
  const pathname = usePathname();
  const [user, setUser] = useState<User | null>(null);
  const [forbiddenUser, setForbiddenUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
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
          setForbiddenUser(response.user);
          return;
        }
        setUser(response.user);
      })
      .catch((err) => {
        if (!cancelled) {
          if (err?.status === 401) {
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
    return (
      <AppShell section="public">
        <LoadingState label="Checking admin access" />
      </AppShell>
    );
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

  return (
    <AppShell section="admin" user={user}>
      {children}
    </AppShell>
  );
}

function AdminForbiddenState({ user }: { user: User }) {
  const router = useRouter();
  const [loggingOut, setLoggingOut] = useState(false);

  async function logout() {
    setLoggingOut(true);
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
