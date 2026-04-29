"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type SyncResult, type SyncStatus } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatDateTime, formatUnits } from "@/lib/format";

const emptyResult: SyncResult = {
  fetched: 0,
  billed: 0,
  duplicate: 0,
  ignored: 0,
  failed: 0,
  suspended_keys: 0
};

type BannerState = {
  tone: "green" | "yellow" | "red" | "blue" | "neutral";
  label: string;
  title: string;
  message: string;
};

export default function AdminSyncPage() {
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const result = status?.last_result ?? emptyResult;
  const banner = useMemo(() => getBannerState(status), [status]);
  const duration = useMemo(() => getLastDuration(status), [status]);
  const troubleshooting = getTroubleshooting(status?.last_error);
  const manualSyncDisabled = syncing || Boolean(status?.currently_running);

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
      setNotice(syncSummaryText(response.sync));
      await api.admin.usage.syncStatus().then((statusResponse) => setStatus(statusResponse.sync_status));
    } catch (err) {
      setError(readableError(err));
    } finally {
      setSyncing(false);
    }
  }

  useEffect(load, []);

  return (
    <AdminShell>
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">Usage sync</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-0.02em] text-slate-950">Sync status</h1>
          <p className="mt-1 text-sm text-slate-500">Monitor OmniRoute usage ingestion, credit billing, and key suspension.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={runSync} disabled={manualSyncDisabled}>{syncing || status?.currently_running ? "Running..." : "Run manual sync"}</Button>
          <Button type="button" variant="secondary" onClick={load} disabled={loading || syncing}>Refresh</Button>
        </div>
      </div>

      <div className="mt-6 space-y-4">
        {loading ? <LoadingState label="Loading sync status" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
        {notice ? <p className="rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-sm text-blue-800">{notice}</p> : null}
      </div>

      {status && banner ? (
        <>
          <StatusBanner banner={banner} lastError={status.last_error} />

          <section className="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-6">
            <SyncMetricCard label="Fetched" value={result.fetched} hint="Events read from OmniRoute" tone="blue" />
            <SyncMetricCard label="Billed" value={result.billed} hint="Ledger debits created" tone="green" />
            <SyncMetricCard label="Duplicate" value={result.duplicate} hint="Already processed" tone="slate" />
            <SyncMetricCard label="Ignored" value={result.ignored} hint="No matching key" tone="yellow" />
            <SyncMetricCard label="Failed" value={result.failed} hint="Processing errors" tone="red" />
            <SyncMetricCard label="Suspended" value={result.suspended_keys} hint="Keys disabled" tone="yellow" />
          </section>

          {!status.last_result ? (
            <p className="mt-4 rounded-lg border border-dashed border-slate-300 bg-white px-4 py-3 text-sm text-slate-500 shadow-sm">No sync run has completed yet.</p>
          ) : null}

          <section className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_minmax(340px,0.8fr)]">
            <Card className="p-0">
              <CardHeader className="border-b border-slate-200 px-5 py-4">
                <div>
                  <CardTitle>Worker timeline</CardTitle>
                  <CardDescription>Runtime state for the current API process.</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Badge dot tone={status.currently_running ? "yellow" : "neutral"}>{status.currently_running ? "Running" : "Idle"}</Badge>
                  <Badge dot tone={status.worker_enabled ? "green" : "yellow"}>{status.worker_enabled ? "Worker enabled" : "Worker disabled"}</Badge>
                </div>
              </CardHeader>
              <div className="grid gap-0 divide-y divide-slate-100 text-sm sm:grid-cols-2 sm:divide-x sm:divide-y-0">
                <div className="divide-y divide-slate-100">
                  <TimelineRow label="Current state" value={status.currently_running ? "Running" : "Idle"} />
                  <TimelineRow label="Worker" value={status.worker_enabled ? "Enabled" : "Disabled"} />
                  <TimelineRow label="Last started" value={formatDateTime(status.last_started_at)} />
                  <TimelineRow label="Last finished" value={formatDateTime(status.last_finished_at)} />
                </div>
                <div className="divide-y divide-slate-100">
                  <TimelineRow label="Last success" value={formatDateTime(status.last_success_at)} />
                  <TimelineRow label="Next run" value={formatDateTime(status.next_run_at)} />
                  <TimelineRow label="Last duration" value={duration} />
                  <TimelineRow label="Last error" value={sanitizeError(status.last_error) ?? "-"} emphasis={Boolean(status.last_error)} />
                </div>
              </div>
            </Card>

            <Card className="p-0">
              <CardHeader className="border-b border-slate-200 px-5 py-4">
                <div>
                  <CardTitle>Configuration</CardTitle>
                  <CardDescription>Safe sync settings exposed by the API.</CardDescription>
                </div>
              </CardHeader>
              <div className="divide-y divide-slate-100 text-sm">
                <ConfigRow label="OmniRoute integration">
                  <Badge dot tone={status.omniroute_enabled ? "green" : "neutral"}>{status.omniroute_enabled ? "Enabled" : "Disabled"}</Badge>
                </ConfigRow>
                <ConfigRow label="Sync mode"><MonoValue>{status.sync_mode || "Not exposed"}</MonoValue></ConfigRow>
                <ConfigRow label="Worker interval"><span>{formatSeconds(status.worker_interval_seconds)}</span></ConfigRow>
                <ConfigRow label="Batch limit"><MonoValue>{formatNullableNumber(status.batch_limit)}</MonoValue></ConfigRow>
                <ConfigRow label="Locking"><span>PostgreSQL advisory lock</span></ConfigRow>
                <ConfigRow label="Billing mode"><span>Async usage debit</span></ConfigRow>
              </div>
            </Card>
          </section>

          <section className="mt-6 grid gap-6 xl:grid-cols-2">
            <Card className="p-5">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <CardTitle>Billing behavior</CardTitle>
                  <CardDescription className="mt-1">How synced usage becomes prepaid credit debits.</CardDescription>
                </div>
                <Badge dot tone="blue">Idempotent</Badge>
              </div>
              <ul className="mt-4 space-y-3 text-sm leading-6 text-slate-600">
                <li>Events are deduplicated by external source and external event ID.</li>
                <li>Credits are deducted only through ledger entries.</li>
                <li>If a balance reaches zero or below, the matching API key is suspended.</li>
              </ul>
              <div className="mt-5 flex flex-wrap gap-2">
                <Link className="inline-flex min-h-9 items-center rounded-md border border-slate-300 bg-white px-3 text-sm font-semibold text-slate-800 hover:bg-slate-50" href="/admin/usage">
                  Open usage events
                </Link>
                <Link className="inline-flex min-h-9 items-center rounded-md border border-slate-300 bg-white px-3 text-sm font-semibold text-slate-800 hover:bg-slate-50" href="/admin/audit">
                  Open audit logs
                </Link>
              </div>
            </Card>

            <Card className="p-5">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <CardTitle>Troubleshooting</CardTitle>
                  <CardDescription className="mt-1">Suggested next action based on the latest sync state.</CardDescription>
                </div>
                <Badge dot tone={status.last_error ? "yellow" : "green"}>{status.last_error ? "Review" : "OK"}</Badge>
              </div>
              <div className={cn("mt-4 rounded-lg border px-4 py-3 text-sm leading-6", status.last_error ? "border-amber-200 bg-amber-50 text-amber-900" : "border-emerald-200 bg-emerald-50 text-emerald-800")}>
                {troubleshooting}
              </div>
              {status.last_error ? (
                <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3">
                  <p className="text-[11px] font-semibold uppercase tracking-[0.1em] text-red-700">Last error</p>
                  <p className="mt-2 break-words font-mono text-xs leading-5 text-red-800">{sanitizeError(status.last_error)}</p>
                </div>
              ) : null}
            </Card>
          </section>
        </>
      ) : null}
    </AdminShell>
  );
}

