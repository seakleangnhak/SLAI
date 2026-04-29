"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { MetricCard } from "@/components/MetricCard";
import { api, readableError, type SyncStatus, type UsageEvent } from "@/lib/api";
import { formatDateTime, formatUnits } from "@/lib/format";

const links = [
  { href: "/admin/users", label: "Users", description: "Open a user by ID for key actions and top-ups." },
  { href: "/admin/packages", label: "Packages", description: "Create and edit public prepaid packages." },
  { href: "/admin/usage", label: "Usage", description: "Inspect billed and ignored usage events." },
  { href: "/admin/sync", label: "Sync status", description: "Trigger manual sync and inspect worker state." }
];

export default function AdminPage() {
  const [syncStatus, setSyncStatus] = useState<SyncStatus | null>(null);
  const [usage, setUsage] = useState<UsageEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    Promise.all([api.admin.usage.syncStatus(), api.admin.usage.list({ limit: 5 })])
      .then(([status, usageResponse]) => {
        setSyncStatus(status.sync_status);
        setUsage(usageResponse.usage);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  return (
    <AdminShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin console</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Operations</h1>
      </div>

      {loading ? <div className="mt-8"><LoadingState label="Loading admin overview" /></div> : null}
      {error ? <div className="mt-8"><ErrorState message={error} onRetry={load} /></div> : null}

      <section className="mt-8 grid gap-4 md:grid-cols-3">
        <MetricCard label="Worker" value={syncStatus?.worker_enabled ? "Enabled" : "Disabled"} hint={syncStatus?.currently_running ? "Currently running" : "Idle"} />
        <MetricCard label="Last billed" value={formatUnits(syncStatus?.last_result?.billed ?? 0)} hint="Most recent sync result" />
        <MetricCard label="Recent usage" value={formatUnits(usage.length)} hint="Latest admin usage rows loaded" />
      </section>

      <section className="mt-8 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {links.map((item) => (
          <Link key={item.href} href={item.href} className="block rounded-lg border border-slate-200 bg-white p-5 shadow-sm hover:border-cyan-200 hover:bg-cyan-50">
            <h2 className="text-base font-semibold text-slate-950">{item.label}</h2>
            <p className="mt-2 text-sm leading-6 text-slate-500">{item.description}</p>
          </Link>
        ))}
      </section>

      <Card className="mt-8">
        <CardHeader>
          <div>
            <CardTitle>Sync snapshot</CardTitle>
            <CardDescription>Manual sync and worker sync update this in-memory status.</CardDescription>
          </div>
          <Badge tone={syncStatus?.last_error ? "red" : "green"}>{syncStatus?.last_error ? "Error" : "OK"}</Badge>
        </CardHeader>
        <dl className="grid gap-4 text-sm md:grid-cols-3">
          <div><dt className="font-medium text-slate-500">Last success</dt><dd className="mt-1 text-slate-950">{formatDateTime(syncStatus?.last_success_at)}</dd></div>
          <div><dt className="font-medium text-slate-500">Last finished</dt><dd className="mt-1 text-slate-950">{formatDateTime(syncStatus?.last_finished_at)}</dd></div>
          <div><dt className="font-medium text-slate-500">Next run</dt><dd className="mt-1 text-slate-950">{formatDateTime(syncStatus?.next_run_at)}</dd></div>
        </dl>
      </Card>
    </AdminShell>
  );
}
