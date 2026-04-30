"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { AppShell } from "@/components/AppShell";
import { Badge } from "@/components/Badge";
import { Card, CardDescription, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, ApiError, readableError, type CreditPackage, type User } from "@/lib/api";
import { formatCompactCredits, formatCredits, formatMoney, formatUnits } from "@/lib/format";

const primaryLink =
  "inline-flex min-h-10 items-center justify-center rounded-lg bg-slate-950 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-800";
const secondaryLink =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50";

const faqs = [
  ["Do credits expire?", "No. SLAI prepaid credits do not expire."],
  ["How is usage billed?", "Usage is synced automatically and debited from your SLAI ledger balance."],
  ["Can I create multiple API keys?", "The MVP supports one active API key per user."],
  ["Can I buy directly?", "Logged-in users can choose a package and pay with Bakong KHQR. Stripe checkout is not enabled in the MVP."]
];

function PackageCard({ pkg, loggedIn, choosing, onChoose }: { pkg: CreditPackage; loggedIn: boolean; choosing: boolean; onChoose: () => void }) {
  const totalCredits = pkg.creditUnits + pkg.bonusCreditUnits;
  const primaryHref = loggedIn ? undefined : "/signup";
  const primaryLabel = loggedIn ? "Choose package" : "Create account";

  return (
    <Card className="relative overflow-hidden rounded-2xl p-5 shadow-sm transition hover:-translate-y-0.5 hover:border-blue-200 hover:shadow-lg">
      <div className="absolute right-0 top-0 size-28 translate-x-10 -translate-y-12 rounded-full bg-blue-100 blur-2xl" />
      <div className="relative flex h-full flex-col">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-slate-950">{pkg.name}</h2>
            <p className="mt-2 min-h-12 text-sm leading-6 text-slate-500">{pkg.description ?? "Prepaid SLAI credits for AI usage."}</p>
          </div>
          {pkg.bonusCreditUnits > 0 ? <Badge tone="blue">Bonus</Badge> : null}
        </div>

        <div className="mt-6">
          <p className="text-4xl font-semibold tracking-normal text-slate-950">{formatMoney(pkg.priceMinor, pkg.currency)}</p>
          <p className="mt-1 text-sm text-slate-500">Bakong KHQR checkout</p>
        </div>

        <div className="mt-6 rounded-2xl border border-slate-200 bg-slate-50 p-4">
          <p className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">Total credits</p>
          <p className="mt-2 text-3xl font-semibold text-slate-950">{formatCredits(totalCredits)}</p>
          <div className="mt-4 grid gap-2 text-sm text-slate-600">
            <div className="flex justify-between gap-4">
              <span>Included credits</span>
              <span className="font-semibold text-slate-950">{formatCredits(pkg.creditUnits)}</span>
            </div>
            {pkg.bonusCreditUnits > 0 ? (
              <div className="flex justify-between gap-4">
                <span>Bonus credits</span>
                <span className="font-semibold text-blue-700">+{formatCredits(pkg.bonusCreditUnits)}</span>
              </div>
            ) : null}
          </div>
        </div>

        <div className="mt-5 flex flex-wrap gap-2">
          <Badge tone="green" dot>Prepaid</Badge>
          <Badge tone="cyan" dot>Never expires</Badge>
        </div>

        <div className="mt-auto pt-6">
          {primaryHref ? (
            <Link href={primaryHref} className={primaryLink}>{primaryLabel}</Link>
          ) : (
            <button className={primaryLink} type="button" onClick={onChoose} disabled={choosing}>{choosing ? "Creating checkout" : primaryLabel}</button>
          )}
          <p className="mt-3 text-xs leading-5 text-slate-500">Pay with Bakong KHQR. Credits are added after payment confirmation.</p>
        </div>
      </div>
    </Card>
  );
}

function EmptyPackages({ loggedIn }: { loggedIn: boolean }) {
  return (
    <Card className="rounded-2xl border-dashed bg-white p-8 text-center shadow-sm">
      <span className="mx-auto grid size-12 place-items-center rounded-2xl bg-blue-50 text-sm font-bold text-blue-700 ring-1 ring-blue-100">S</span>
      <h2 className="mt-5 text-xl font-semibold text-slate-950">No packages available</h2>
      <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-500">An administrator has not published prepaid credit packages yet.</p>
      <div className="mt-6 flex flex-wrap justify-center gap-3">
        <Link href={loggedIn ? "/dashboard/billing" : "/signup"} className={primaryLink}>{loggedIn ? "Open billing" : "Create account"}</Link>
        <Link href="/" className={secondaryLink}>Back to home</Link>
      </div>
    </Card>
  );
}

function PaymentCheckoutCallout() {
  return (
    <Card className="rounded-2xl border-amber-200 bg-gradient-to-r from-amber-50 via-white to-blue-50 p-5 shadow-sm">
      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div className="flex gap-3">
          <span className="mt-0.5 grid size-10 shrink-0 place-items-center rounded-xl bg-amber-100 text-sm font-bold text-amber-700">M</span>
          <div>
            <CardTitle>Bakong KHQR checkout</CardTitle>
            <CardDescription className="mt-1 text-amber-800/80">
              Choose a package, scan the KHQR, and SLAI credits your balance after payment confirmation.
            </CardDescription>
          </div>
        </div>
        <span className="rounded-lg bg-white px-3 py-2 text-xs font-semibold text-slate-600 shadow-sm ring-1 ring-slate-200">Stripe checkout not enabled</span>
      </div>
    </Card>
  );
}

