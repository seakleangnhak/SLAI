"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";

import { api, readableError, type User } from "@/lib/api";

import { AppShell } from "./AppShell";
import { ErrorState } from "./ErrorState";
import { LoadingState } from "./LoadingState";

type DashboardShellProps = {
  children: React.ReactNode;
};

export function DashboardShell({ children }: DashboardShellProps) {
  const router = useRouter();
  const pathname = usePathname();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    api.auth
      .me()
      .then((response) => {
        if (!cancelled) {
          setUser(response.user);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          if (err?.status === 401) {
            router.replace(`/login?next=${encodeURIComponent(pathname)}`);
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
      <AppShell section="dashboard">
        <LoadingState label="Checking session" />
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell section="dashboard">
        <ErrorState message={error} />
      </AppShell>
    );
  }

  if (!user) {
    return null;
  }

  return (
    <AppShell section="dashboard" user={user}>
      {children}
    </AppShell>
  );
}
