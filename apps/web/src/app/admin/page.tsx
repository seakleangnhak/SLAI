"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { Table, Td, Th } from "@/components/Table";
import { api, readableError, type AdminDashboard, type AdminDashboardUsage } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactCredits, formatDateTime, formatMoney, formatUnits } from "@/lib/format";

export default function AdminPage() {
  const [dashboard, setDashboard] = useState<AdminDashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function load({ silent = false }: { silent?: boolean } = {}) {
    if (silent) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);
    try {
      const response = await api.admin.dashboard.get();
      setDashboard(response);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  const creditUsagePercent = useMemo(() => {
    if (!dashboard?.credits.total_purchased_units) {
      return 0;
    }
    return Math.min(100, Math.round((dashboard.credits.total_used_units / dashboard.credits.total_purchased_units) * 100));
  }, [dashboard]);

  return (
    <AdminShell>
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-[-0.02em] text-slate-950">Admin overview</h1>
          <p className="mt-1 text-sm text-slate-500">System performance, credits, usage, and billing health.</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <span className="inline-flex min-h-9 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 shadow-sm dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
            All-time summary
          </span>
          <Button type="button" onClick={() => load({ silent: true })} disabled={loading || refreshing}>
            {refreshing ? "Refreshing" : "Refresh"}
          </Button>
        </div>
      </div>

      <div className="mt-6">
        {loading ? <LoadingDashboard /> : null}
        {error ? <ErrorState message={error} onRetry={() => load()} /> : null}
      </div>

      {!loading && !error && dashboard ? (
        <>
          <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <OverviewMetricCard
              icon="U"
              label="Users"
              value={formatCompact(dashboard.users.total)}
              footer={
                <div className="grid grid-cols-2 gap-2 border-t border-slate-100 pt-3 text-xs">
                  <MetricSubValue label="Active" tone="green" value={dashboard.users.active} />
                  <MetricSubValue label="Suspended" tone="yellow" value={dashboard.users.suspended} />
                </div>
              }
            />
            <OverviewMetricCard
              accent
              icon="R"
              label="Revenue"
              value={formatMoney(dashboard.revenue.total_paid_minor, dashboard.revenue.currency)}
              hint="Paid top-ups"
              footer={<p className="border-t border-slate-100 pt-3 text-xs text-slate-500">Currency: <span className="font-mono text-slate-800">{dashboard.revenue.currency}</span></p>}
            />
            <OverviewMetricCard
              icon="C"
              label="Credits"
              value={formatCompactCredits(dashboard.credits.total_available_units)}
              hint="Available across all users"
              footer={
                <div className="border-t border-slate-100 pt-3">
                  <div className="grid grid-cols-2 gap-2 text-xs">
                    <MetricSubValue label="Purchased" tone="green" value={dashboard.credits.total_purchased_units} formatValue={formatCompactCredits} />
                    <MetricSubValue label="Used" tone="red" value={dashboard.credits.total_used_units} formatValue={formatCompactCredits} />
                  </div>
                  {dashboard.credits.total_purchased_units > 0 ? (
                    <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-slate-100">
                      <div className="h-full rounded-full bg-blue-600" style={{ width: `${creditUsagePercent}%` }} />
                    </div>
                  ) : null}
                </div>
              }
            />
            <OverviewMetricCard
              icon="A"
              label="API / Usage Health"
              value={formatCompact(dashboard.usage.total_events)}
              hint="Total usage events"
              footer={
                <div className="grid grid-cols-3 gap-2 border-t border-slate-100 pt-3 text-xs">
                  <MetricSubValue label="Billed" tone="green" value={dashboard.usage.billed_events} />
                  <MetricSubValue label="Failed" tone="red" value={dashboard.usage.failed_events} />
                  <MetricSubValue label="Ignored" tone="yellow" value={dashboard.usage.ignored_events} />
                </div>
              }
              badge={<Badge tone={dashboard.sync_status.last_error ? "red" : "green"}>{dashboard.sync_status.last_error ? "Sync issue" : "Sync OK"}</Badge>}
            />
          </section>

          <section className="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(320px,0.8fr)]">
            <UsageActivityCard usage={dashboard.recent_usage} totalTokens={dashboard.usage.total_tokens} totalCost={dashboard.usage.total_cost_units} />
            <SyncWorkerCard dashboard={dashboard} />
          </section>

          <section className="mt-6 grid items-start gap-4 xl:grid-cols-3">
            <RecentPaymentsCard dashboard={dashboard} />
            <RecentUsageCard usage={dashboard.recent_usage} />
            <RecentAuditCard dashboard={dashboard} />
          </section>
        </>
      ) : null}
    </AdminShell>
  );
}

function LoadingDashboard() {
  return (
    <div className="space-y-4">
      <LoadingState label="Loading dashboard metrics" />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <div className="h-40 animate-pulse rounded-lg border border-slate-200 bg-white" key={index} />
        ))}
      </div>
    </div>
  );
}

