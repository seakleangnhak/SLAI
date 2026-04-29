"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type User } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

export default function SettingsPage() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    api.auth
      .me()
      .then((response) => setUser(response.user))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  async function logout() {
    await api.auth.logout();
    router.push("/login");
    router.refresh();
  }

  useEffect(load, []);

  return (
    <DashboardShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Settings</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Account</h1>
      </div>

      <div className="mt-8 max-w-2xl">
        {loading ? <LoadingState label="Loading account" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
        {user ? (
          <Card>
            <CardHeader>
              <div>
                <CardTitle>{user.email}</CardTitle>
                <CardDescription>Session-backed account information.</CardDescription>
              </div>
              <Badge tone={statusTone(user.status)}>{user.status}</Badge>
            </CardHeader>
            <dl className="grid gap-4 text-sm sm:grid-cols-2">
              <div><dt className="font-medium text-slate-500">Role</dt><dd className="mt-1 text-slate-950">{user.role}</dd></div>
              <div><dt className="font-medium text-slate-500">Balance policy</dt><dd className="mt-1 text-slate-950">{user.balancePolicy}</dd></div>
              <div><dt className="font-medium text-slate-500">Created</dt><dd className="mt-1 text-slate-950">{formatDateTime(user.createdAt)}</dd></div>
              <div><dt className="font-medium text-slate-500">Updated</dt><dd className="mt-1 text-slate-950">{formatDateTime(user.updatedAt)}</dd></div>
            </dl>
            <Button className="mt-6" variant="secondary" type="button" onClick={logout}>Logout</Button>
          </Card>
        ) : null}
      </div>
    </DashboardShell>
  );
}