function StatusBanner({ banner, lastError }: { banner: BannerState; lastError?: string | null }) {
  const styles = {
    green: "border-emerald-200 bg-emerald-50 text-emerald-900",
    yellow: "border-amber-200 bg-amber-50 text-amber-900",
    red: "border-red-200 bg-red-50 text-red-900",
    blue: "border-blue-200 bg-blue-50 text-blue-900",
    neutral: "border-slate-200 bg-white text-slate-800"
  }[banner.tone];

  return (
    <section className={cn("mt-6 rounded-lg border px-5 py-4 shadow-sm", styles)}>
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <Badge dot tone={banner.tone === "red" ? "red" : banner.tone === "yellow" ? "yellow" : banner.tone === "green" ? "green" : banner.tone === "blue" ? "blue" : "neutral"}>{banner.label}</Badge>
          <h2 className="mt-3 text-base font-semibold">{banner.title}</h2>
          <p className="mt-1 text-sm leading-6 opacity-90">{banner.message}</p>
          {lastError ? <p className="mt-2 max-w-4xl break-words font-mono text-xs leading-5 opacity-90">{sanitizeError(lastError)}</p> : null}
        </div>
      </div>
    </section>
  );
}

function SyncMetricCard({ label, value, hint, tone }: { label: string; value: number; hint: string; tone: "blue" | "green" | "red" | "yellow" | "slate" }) {
  const marker = {
    blue: "bg-blue-500",
    green: "bg-emerald-500",
    red: "bg-red-500",
    yellow: "bg-amber-500",
    slate: "bg-slate-300"
  }[tone];

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</p>
        <span className={cn("size-2 rounded-full", marker)} />
      </div>
      <p className="mt-3 text-2xl font-semibold tracking-[-0.02em] text-slate-950">{formatUnits(value)}</p>
      <p className="mt-1 text-xs leading-5 text-slate-500">{hint}</p>
    </Card>
  );
}

