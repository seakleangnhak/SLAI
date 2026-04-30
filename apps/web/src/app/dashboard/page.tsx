"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import { Badge, statusTone } from "@/components/Badge";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, isNotFound, readableError, type Balance, type LedgerEntry, type PublicAPIKey, type UsageEvent } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactCredits, formatCompactUnits, formatCredits, formatDateTime, formatDelta, formatLedgerReason } from "@/lib/format";

type DashboardData = {
  balance: Balance | null;
  usage: UsageEvent[];
  ledger: LedgerEntry[];
  apiKey: PublicAPIKey | null;
};

const primaryButton =
  "inline-flex items-center justify-center rounded-lg bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-800";
const secondaryButton =
  "inline-flex items-center justify-center rounded-lg border border-slate-200 bg-white px-3.5 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50";
const ghostLink = "text-sm font-semibold text-blue-700 hover:text-blue-800";
const quickstartCurl = [
  "curl https://YOUR_SLAI_GATEWAY_DOMAIN/v1/chat/completions \\",
  '  -H "Authorization: Bearer YOUR_API_KEY" \\',
  '  -H "Content-Type: application/json"'
].join("\n");

function apiKeyLabel(apiKey: PublicAPIKey | null) {
  if (!apiKey) {
    return "None";
  }
  return apiKey.status;
}

function usageTone(status: string) {
  if (status === "billed") {
    return "green" as const;
  }
  if (status === "failed") {
    return "red" as const;
  }
  if (status === "ignored") {
    return "yellow" as const;
  }
  if (status === "pending") {
    return "blue" as const;
  }
  return statusTone(status);
}

function HeroMetricCard({
  label,
  value,
  hint,
  accent = false,
  children
}: {
  label: string;
  value: string;
  hint: string;
  accent?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <Card
      className={cn(
        "relative min-h-44 overflow-hidden rounded-2xl p-5 shadow-sm transition hover:shadow-md",
        accent
          ? "border-blue-200 bg-gradient-to-br from-white via-blue-50/70 to-cyan-50 shadow-blue-950/5"
          : "bg-white"
      )}
    >
      {accent ? (
        <>
          <div className="absolute -right-14 -top-16 size-36 rounded-full bg-blue-200/60 blur-3xl" />
          <div className="absolute bottom-0 left-0 h-1 w-full bg-gradient-to-r from-blue-600 via-cyan-500 to-transparent" />
        </>
      ) : null}
      <div className="relative flex h-full flex-col">
        <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">{label}</p>
        <p className={cn("mt-3 font-semibold tracking-normal text-slate-950", accent ? "text-4xl" : "text-3xl")}>{value}</p>
        <p className="mt-2 text-sm leading-6 text-slate-500">{hint}</p>
        {children ? <div className="mt-auto pt-4">{children}</div> : null}
      </div>
    </Card>
  );
}

function StatPill({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white/75 px-3 py-2 shadow-sm">
      <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</p>
      <p className="mt-1 text-sm font-semibold text-slate-950">{value}</p>
    </div>
  );
}

function PanelEmpty({
  title,
  message,
  action,
  compact = true
}: {
  title: string;
  message: string;
  action?: React.ReactNode;
  compact?: boolean;
}) {
  return (
    <div className={cn("rounded-xl border border-dashed border-slate-200 bg-slate-50/80 px-4 text-center", compact ? "py-5" : "py-8")}>
      <span className="mx-auto grid size-8 place-items-center rounded-full bg-white text-sm font-semibold text-blue-700 shadow-sm ring-1 ring-slate-200">S</span>
      <h3 className="mt-3 text-sm font-semibold text-slate-950">{title}</h3>
      <p className="mx-auto mt-1 max-w-md text-sm leading-6 text-slate-500">{message}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

function DetailItem({ label, value, mono = false }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</dt>
      <dd className={cn("mt-1 truncate text-sm font-medium text-slate-950", mono && "font-mono")}>{value}</dd>
    </div>
  );
}

function SectionHeader({ title, description, href, linkLabel }: { title: string; description: string; href: string; linkLabel: string }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-slate-200 px-5 py-4">
      <div className="min-w-0">
        <CardTitle>{title}</CardTitle>
        <CardDescription className="mt-1 leading-5">{description}</CardDescription>
      </div>
      <Link className={cn(ghostLink, "shrink-0")} href={href}>{linkLabel}</Link>
    </div>
  );
}

