"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { Badge, statusTone } from "@/components/Badge";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { MetricCard } from "@/components/MetricCard";
import { Table, Td, Th } from "@/components/Table";
import { api, isNotFound, readableError, type Balance, type LedgerEntry, type PublicAPIKey, type UsageEvent } from "@/lib/api";
import { formatDateTime, formatDelta, formatUnits } from "@/lib/format";

type DashboardData = {
  balance: Balance | null;
  usage: UsageEvent[];
  ledger: LedgerEntry[];
  apiKey: PublicAPIKey | null;
};

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

  return (
    <DashboardShell>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">User dashboard</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Credits and usage</h1>
        </div>
        <Link className="rounded-md bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800" href="/dashboard/api-key">
          Manage API key
        </Link>
      </div>

      {loading ? <div className="mt-8"><LoadingState label="Loading dashboard" /></div> : null}
      {error ? <div className="mt-8"><ErrorState message={error} onRetry={load} /></div> : null}

      {data ? (
        <>
          <section className="mt-8 grid gap-4 md:grid-cols-3">
            <MetricCard label="Available credits" value={formatUnits(data.balance?.availableUnits)} hint="Ledger-backed balance" />
            <MetricCard label="Lifetime used" value={formatUnits(data.balance?.lifetimeUsedUnits)} hint="Synced from OmniRoute logs" />
            <MetricCard label="API key" value={data.apiKey?.status ?? "None"} hint={data.apiKey?.key_prefix ?? "Create one to call OmniRoute"} />
          </section>

          <section className="mt-8 grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
            <Card>
              <CardHeader>
                <div>
                  <CardTitle>Current API key</CardTitle>
                  <CardDescription>Raw keys are shown only after create or rotate.</CardDescription>
                </div>
                {data.apiKey ? <Badge tone={statusTone(data.apiKey.status)}>{data.apiKey.status}</Badge> : null}
              </CardHeader>
              {data.apiKey ? (
                <div className="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 font-mono text-sm text-slate-700">
                  {data.apiKey.key_prefix}
                </div>
              ) : (
                <EmptyState title="No API key" message="Create one active key to call OmniRoute through SLAI billing." />
              )}
            </Card>

            <Card>
              <CardHeader>
                <div>
                  <CardTitle>Quick links</CardTitle>
                  <CardDescription>Common billing and developer workflows.</CardDescription>
                </div>
              </CardHeader>
              <div className="grid gap-3 sm:grid-cols-3">
                <Link className="rounded-lg border border-slate-200 p-4 text-sm font-semibold hover:bg-slate-50" href="/dashboard/api-key">API key</Link>
                <Link className="rounded-lg border border-slate-200 p-4 text-sm font-semibold hover:bg-slate-50" href="/dashboard/usage">Usage</Link>
                <Link className="rounded-lg border border-slate-200 p-4 text-sm font-semibold hover:bg-slate-50" href="/dashboard/billing">Billing</Link>
              </div>
            </Card>
          </section>

          <section className="mt-8 grid gap-6 xl:grid-cols-2">
            <div>
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-base font-semibold text-slate-950">Recent usage</h2>
                <Link className="text-sm font-semibold text-cyan-700" href="/dashboard/usage">View all</Link>
              </div>
              {data.usage.length === 0 ? (
                <EmptyState title="No usage yet" message="Usage appears after OmniRoute call logs are synced." />
              ) : (
                <Table>
                  <thead className="bg-slate-50"><tr><Th>Model</Th><Th>Cost</Th><Th>Status</Th><Th>Occurred</Th></tr></thead>
                  <tbody className="divide-y divide-slate-100">
                    {data.usage.map((event) => (
                      <tr key={event.id}>
                        <Td className="font-medium text-slate-950">{event.model ?? "-"}</Td>
                        <Td>{formatUnits(event.cost_units)}</Td>
                        <Td><Badge tone={statusTone(event.status)}>{event.status}</Badge></Td>
                        <Td>{formatDateTime(event.occurred_at)}</Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              )}
            </div>

            <div>
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-base font-semibold text-slate-950">Recent ledger</h2>
                <Link className="text-sm font-semibold text-cyan-700" href="/dashboard/billing">View billing</Link>
              </div>
              {data.ledger.length === 0 ? (
                <EmptyState title="No ledger entries" message="Top-ups and usage debits will appear here." />
              ) : (
                <Table>
                  <thead className="bg-slate-50"><tr><Th>Type</Th><Th>Delta</Th><Th>Balance</Th><Th>Created</Th></tr></thead>
                  <tbody className="divide-y divide-slate-100">
                    {data.ledger.map((entry) => (
                      <tr key={entry.id}>
                        <Td className="font-medium text-slate-950">{entry.type}</Td>
                        <Td>{formatDelta(entry.deltaUnits)}</Td>
                        <Td>{formatUnits(entry.balanceAfterUnits)}</Td>
                        <Td>{formatDateTime(entry.createdAt)}</Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              )}
            </div>
          </section>
        </>
      ) : null}
    </DashboardShell>
  );
}