export default function PackagesPage() {
  const router = useRouter();
  const [packages, setPackages] = useState<CreditPackage[]>([]);
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [loadingPackages, setLoadingPackages] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [checkoutError, setCheckoutError] = useState<string | null>(null);
  const [choosingPackageId, setChoosingPackageId] = useState<string | null>(null);
  const loggedIn = currentUser !== null;

  function load() {
    setLoadingPackages(true);
    setError(null);
    api.packages
      .listPublic()
      .then((response) => setPackages(response.packages.filter((pkg) => pkg.active)))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoadingPackages(false));
  }

  useEffect(() => {
    load();
    api.auth
      .me()
      .then((response) => setCurrentUser(response.user))
      .catch((err) => {
        if (!(err instanceof ApiError) || err.status !== 401) {
          setCurrentUser(null);
        }
      });
  }, []);

  async function choosePackage(packageId: string) {
    if (!loggedIn) {
      router.push("/signup");
      return;
    }
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

  const packageTotals = useMemo(() => {
    return packages.reduce(
      (summary, pkg) => ({
        credits: summary.credits + pkg.creditUnits + pkg.bonusCreditUnits,
        bonus: summary.bonus + pkg.bonusCreditUnits
      }),
      { credits: 0, bonus: 0 }
    );
  }, [packages]);

  return (
    <AppShell section="public" user={currentUser}>
      <section className="relative overflow-hidden rounded-[2rem] border border-slate-200 bg-white p-6 shadow-sm sm:p-8 lg:p-10">
        <div className="absolute inset-0 bg-[linear-gradient(to_right,#e2e8f0_1px,transparent_1px),linear-gradient(to_bottom,#e2e8f0_1px,transparent_1px)] bg-[size:44px_44px] opacity-30" />
        <div className="absolute right-10 top-0 size-80 rounded-full bg-blue-200/45 blur-3xl" />
        <div className="relative flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-blue-700">Packages</p>
            <h1 className="mt-4 max-w-3xl text-4xl font-semibold tracking-normal text-slate-950 sm:text-5xl">Prepaid credit packages</h1>
            <p className="mt-4 max-w-2xl text-base leading-7 text-slate-600">Choose a prepaid bundle for SLAI usage. Credits never expire.</p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Link href={loggedIn ? "/dashboard" : "/login"} className={secondaryLink}>{loggedIn ? "Dashboard" : "Sign in"}</Link>
            <Link href={loggedIn ? "/dashboard/billing" : "/signup"} className={primaryLink}>{loggedIn ? "Open billing" : "Create account"}</Link>
          </div>
        </div>
      </section>

      <section className="mt-6 grid gap-4 md:grid-cols-3">
        <Card className="rounded-2xl p-5 shadow-sm">
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Published packages</p>
          <p className="mt-3 text-3xl font-semibold text-slate-950">{formatUnits(packages.length)}</p>
          <p className="mt-2 text-sm text-slate-500">Active public bundles</p>
        </Card>
        <Card className="rounded-2xl p-5 shadow-sm">
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Credits available</p>
          <p className="mt-3 text-3xl font-semibold text-slate-950">{formatCompactCredits(packageTotals.credits)}</p>
          <p className="mt-2 text-sm text-slate-500">Across active packages</p>
        </Card>
        <Card className="rounded-2xl p-5 shadow-sm">
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Bonus credits</p>
          <p className="mt-3 text-3xl font-semibold text-slate-950">{formatCompactCredits(packageTotals.bonus)}</p>
          <p className="mt-2 text-sm text-slate-500">When included by package</p>
        </Card>
      </section>

      <div className="mt-6">
        <PaymentCheckoutCallout />
      </div>

      <section className="mt-8">
        {checkoutError ? <div className="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{checkoutError}</div> : null}
        {loadingPackages ? <LoadingState label="Loading packages" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
        {!loadingPackages && !error && packages.length === 0 ? <EmptyPackages loggedIn={loggedIn} /> : null}
        {!loadingPackages && !error && packages.length > 0 ? (
          <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
            {packages.map((pkg) => <PackageCard key={pkg.id} pkg={pkg} loggedIn={loggedIn} choosing={choosingPackageId === pkg.id} onChoose={() => choosePackage(pkg.id)} />)}
          </div>
        ) : null}
      </section>

      <section className="mt-14 rounded-[2rem] border border-slate-200 bg-white p-6 shadow-sm lg:p-8">
        <div className="max-w-2xl">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-blue-700">FAQ</p>
          <h2 className="mt-3 text-3xl font-semibold tracking-normal text-slate-950">Pricing and billing basics</h2>
        </div>
        <div className="mt-8 grid gap-4 md:grid-cols-2">
          {faqs.map(([question, answer]) => (
            <div key={question} className="rounded-2xl border border-slate-200 bg-slate-50 p-5">
              <h3 className="text-base font-semibold text-slate-950">{question}</h3>
              <p className="mt-2 text-sm leading-6 text-slate-500">{answer}</p>
            </div>
          ))}
        </div>
      </section>
    </AppShell>
  );
}