function OverviewMetricCard({
  label,
  value,
  icon,
  hint,
  footer,
  badge,
  accent = false
}: {
  label: string;
  value: string;
  icon: string;
  hint?: string;
  footer: React.ReactNode;
  badge?: React.ReactNode;
  accent?: boolean;
}) {
  return (
    <Card className="relative flex min-h-40 flex-col justify-between overflow-hidden p-4">
      {accent ? <div className="pointer-events-none absolute right-0 top-0 h-24 w-24 rounded-bl-full bg-blue-600/5" /> : null}
      <div className="relative z-10 flex items-center justify-between gap-3">
        <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</span>
        {badge ?? <span className="grid size-7 place-items-center rounded-md bg-slate-100 text-xs font-semibold text-slate-500">{icon}</span>}
      </div>
      <div className="relative z-10 mt-4">
        <p className="truncate text-3xl font-semibold tracking-[-0.02em] text-slate-950">{value}</p>
        {hint ? <p className="mt-1 text-xs text-slate-500">{hint}</p> : null}
      </div>
      <div className="relative z-10 mt-4">{footer}</div>
    </Card>
  );
}

function MetricSubValue({ label, value, tone = "neutral", formatValue = formatCompact }: { label: string; value: number; tone?: "neutral" | "green" | "red" | "yellow"; formatValue?: (value: number | null | undefined) => string }) {
  const toneClass = {
    neutral: "text-slate-800",
    green: "text-emerald-700",
    red: "text-red-700",
    yellow: "text-amber-700"
  }[tone];

  return (
    <div>
      <div className="text-[10px] font-semibold uppercase tracking-[0.08em] text-slate-400">{label}</div>
      <div className={cn("mt-1 font-mono text-xs font-semibold", toneClass)}>{formatValue(value)}</div>
    </div>
  );
}

function UsageActivityCard({ usage, totalTokens, totalCost }: { usage: AdminDashboardUsage[]; totalTokens: number; totalCost: number }) {
  const maxCost = Math.max(...usage.map((event) => event.cost_units), 1);

  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Usage activity</CardTitle>
          <CardDescription>Recent billed and ignored gateway events synced into SLAI.</CardDescription>
        </div>
        <div className="hidden gap-4 text-right text-xs sm:flex">
          <div>
            <p className="text-slate-400">Tokens</p>
            <p className="font-mono font-semibold text-slate-900 dark:text-slate-100">{formatCompact(totalTokens)}</p>
          </div>
          <div>
            <p className="text-slate-400">Credits</p>
            <p className="font-mono font-semibold text-slate-900 dark:text-slate-100">{formatCompactCredits(totalCost)}</p>
          </div>
        </div>
      </CardHeader>
      {usage.length === 0 ? (
        <EmptyState title="No usage yet" message="Usage activity appears here after gateway logs are ingested." />
      ) : (
        <div className="space-y-3">
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-3 dark:border-slate-800 dark:bg-slate-950/50">
            <div className="space-y-2">
              {usage.slice(0, 5).map((event) => (
                <UsageActivityRow event={event} key={event.id} maxCost={maxCost} />
              ))}
            </div>
          </div>
          <div className="grid gap-2 sm:grid-cols-3">
            <ActivitySummary label="Events" value={formatUnits(usage.length)} />
            <ActivitySummary label="Tokens" value={formatCompact(totalTokens)} />
            <ActivitySummary label="Credits" value={formatCompactCredits(totalCost)} />
          </div>
        </div>
      )}
    </Card>
  );
}

