"use client";

import { useEffect, useState } from "react";

import { AppShell } from "@/components/AppShell";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type CreditPackage } from "@/lib/api";
import { formatMoney, formatUnits } from "@/lib/format";

export default function PackagesPage() {
  const [packages, setPackages] = useState<CreditPackage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    api.packages
      .listPublic()
      .then((response) => setPackages(response.packages))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  return (
    <AppShell section="public">
      <div className="mb-8">
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Packages</p>
        <h1 className="mt-2 text-3xl font-semibold text-slate-950">Prepaid credit packages</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-500">Stripe is not implemented in MVP. Packages are visible here, and top-ups are created manually by an admin.</p>
      </div>
      {loading ? <LoadingState label="Loading packages" /> : null}
      {error ? <ErrorState message={error} onRetry={load} /> : null}
      {!loading && !error && packages.length === 0 ? <EmptyState title="No packages yet" message="An admin can create active packages from the admin console." /> : null}
      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {packages.map((pkg) => (
          <Card key={pkg.id}>
            <CardHeader>
              <div>
                <CardTitle>{pkg.name}</CardTitle>
                <CardDescription>{pkg.description ?? "Prepaid credits that never expire."}</CardDescription>
              </div>
            </CardHeader>
            <p className="text-3xl font-semibold text-slate-950">{formatUnits(pkg.creditUnits + pkg.bonusCreditUnits)}</p>
            <p className="mt-1 text-sm text-slate-500">credit units</p>
            <p className="mt-5 text-lg font-semibold text-slate-900">{formatMoney(pkg.priceMinor, pkg.currency)}</p>
          </Card>
        ))}
      </section>
    </AppShell>
  );
}