function CurrentAPIKeyCard({ apiKey }: { apiKey: PublicAPIKey | null }) {
  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-4 items-start sm:items-center">
        <div>
          <CardTitle>Current API key</CardTitle>
          <CardDescription>Only safe key metadata is visible after create or rotate.</CardDescription>
        </div>
        {apiKey ? <Badge dot tone={statusTone(apiKey.status)}>{apiKey.status}</Badge> : <Badge dot tone="neutral">None</Badge>}
      </CardHeader>

      {apiKey ? (
        <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div className="min-w-0">
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">Key prefix</p>
                <span className="rounded-md bg-white px-2 py-1 text-xs font-medium text-slate-500 ring-1 ring-slate-200">raw key hidden</span>
              </div>
              <p className="mt-3 break-all font-mono text-sm font-semibold text-slate-950">{apiKey.key_prefix}</p>
            </div>
            <dl className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <DetailItem label="Created" value={formatDateTime(apiKey.created_at)} />
              <DetailItem label="Last used" value={formatDateTime(apiKey.last_used_at)} />
              <DetailItem label="Access" value="Managed by SLAI" />
              <DetailItem label="Name" value={apiKey.name || "Default key"} />
            </dl>
          </div>
          <div className="flex flex-wrap gap-3 lg:flex-col">
            <Link className={primaryButton} href="/dashboard/api-key">Manage</Link>
            {apiKey.status !== "REVOKED" ? <Link className={secondaryButton} href="/dashboard/api-key">Rotate</Link> : null}
          </div>
        </div>
      ) : (
        <PanelEmpty
          title="No API key"
          message="Create one active key to send SLAI-billed AI requests. The raw key is shown once."
          action={<Link className={primaryButton} href="/dashboard/api-key">Create API key</Link>}
          compact={false}
        />
      )}
    </Card>
  );
}

function QuickstartCard({ hasKey }: { hasKey: boolean }) {
  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-4 items-start">
        <div>
          <CardTitle>Quickstart</CardTitle>
          <CardDescription>Start sending requests through your SLAI-billed API key.</CardDescription>
        </div>
      </CardHeader>

      <div className="overflow-hidden rounded-2xl border border-slate-800 bg-slate-950 shadow-sm">
        <div className="flex items-center justify-between border-b border-white/10 px-4 py-2.5">
          <span className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-400">curl</span>
          <span className="rounded-md bg-white/10 px-2 py-1 text-xs font-medium text-slate-300">SLAI API</span>
        </div>
        <pre className="overflow-x-auto p-4 text-xs leading-6 text-slate-100 sm:text-sm">
          <code>{quickstartCurl}</code>
        </pre>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_auto] lg:items-center">
        <p className="text-sm leading-6 text-slate-500">
          {hasKey
            ? "Use the raw key you saved after create or rotate. SLAI stores only safe key metadata after that moment."
            : "Create an API key first, then use the one-time raw key in your server-side integration."}
        </p>
        <div className="grid gap-2 sm:grid-cols-3 lg:min-w-80">
          <QuickAction href="/dashboard/api-key" label="API key" helper="Manage" />
          <QuickAction href="/dashboard/usage" label="Usage" helper="Inspect" />
          <QuickAction href="/dashboard/billing" label="Billing" helper="Ledger" />
        </div>
      </div>
    </Card>
  );
}