function TimelineRow({ emphasis = false, label, value }: { emphasis?: boolean; label: string; value: string }) {
  return (
    <div className="grid gap-1 px-5 py-4 sm:grid-cols-[150px_minmax(0,1fr)] sm:gap-4">
      <p className="text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">{label}</p>
      <p className={cn("break-words text-slate-900", emphasis && "font-mono text-xs text-red-700")}>{value}</p>
    </div>
  );
}

function ConfigRow({ children, label }: { children: React.ReactNode; label: string }) {
  return (
    <div className="flex items-center justify-between gap-4 px-5 py-4">
      <span className="text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">{label}</span>
      <span className="text-right text-slate-900">{children}</span>
    </div>
  );
}

function MonoValue({ children }: { children: React.ReactNode }) {
  return <span className="font-mono text-xs text-slate-800">{children}</span>;
}

function getBannerState(status: SyncStatus | null): BannerState | null {
  if (!status) {
    return null;
  }
  if (status.last_error) {
    return {
      tone: "red",
      label: "Sync issue",
      title: "Attention needed",
      message: "Check OmniRoute configuration or run manual sync after fixing settings."
    };
  }
  if (!status.worker_enabled) {
    return {
      tone: "yellow",
      label: "Worker disabled",
      title: "Automatic sync is disabled",
      message: "Manual sync is still available. Enable the background worker in production when OmniRoute billing sync should run automatically."
    };
  }
  if (!status.last_started_at) {
    return {
      tone: "blue",
      label: "Not synced yet",
      title: "No sync has started",
      message: "Run a manual sync or wait for the background worker to complete its first scheduled run."
    };
  }
  if (status.last_success_at) {
    return {
      tone: "green",
      label: "Sync healthy",
      title: "Usage billing is syncing normally",
      message: "The last sync completed successfully and no current sync error is recorded."
    };
  }
  return {
    tone: "neutral",
    label: "Waiting",
    title: "Sync state is pending",
    message: "A sync has started, but no successful completion has been recorded yet."
  };
}

function getTroubleshooting(error?: string | null) {
  if (!error) {
    return "No sync error is currently recorded. Use the usage events page to inspect billed, ignored, duplicate, and failed events.";
  }
  const normalized = error.toLowerCase();
  if (normalized.includes("not implemented")) {
    return "The configured client cannot fetch usage. Check OMNIROUTE_ENABLED and real HTTP client wiring.";
  }
  if (normalized.includes("unauthorized") || normalized.includes("forbidden") || normalized.includes("401") || normalized.includes("403")) {
    return "Check OMNIROUTE_MANAGEMENT_TOKEN and confirm SLAI and OmniRoute use the same management token.";
  }
  if (normalized.includes("not found") || normalized.includes("404")) {
    return "Check the OmniRoute usage endpoint path and confirm the patched OmniRoute fork is deployed.";
  }
  return "Review API logs and OmniRoute configuration, then run a manual sync after applying the fix.";
}

function getLastDuration(status: SyncStatus | null) {
  if (!status?.last_started_at || !status.last_finished_at) {
    return "-";
  }
  const started = new Date(status.last_started_at).getTime();
  const finished = new Date(status.last_finished_at).getTime();
  if (!Number.isFinite(started) || !Number.isFinite(finished) || finished < started) {
    return "-";
  }
  return formatDurationMs(finished - started);
}

function formatDurationMs(ms: number) {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
}

function formatSeconds(seconds?: number) {
  if (!seconds || seconds <= 0) {
    return "Not exposed";
  }
  return formatDurationMs(seconds * 1000);
}

function formatNullableNumber(value?: number) {
  if (value === undefined || value === null || value <= 0) {
    return "Not exposed";
  }
  return formatUnits(value);
}

function syncSummaryText(result: SyncResult) {
  return `Sync complete: ${formatUnits(result.fetched)} fetched, ${formatUnits(result.billed)} billed, ${formatUnits(result.duplicate)} duplicate, ${formatUnits(result.ignored)} ignored, ${formatUnits(result.failed)} failed, ${formatUnits(result.suspended_keys)} suspended keys.`;
}

function sanitizeError(error?: string | null) {
  if (!error) {
    return null;
  }
  return error
    .replace(/Bearer\s+[^\s,;]+/gi, "Bearer [redacted]")
    .replace(/(token|secret|password|authorization)=([^\s,;]+)/gi, "$1=[redacted]");
}
