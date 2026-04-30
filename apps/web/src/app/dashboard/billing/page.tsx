"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type Balance, type CreditPackage, type LedgerEntry, type Payment } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactCredits, formatCredits, formatDateTime, formatDelta, formatLedgerReason, formatMoney, formatUnits, truncateId } from "@/lib/format";

type BillingData = {
  balance: Balance;
  ledger: LedgerEntry[];
  packages: CreditPackage[];
  payments: Payment[];
};

const secondaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-400";

function paymentStatusTone(status?: string) {
  if (status === "paid") {
    return "green" as const;
  }
  if (status === "pending" || status === "pending_payment" || status === "pending_proof" || status === "pending_review") {
    return "blue" as const;
  }
  if (status === "failed" || status === "cancelled" || status === "rejected" || status === "needs_review") {
    return "red" as const;
  }
  if (status === "refunded" || status === "expired") {
    return "yellow" as const;
  }
  return "neutral" as const;
}

function ledgerTone(type: string) {
  if (type === "payment_credit" || type === "bonus_credit") {
    return "green" as const;
  }
  if (type === "admin_adjustment_credit") {
    return "blue" as const;
  }
  if (type === "usage_debit") {
    return "red" as const;
  }
  if (type === "admin_adjustment_debit" || type === "refund_debit") {
    return "yellow" as const;
  }
  return "neutral" as const;
}