function UsageActivityRow({ event, maxCost }: { event: AdminDashboardUsage; maxCost: number }) {
  const percent = Math.max(4, Math.min(100, Math.round((event.cost_units / maxCost) * 100)));
  const billed = event.status === "billed";

  return (
    <div className="grid gap-3 rounded-lg border border-slate-200 bg-white px-3 py-3 text-xs shadow-sm dark:border-slate-800 dark:bg-slate-900/80 md:grid-cols-[minmax(0,1fr)_160px_120px] md:items-center">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className={cn("size-2 rounded-full", billed ? "bg-emerald-500" : "bg-amber-500")} />
          <p className="truncate font-semibold text-slate-950 dark:text-slate-100">{event.user_email}</p>
        </div>
        <p className="mt-1 truncate text-slate-500 dark:text-slate-400">{event.provider ?? "unknown"} / {event.model ?? "unknown"}</p>
      </div>
      <div className="min-w-0">
        <div className="h-2 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
          <div className={cn("h-full rounded-full", billed ? "bg-blue-600" : "bg-amber-500")} style={{ width: percent + "%" }} />
        </div>
        <p className="mt-1 text-[11px] text-slate-500 dark:text-slate-400">{formatCompactCredits(event.cost_units)} credits</p>
      </div>
      <div className="flex items-center justify-between gap-3 md:block md:text-right">
        <Badge tone={statusTone(event.status)}>{event.status}</Badge>
        <p className="font-mono text-xs font-semibold text-slate-700 dark:text-slate-300 md:mt-1">{formatCompact(event.total_tokens)} tokens</p>
      </div>
    </div>
  );
}

function ActivitySummary({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs dark:border-slate-800 dark:bg-slate-900/80">
      <p className="font-semibold uppercase tracking-[0.12em] text-slate-400">{label}</p>
      <p className="mt-1 font-mono font-semibold text-slate-900 dark:text-slate-100">{value}</p>
    </div>
  );
}

function SyncWorkerCard({ dashboard }: { dashboard: AdminDashboard }) {
  const status = dashboard.sync_status;
  const apiKeyTotal = dashboard.api_keys.active + dashboard.api_keys.suspended + dashboard.api_keys.revoked;

  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Sync worker</CardTitle>
          <CardDescription>Automatic call-log ingestion and credit billing.</CardDescription>
        </div>
        <Badge tone={status.last_error ? "red" : status.worker_enabled ? "green" : "neutral"}>{status.last_error ? "Issue" : status.worker_enabled ? "Enabled" : "Off"}</Badge>
      </CardHeader>
      <dl className="space-y-3 text-sm">
        <SyncRow label="Worker" value={status.worker_enabled ? "Enabled" : "Disabled"} />
        <SyncRow label="Current state" value={status.currently_running ? "Running" : "Idle"} />
        <SyncRow label="Last success" value={formatDateTime(status.last_success_at)} />
        <SyncRow label="Active keys" value={formatUnits(dashboard.api_keys.active)} />
        <SyncRow label="Total keys" value={formatUnits(apiKeyTotal)} />
      </dl>
      {status.last_error ? <p className="mt-4 rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">{status.last_error}</p> : null}
      <Link className="mt-4 inline-flex min-h-9 items-center justify-center rounded-md border border-slate-300 bg-white px-3 text-sm font-semibold text-slate-800 hover:bg-slate-50" href="/admin/sync">
        Open sync controls
      </Link>
    </Card>
  );
}

function SyncRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-slate-100 pb-3 last:border-0 last:pb-0">
      <dt className="text-slate-500">{label}</dt>
      <dd className="text-right font-medium text-slate-950">{value}</dd>
    </div>
  );
}

