"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { api, readableError, type User } from "@/lib/api";
import { isAdmin } from "@/lib/auth";

import { AppShell } from "./AppShell";
import { ErrorState } from "./ErrorState";
import { LoadingState } from "./LoadingState";

type AdminShellProps = {
  children: React.ReactNode;
};

export function AdminShell({ children }: AdminShellProps) {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
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
          router.replace("/dashboard");
          return;
        }
        setUser(response.user);
      })
      .catch((err) => {
        if (!cancelled) {
          if (err?.status === 401) {
            router.replace("/login?next=/admin");
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
  }, [router]);

  if (loading) {
    return (
      <AppShell section="admin">
        <LoadingState label="Checking admin access" />
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell section="admin">
        <ErrorState message={error} />
      </AppShell>
    );
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
