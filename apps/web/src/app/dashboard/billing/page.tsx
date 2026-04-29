"use client";

import { useEffect, useState } from "react";

import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { MetricCard } from "@/components/MetricCard";
import { Table, Td, Th } from "@/components/Table";
import { api, readableError, type Balance, type CreditPackage, type LedgerEntry } from "@/lib/api";
import { formatDateTime, formatDelta, formatMoney, formatUnits } from "@/lib/format";

type BillingData = {
  balance: Balance;
  ledger: LedgerEntry[];
  packages: CreditPackage[];
};

export default function BillingPage() {
  const [data, setData] = useState<BillingData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    Promise.all([api.balance.get(), api.ledger.list(50), api.packages.listPublic()])
      .then(([balance, ledger, packages]) => setData({ balance: balance.balance, ledger: ledger.ledger, packages: packages.packages }))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  return (
    <DashboardShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Billing</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Credits and ledger</h1>
      </div>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading billing" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
      </div>

      {data ? (
        <>
          <section className="mt-8 grid gap-4 md:grid-cols-3">
            <MetricCard label="Available credits" value={formatUnits(data.balance.availableUnits)} hint="Credits never expire" />
            <MetricCard label="Lifetime purchased" value={formatUnits(data.balance.lifetimePurchasedUnits)} hint="Manual top-ups" />
            <MetricCard label="Lifetime used" value={formatUnits(data.balance.lifetimeUsedUnits)} hint="Usage debits" />
          </section>

          <Card className="mt-8 border-cyan-200 bg-cyan-50">
            <CardTitle>Manual top-up only</CardTitle>
            <CardDescription>
              Stripe is not implemented in the MVP. Contact an admin to add prepaid credits to your account.
            </CardDescription>
          </Card>

          <section className="mt-8 grid gap-6 xl:grid-cols-[0.9fr_1.1fr]">
            <div>
              <h2 className="mb-3 text-base font-semibold text-slate-950">Available packages</h2>
              {data.packages.length === 0 ? (
                <EmptyState title="No packages" message="An admin has not created public credit packages yet." />
              ) : (
                <div className="grid gap-4">
                  {data.packages.map((pkg) => (
                    <Card key={pkg.id}>
                      <CardHeader>
                        <div>
                          <CardTitle>{pkg.name}</CardTitle>
                          <CardDescription>{pkg.description ?? "Prepaid credits."}</CardDescription>
                        </div>
                      </CardHeader>
                      <div className="flex items-end justify-between gap-4">
                        <div>
                          <p className="text-2xl font-semibold text-slate-950">{formatUnits(pkg.creditUnits + pkg.bonusCreditUnits)}</p>
                          <p className="text-sm text-slate-500">credit units</p>
                        </div>
                        <p className="text-lg font-semibold text-slate-900">{formatMoney(pkg.priceMinor, pkg.currency)}</p>
                      </div>
                    </Card>
                  ))}
                </div>
              )}
            </div>

            <div>
              <h2 className="mb-3 text-base font-semibold text-slate-950">Ledger</h2>
              {data.ledger.length === 0 ? (
                <EmptyState title="No ledger entries" message="Top-ups, usage, refunds, and adjustments appear here." />
              ) : (
                <Table>
                  <thead className="bg-slate-50"><tr><Th>Type</Th><Th>Delta</Th><Th>Balance</Th><Th>Reason</Th><Th>Created</Th></tr></thead>
                  <tbody className="divide-y divide-slate-100">
                    {data.ledger.map((entry) => (
                      <tr key={entry.id}>
                        <Td className="font-medium text-slate-950">{entry.type}</Td>
                        <Td>{formatDelta(entry.deltaUnits)}</Td>
                        <Td>{formatUnits(entry.balanceAfterUnits)}</Td>
                        <Td>{entry.reason ?? "-"}</Td>
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
