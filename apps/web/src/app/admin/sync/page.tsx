"use client";

import { useEffect, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { MetricCard } from "@/components/MetricCard";
import { api, readableError, type SyncStatus } from "@/lib/api";
import { formatDateTime, formatUnits } from "@/lib/format";

export default function AdminSyncPage() {
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    api.admin.usage
      .syncStatus()
      .then((response) => setStatus(response.sync_status))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  async function runSync() {
    setSyncing(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.admin.usage.sync();
      setNotice(`Sync completed: ${response.sync.billed} billed, ${response.sync.duplicate} duplicate, ${response.sync.ignored} ignored.`);
      await api.admin.usage.syncStatus().then((statusResponse) => setStatus(statusResponse.sync_status));
    } catch (err) {
      setError(readableError(err));
    } finally {
      setSyncing(false);
    }
  }

  useEffect(load, []);

  const result = status?.last_result;

  return (
    <AdminShell>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Usage sync</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Sync status</h1>
        </div>
        <Button type="button" disabled={syncing} onClick={runSync}>{syncing ? "Syncing" : "Run manual sync"}</Button>
      </div>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading sync status" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
        {notice ? <p className="rounded-md bg-cyan-50 px-3 py-2 text-sm text-cyan-800">{notice}</p> : null}
      </div>

      {status ? (
        <>
          <section className="mt-8 grid gap-4 md:grid-cols-3 xl:grid-cols-6">
            <MetricCard label="Fetched" value={formatUnits(result?.fetched ?? 0)} hint="Last run" />
            <MetricCard label="Billed" value={formatUnits(result?.billed ?? 0)} hint="Debited" />
            <MetricCard label="Duplicate" value={formatUnits(result?.duplicate ?? 0)} hint="Skipped" />
            <MetricCard label="Ignored" value={formatUnits(result?.ignored ?? 0)} hint="No matching key" />
            <MetricCard label="Failed" value={formatUnits(result?.failed ?? 0)} hint="Errors" />
            <MetricCard label="Suspended" value={formatUnits(result?.suspended_keys ?? 0)} hint="Keys" />
          </section>

          <Card className="mt-8">
            <CardHeader>
              <div>
                <CardTitle>Worker state</CardTitle>
                <CardDescription>Stored in memory for the running API process.</CardDescription>
              </div>
              <div className="flex gap-2">
                <Badge tone={status.worker_enabled ? "green" : "neutral"}>{status.worker_enabled ? "Worker enabled" : "Worker disabled"}</Badge>
                <Badge tone={status.currently_running ? "yellow" : "neutral"}>{status.currently_running ? "Running" : "Idle"}</Badge>
              </div>
            </CardHeader>
            <dl className="grid gap-4 text-sm md:grid-cols-3">
              <div><dt className="font-medium text-slate-500">Last started</dt><dd className="mt-1 text-slate-950">{formatDateTime(status.last_started_at)}</dd></div>
              <div><dt className="font-medium text-slate-500">Last finished</dt><dd className="mt-1 text-slate-950">{formatDateTime(status.last_finished_at)}</dd></div>
              <div><dt className="font-medium text-slate-500">Last success</dt><dd className="mt-1 text-slate-950">{formatDateTime(status.last_success_at)}</dd></div>
              <div><dt className="font-medium text-slate-500">Next run</dt><dd className="mt-1 text-slate-950">{formatDateTime(status.next_run_at)}</dd></div>
              <div className="md:col-span-2"><dt className="font-medium text-slate-500">Last error</dt><dd className="mt-1 text-slate-950">{status.last_error ?? "-"}</dd></div>
            </dl>
          </Card>
        </>
      ) : null}
    </AdminShell>
  );
}