function RecentPaymentsCard({ dashboard }: { dashboard: AdminDashboard }) {
  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Recent payments</CardTitle>
          <CardDescription>Latest credit payments.</CardDescription>
        </div>
        <Link className="text-sm font-semibold text-blue-700 hover:text-blue-800" href="/admin/payments">View all</Link>
      </CardHeader>
      {dashboard.recent_payments.length === 0 ? (
        <EmptyState title="No payments" message="Credit payments will appear here." />
      ) : (
        <div className="overflow-hidden rounded-lg border border-slate-200">
          <Table className="text-xs">
            <thead className="bg-slate-50"><tr><Th>User</Th><Th>Amount</Th><Th>Status</Th></tr></thead>
            <tbody className="divide-y divide-slate-100">
              {dashboard.recent_payments.map((payment) => (
                <tr key={payment.id}>
                  <Td className="max-w-40 truncate font-medium text-slate-950">{payment.user_email}</Td>
                  <Td>
                    <div className="font-mono text-slate-900">{formatMoney(payment.amount_minor, payment.currency)}</div>
                    <div className="text-xs text-slate-500">{formatCompactCredits(payment.credit_units)} credits</div>
                  </Td>
                  <Td>
                    <Badge tone={payment.status === "paid" ? "green" : "neutral"}>{payment.status}</Badge>
                    <div className="mt-1 text-xs text-slate-500">{formatDateTime(payment.created_at)}</div>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </div>
      )}
    </Card>
  );
}

function RecentUsageCard({ usage }: { usage: AdminDashboardUsage[] }) {
  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Recent usage</CardTitle>
          <CardDescription>Latest usage events by user.</CardDescription>
        </div>
        <Link className="text-sm font-semibold text-blue-700 hover:text-blue-800" href="/admin/usage">View all</Link>
      </CardHeader>
      {usage.length === 0 ? (
        <EmptyState title="No usage" message="Usage sync has not billed any events yet." />
      ) : (
        <div className="space-y-3">
          {usage.map((event) => (
            <div className="rounded-lg border border-slate-200 px-3 py-2" key={event.id}>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-slate-950">{event.user_email}</p>
                  <p className="mt-1 truncate text-xs text-slate-500">{event.provider ?? "unknown"} / {event.model ?? "unknown"}</p>
                </div>
                <Badge tone={statusTone(event.status)}>{event.status}</Badge>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
                <MetricSubValue label="Tokens" value={event.total_tokens} />
                <MetricSubValue label="Cost" value={event.cost_units} formatValue={formatCompactCredits} />
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function RecentAuditCard({ dashboard }: { dashboard: AdminDashboard }) {
  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Recent audit logs</CardTitle>
          <CardDescription>Recent admin actions.</CardDescription>
        </div>
        <Link className="text-sm font-semibold text-blue-700 hover:text-blue-800" href="/admin/audit">View all</Link>
      </CardHeader>
      {dashboard.recent_audit_logs.length === 0 ? (
        <EmptyState title="No audit logs" message="Admin actions will be recorded here." />
      ) : (
        <div className="space-y-3">
          {dashboard.recent_audit_logs.slice(0, 4).map((log) => (
            <div className="rounded-lg border border-slate-200 px-3 py-2" key={log.id}>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-slate-950">{log.action}</p>
                  <p className="mt-1 truncate text-xs text-slate-500">{log.admin_email}</p>
                </div>
                <Badge>{log.target_type ?? "system"}</Badge>
              </div>
              <p className="mt-2 text-xs text-slate-500">{formatDateTime(log.created_at)}</p>
              {log.target_id ? <p className="mt-1 truncate font-mono text-[11px] text-slate-400">{truncateID(log.target_id)}</p> : null}
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function formatCompact(value: number | null | undefined) {
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 1,
    notation: "compact"
  }).format(value ?? 0);
}

function truncateID(value: string) {
  if (value.length <= 18) {
    return value;
  }
  return `${value.slice(0, 8)}...${value.slice(-6)}`;
}