function QuickAction({ href, label, helper }: { href: string; label: string; helper: string }) {
  return (
    <Link className="rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm shadow-sm transition hover:border-blue-200 hover:bg-blue-50/50" href={href}>
      <span className="block font-semibold text-slate-950">{label}</span>
      <span className="mt-0.5 block text-xs text-slate-500">{helper}</span>
    </Link>
  );
}

function RecentUsagePanel({ usage }: { usage: UsageEvent[] }) {
  const hasUsage = usage.length > 0;

  return (
    <Card className="rounded-2xl p-0">
      <SectionHeader title="Recent usage" description="Latest synced usage events." href="/dashboard/usage" linkLabel="View all" />

      {hasUsage ? (
        <div className="divide-y divide-slate-100">
          {usage.map((event) => (
            <div key={event.id} className="px-4 py-3.5 transition hover:bg-slate-50">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-slate-950">{event.model ?? "Unknown model"}</p>
                  <p className="mt-1 truncate text-xs text-slate-500">{event.provider ?? "Unknown provider"} / {formatDateTime(event.occurred_at)}</p>
                </div>
                <Badge dot tone={usageTone(event.status)}>{event.status}</Badge>
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-slate-500">
                <span><span className="font-semibold text-slate-800">{formatCompactUnits(event.total_tokens)}</span> tokens</span>
                <span><span className="font-semibold text-slate-800">{formatCredits(event.cost_units)}</span> credits</span>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="p-4">
          <PanelEmpty title="No usage yet" message="Usage appears after provider logs are synced." />
        </div>
      )}
    </Card>
  );
}

function RecentLedgerPanel({ ledger }: { ledger: LedgerEntry[] }) {
  const hasLedger = ledger.length > 0;

  return (
    <Card className="rounded-2xl p-0">
      <SectionHeader title="Recent ledger" description="Credits, debits, and balance changes." href="/dashboard/billing" linkLabel="View billing" />

      {hasLedger ? (
        <div className="divide-y divide-slate-100">
          {ledger.map((entry) => {
            const isCredit = entry.deltaUnits >= 0;
            return (
              <div key={entry.id} className="px-4 py-3.5 transition hover:bg-slate-50">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge dot tone={isCredit ? "green" : "yellow"}>{entry.type}</Badge>
                      <span className="text-xs text-slate-500">{formatDateTime(entry.createdAt)}</span>
                    </div>
                    <p className="mt-2 truncate text-sm text-slate-600">{formatLedgerReason(entry)}</p>
                  </div>
                  <div className="shrink-0 text-right">
                    <p className={cn("text-sm font-semibold", isCredit ? "text-emerald-700" : "text-slate-950")}>{formatDelta(entry.deltaUnits)}</p>
                    <p className="mt-1 text-xs text-slate-500">{formatCompactCredits(entry.balanceAfterUnits)} bal.</p>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="p-4">
          <PanelEmpty title="No ledger entries" message="Top-ups and usage debits will appear here." />
        </div>
      )}
    </Card>
  );
}

function NeedCreditsCard() {
  return (
    <Card className="rounded-2xl border-amber-200 bg-gradient-to-r from-amber-50 to-white p-4 shadow-sm">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex gap-3">
          <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-full bg-amber-100 text-sm font-bold text-amber-700">0</span>
          <div>
            <CardTitle>Need credits?</CardTitle>
            <CardDescription className="mt-1 text-amber-800/80">Top-ups are managed by an administrator in this MVP. Credits never expire.</CardDescription>
          </div>
        </div>
        <Link className={secondaryButton} href="/dashboard/billing">Open billing</Link>
      </div>
    </Card>
  );
}

export default function DashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    Promise.allSettled([api.balance.get(), api.usage.list(5), api.ledger.list(5), api.apiKeys.get()])
      .then(([balanceResult, usageResult, ledgerResult, apiKeyResult]) => {
        if (balanceResult.status === "rejected") {
          throw balanceResult.reason;
        }
        if (usageResult.status === "rejected") {
          throw usageResult.reason;
        }
        if (ledgerResult.status === "rejected") {
          throw ledgerResult.reason;
        }
        let apiKey: PublicAPIKey | null = null;
        if (apiKeyResult.status === "fulfilled") {
          apiKey = apiKeyResult.value.api_key;
        } else if (!isNotFound(apiKeyResult.reason)) {
          throw apiKeyResult.reason;
        }
        setData({
          balance: balanceResult.value.balance,
          usage: usageResult.value.usage,
          ledger: ledgerResult.value.ledger,
          apiKey
        });
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  const balance = data?.balance;
  const availableCredits = balance?.availableUnits ?? 0;
  const purchasedCredits = balance?.lifetimePurchasedUnits ?? 0;
  const usedCredits = balance?.lifetimeUsedUnits ?? 0;
  const hasApiKey = Boolean(data?.apiKey);
  const apiKeyStatus = apiKeyLabel(data?.apiKey ?? null);
  const usedPercent = useMemo(() => {
    if (!purchasedCredits) {
      return 0;
    }
    return Math.min(100, Math.round((usedCredits / purchasedCredits) * 100));
  }, [purchasedCredits, usedCredits]);

  return (
    <DashboardShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">User dashboard</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Credits and usage</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Monitor your credit balance, API key status, and recent activity.</p>
          </div>
          <Link className={primaryButton} href="/dashboard/api-key">
            {hasApiKey ? "Manage API key" : "Create API key"}
          </Link>
        </div>
      </section>

      {loading ? <div className="mt-8"><LoadingState label="Loading dashboard" /></div> : null}
      {error ? <div className="mt-8"><ErrorState message={error} onRetry={load} /></div> : null}

      {data ? (
        <>
          <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-[1.35fr_1fr_1fr_1fr]">
            <HeroMetricCard label="Available credits" value={formatCredits(availableCredits)} hint="Ledger-backed prepaid balance" accent>
              <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
                <StatPill label="Never expires" value="Yes" />
                <StatPill label="Backing" value="Ledger" />
              </div>
            </HeroMetricCard>
            <HeroMetricCard label="Lifetime used" value={formatCredits(usedCredits)} hint="Synced from provider logs">
              {purchasedCredits > 0 ? (
                <div>
                  <div className="h-2 rounded-full bg-slate-100">
                    <div className="h-2 rounded-full bg-blue-600" style={{ width: usedPercent + "%" }} />
                  </div>
                  <p className="mt-2 text-xs text-slate-500">{usedPercent}% of purchased credits used</p>
                </div>
              ) : <p className="text-xs text-slate-500">No usage debits yet</p>}
            </HeroMetricCard>
            <HeroMetricCard label="API key status" value={apiKeyStatus} hint={data.apiKey?.key_prefix ?? "Create one key to send requests"}>
              <Badge dot tone={data.apiKey ? statusTone(data.apiKey.status) : "neutral"}>{apiKeyStatus}</Badge>
            </HeroMetricCard>
            <HeroMetricCard label="Lifetime purchased" value={formatCredits(purchasedCredits)} hint="Admin-managed manual top-ups">
              <p className="text-xs text-slate-500">Updated {formatDateTime(balance?.updatedAt)}</p>
            </HeroMetricCard>
          </section>

          {availableCredits === 0 ? <div className="mt-5"><NeedCreditsCard /></div> : null}

          <section className="mt-7 grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_minmax(340px,0.9fr)] 2xl:grid-cols-[minmax(0,1.55fr)_minmax(380px,0.9fr)]">
            <div className="space-y-6">
              <CurrentAPIKeyCard apiKey={data.apiKey} />
              <QuickstartCard hasKey={hasApiKey} />
            </div>
            <div className="space-y-6">
              <RecentUsagePanel usage={data.usage} />
              <RecentLedgerPanel ledger={data.ledger} />
            </div>
          </section>
        </>
      ) : null}
    </DashboardShell>
  );
}