function BalanceCard({ label, value, hint, accent = false, children }: { label: string; value: string; hint: string; accent?: boolean; children?: React.ReactNode }) {
  return (
    <Card
      className={cn(
        "relative min-h-40 overflow-hidden rounded-2xl p-5 shadow-sm transition hover:shadow-md",
        accent ? "border-blue-200 bg-gradient-to-br from-white via-blue-50/70 to-cyan-50 shadow-blue-950/5" : "bg-white"
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

function EmptyPanel({ title, message, action }: { title: string; message: string; action?: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-dashed border-slate-200 bg-slate-50/80 px-5 py-8 text-center">
      <span className="mx-auto grid size-10 place-items-center rounded-full bg-white font-semibold text-blue-700 shadow-sm ring-1 ring-slate-200">B</span>
      <h3 className="mt-4 text-base font-semibold text-slate-950">{title}</h3>
      <p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-slate-500">{message}</p>
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  );
}

function PaymentCheckoutBanner() {
  return (
    <Card className="rounded-2xl border-amber-200 bg-gradient-to-r from-amber-50 to-white p-4 shadow-sm">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex gap-3">
          <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-full bg-amber-100 text-sm font-bold text-amber-700">M</span>
          <div>
            <CardTitle>Bakong KHQR checkout</CardTitle>
            <CardDescription className="mt-1 text-amber-800/80">
              Choose a package, pay with KHQR, and SLAI credits your balance after payment confirmation.
            </CardDescription>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span className="rounded-md bg-white px-2 py-1 text-xs font-semibold text-slate-500 ring-1 ring-slate-200">Auto confirmation</span>
          <a className={secondaryButton} href="#packages">View packages</a>
        </div>
      </div>
    </Card>
  );
}

function PackageCard({ pkg, choosing, onChoose }: { pkg: CreditPackage; choosing: boolean; onChoose: () => void }) {
  const totalCredits = pkg.creditUnits + pkg.bonusCreditUnits;
  return (
    <Card className="rounded-2xl p-5 shadow-sm transition hover:border-blue-200 hover:shadow-md">
      <div className="flex h-full flex-col gap-5">
        <div>
          <div className="flex items-start justify-between gap-3">
            <div>
              <h3 className="text-base font-semibold text-slate-950">{pkg.name}</h3>
              <p className="mt-1 line-clamp-2 text-sm leading-6 text-slate-500">{pkg.description ?? "Prepaid SLAI credits."}</p>
            </div>
            {pkg.bonusCreditUnits > 0 ? <Badge tone="blue">Bonus</Badge> : null}
          </div>
          <p className="mt-5 text-3xl font-semibold text-slate-950">{formatMoney(pkg.priceMinor, pkg.currency)}</p>
          <p className="mt-1 text-sm text-slate-500">Bakong KHQR checkout</p>
        </div>

        <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
          <div className="flex items-end justify-between gap-4">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">Total credits</p>
              <p className="mt-1 text-2xl font-semibold text-slate-950">{formatCredits(totalCredits)}</p>
            </div>
            <div className="text-right text-sm text-slate-500">
              <p>{formatCompactCredits(pkg.creditUnits)} included</p>
              {pkg.bonusCreditUnits > 0 ? <p>{formatCompactCredits(pkg.bonusCreditUnits)} bonus</p> : null}
            </div>
          </div>
        </div>

        <div className="mt-auto">
          <Button className="w-full rounded-lg" type="button" variant="secondary" onClick={onChoose} disabled={choosing}>{choosing ? "Creating checkout" : "Choose package"}</Button>
          <p className="mt-2 text-center text-xs text-slate-500">
            Pay with KHQR. Credits are added after confirmation.
          </p>
        </div>
      </div>
    </Card>
  );
}


function formatProvider(provider: string) {
  if (provider === "bakong_khqr") {
    return "Bakong KHQR";
  }
  if (provider === "manual") {
    return "Manual";
  }
  return provider;
}

function PaymentsTable({ payments, packageNameById }: { payments: Payment[]; packageNameById: Map<string, string> }) {
  if (payments.length === 0) {
    return <EmptyPanel title="No top-ups yet" message="Package checkouts and admin top-ups will appear here." />;
  }

  return (
    <Card className="overflow-hidden rounded-2xl p-0">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50">
            <tr>
              {["Date", "Package", "Amount", "Credits", "Status", "Provider", "Reference", "Action"].map((label) => (
                <th key={label} className="whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {payments.map((payment) => {
              const packageName = payment.packageName ?? (payment.packageId ? packageNameById.get(payment.packageId) ?? truncateId(payment.packageId, 8, 4) : "Manual top-up");
              return (
                <tr key={payment.id} className="hover:bg-slate-50">
                  <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(payment.paidAt ?? payment.createdAt)}</td>
                  <td className="whitespace-nowrap px-4 py-3 font-medium text-slate-950">{packageName}</td>
                  <td className="whitespace-nowrap px-4 py-3 font-semibold text-slate-950">{formatMoney(payment.amountMinor, payment.currency)}</td>
                  <td className="whitespace-nowrap px-4 py-3 text-slate-700">{formatCredits(payment.creditUnits)}</td>
                  <td className="whitespace-nowrap px-4 py-3"><Badge dot tone={paymentStatusTone(payment.status)}>{payment.status}</Badge></td>
                  <td className="whitespace-nowrap px-4 py-3 text-slate-700">{formatProvider(payment.provider)}</td>
                  <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-slate-600">{truncateId(payment.providerTransactionId ?? payment.adminPaymentReference ?? payment.checkoutReference ?? payment.providerRef ?? payment.id, 10, 4)}</td>
                  <td className="whitespace-nowrap px-4 py-3"><Link className="text-sm font-semibold text-blue-700 hover:text-blue-800" href={`/checkout/${payment.id}`}>{payment.status === "pending_payment" || payment.status === "pending_proof" || payment.status === "rejected" ? "Continue" : "View"}</Link></td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

function LedgerTable({ ledger }: { ledger: LedgerEntry[] }) {
  if (ledger.length === 0) {
    return <EmptyPanel title="No ledger activity" message="Top-ups, usage debits, refunds, and adjustments will appear here." />;
  }

  return (
    <Card className="overflow-hidden rounded-2xl p-0">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50">
            <tr>
              {["Date", "Type", "Delta", "Balance after", "Reason", "Reference"].map((label) => (
                <th key={label} className="whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {ledger.map((entry) => {
              const positive = entry.deltaUnits >= 0;
              const reference = entry.paymentId ?? entry.usageEventId ?? entry.idempotencyKey ?? entry.id;
              return (
                <tr key={entry.id} className="hover:bg-slate-50">
                  <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(entry.createdAt)}</td>
                  <td className="whitespace-nowrap px-4 py-3"><Badge dot tone={ledgerTone(entry.type)}>{entry.type}</Badge></td>
                  <td className={cn("whitespace-nowrap px-4 py-3 font-semibold", positive ? "text-emerald-700" : "text-red-700")}>{formatDelta(entry.deltaUnits)}</td>
                  <td className="whitespace-nowrap px-4 py-3 text-slate-700">{formatCredits(entry.balanceAfterUnits)}</td>
                  <td className="min-w-56 px-4 py-3 text-slate-700">{formatLedgerReason(entry)}</td>
                  <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-slate-600">{truncateId(reference, 10, 4)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

function SectionHeading({ title, description }: { title: string; description: string }) {
  return (
    <div className="mb-3 flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h2 className="text-base font-semibold text-slate-950">{title}</h2>
        <p className="mt-1 text-sm text-slate-500">{description}</p>
      </div>
    </div>
  );
}

export default function BillingPage() {
  const [data, setData] = useState<BillingData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const router = useRouter();
  const [choosingPackageId, setChoosingPackageId] = useState<string | null>(null);
  const [checkoutError, setCheckoutError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    Promise.all([api.balance.get(), api.ledger.list(50), api.packages.listPublic()])
      .then(async ([balance, ledger, packages]) => {
        let payments: Payment[] = [];
        try {
          const paymentResponse = await api.payments.list(50);
          payments = Array.isArray(paymentResponse.payments) ? paymentResponse.payments : [];
        } catch {
          payments = [];
        }
        setData({
          balance: balance.balance,
          ledger: ledger.ledger,
          packages: packages.packages,
          payments
        });
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  async function choosePackage(packageId: string) {
    setChoosingPackageId(packageId);
    setCheckoutError(null);
    try {
      const response = await api.checkout.package(packageId);
      router.push(`/checkout/${response.payment.id}`);
    } catch (err) {
      setCheckoutError(readableError(err));
    } finally {
      setChoosingPackageId(null);
    }
  }

  const payments = data?.payments ?? [];
  const packages = data?.packages ?? [];
  const ledger = data?.ledger ?? [];
  const packageNameById = useMemo(() => {
    const result = new Map<string, string>();
    for (const pkg of packages) {
      result.set(pkg.id, pkg.name);
    }
    return result;
  }, [packages]);

  return (
    <DashboardShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Billing</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Credits and billing</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Manage prepaid credits, packages, top-ups, and ledger activity.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <a className={secondaryButton} href="#packages">View packages</a>
            <Button className="rounded-lg" type="button" variant="secondary" onClick={load} disabled={loading}>Refresh</Button>
          </div>
        </div>
      </section>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading billing" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
      </div>

      {data ? (
        <>
          <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-[1.35fr_1fr_1fr_1fr]">
            <BalanceCard label="Available credits" value={formatCredits(data.balance.availableUnits)} hint="Credits never expire" accent>
              <div className="flex flex-wrap gap-2">
                <Badge tone="blue">Prepaid</Badge>
                <Badge tone="green">Ledger-backed</Badge>
              </div>
            </BalanceCard>
            <BalanceCard label="Lifetime purchased" value={formatCredits(data.balance.lifetimePurchasedUnits)} hint="Admin-confirmed top-ups" />
            <BalanceCard label="Lifetime used" value={formatCredits(data.balance.lifetimeUsedUnits)} hint="Usage debits" />
            <BalanceCard label="Payment records" value={formatUnits(payments.length)} hint="Current payment history" />
          </section>

          <div className="mt-5"><PaymentCheckoutBanner /></div>

          {checkoutError ? <div className="mt-6 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{checkoutError}</div> : null}

          <section className="mt-8" id="packages">
            <SectionHeading title="Available packages" description="Published prepaid bundles available for Bakong KHQR checkout." />
            {packages.length === 0 ? (
              <EmptyPanel
                title="No credit packages"
                message="An administrator has not published prepaid packages yet."
              />
            ) : (
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {packages.map((pkg) => (
                  <PackageCard
                    key={pkg.id}
                    pkg={pkg}
                    choosing={choosingPackageId === pkg.id}
                    onChoose={() => void choosePackage(pkg.id)}
                  />
                ))}
              </div>
            )}
          </section>

          <section className="mt-8">
            <SectionHeading title="Payment history" description="Package checkouts and credit additions recorded for this account." />
            <PaymentsTable payments={payments} packageNameById={packageNameById} />
          </section>

          <section className="mt-8">
            <SectionHeading title="Credit ledger" description="Every credit addition, usage debit, refund, and adjustment." />
            <LedgerTable ledger={ledger} />
          </section>
        </>
      ) : null}
    </DashboardShell>
  );
}
