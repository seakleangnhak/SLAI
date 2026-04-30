"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type User } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatDateTime, truncateId } from "@/lib/format";

const primaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg bg-slate-950 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-300";

function balancePolicyDisplay(policy?: string | null) {
  if (policy === "allow_overdraft_until_sync") {
    return {
      value: "Async usage settlement during sync",
      helper: "Usage can settle after sync before the final debit appears in your ledger."
    };
  }
  return {
    value: "Standard prepaid billing",
    helper: "Credits are tracked through your ledger-backed balance."
  };
}

function DetailCard({ label, value, helper, mono = false }: { label: string; value: React.ReactNode; helper?: string; mono?: boolean }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-3 py-3 shadow-sm">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</dt>
      <dd className={cn("mt-1 text-sm font-semibold text-slate-950", mono && "font-mono")}>{value}</dd>
      {helper ? <p className="mt-1 text-xs leading-5 text-slate-500">{helper}</p> : null}
    </div>
  );
}

function RuleItem({ title, text, tone = "blue" }: { title: string; text: string; tone?: "blue" | "green" | "yellow" | "neutral" }) {
  const tones = {
    blue: "bg-blue-600",
    green: "bg-emerald-500",
    yellow: "bg-amber-500",
    neutral: "bg-slate-400"
  };

  return (
    <div className="flex gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-3">
      <span className={cn("mt-1 size-2 shrink-0 rounded-full", tones[tone])} />
      <div>
        <p className="text-sm font-semibold text-slate-950">{title}</p>
        <p className="mt-0.5 text-sm leading-6 text-slate-500">{text}</p>
      </div>
    </div>
  );
}

function AccountProfileCard({ user }: { user: User }) {
  const policy = balancePolicyDisplay(user.balancePolicy);

  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-5 items-start">
        <div>
          <CardTitle>Account profile</CardTitle>
          <CardDescription>Developer account details and access state.</CardDescription>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          <Badge dot tone={statusTone(user.status)}>{user.status}</Badge>
          <Badge dot tone={statusTone(user.role)}>{user.role}</Badge>
        </div>
      </CardHeader>

      <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
        <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">Email</p>
        <p className="mt-2 break-all text-lg font-semibold text-slate-950">{user.email}</p>
        <p className="mt-1 text-sm text-slate-500">Developer account</p>
      </div>

      <dl className="mt-4 grid gap-3 sm:grid-cols-2">
        <DetailCard label="Role" value={user.role === "ADMIN" ? "Administrator" : "User"} />
        <DetailCard label="Status" value={user.status === "ACTIVE" ? "Active" : "Suspended"} />
        <DetailCard label="Account type" value="Developer account" />
        <DetailCard label="User ID" value={truncateId(user.id, 10, 4)} mono />
        <DetailCard label="Created" value={formatDateTime(user.createdAt)} />
        <DetailCard label="Updated" value={formatDateTime(user.updatedAt)} />
        <div className="sm:col-span-2">
          <DetailCard label="Balance behavior" value={policy.value} helper={policy.helper} />
        </div>
      </dl>
    </Card>
  );
}

function SessionSecurityCard({ loggingOut, onLogout }: { loggingOut: boolean; onLogout: () => void }) {
  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-5 items-start">
        <div>
          <CardTitle>Session and security</CardTitle>
          <CardDescription>How SLAI protects account access and sensitive key material.</CardDescription>
        </div>
      </CardHeader>
      <div className="space-y-3">
        <RuleItem title="Session type" text="HttpOnly cookie session" tone="blue" />
        <RuleItem title="Token storage" text="Session tokens are not stored in localStorage or sessionStorage." tone="green" />
        <RuleItem title="API key handling" text="Raw API keys are shown only once after create or rotate." tone="yellow" />
        <RuleItem title="Billing integrity" text="Credits and usage are ledger-backed." tone="green" />
      </div>
      <Button className="mt-5 rounded-lg" type="button" onClick={onLogout} disabled={loggingOut}>
        {loggingOut ? "Signing out" : "Sign out"}
      </Button>
    </Card>
  );
}

function ProductRulesCard() {
  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-5 items-start">
        <div>
          <CardTitle>Product rules</CardTitle>
          <CardDescription>MVP account limits and billing behavior.</CardDescription>
        </div>
      </CardHeader>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
        <RuleItem title="API keys" text="1 active key per user in this MVP." tone="blue" />
        <RuleItem title="Billing" text="Manual top-up only." tone="yellow" />
        <RuleItem title="Credits" text="Credits never expire." tone="green" />
        <RuleItem title="Usage" text="Synced asynchronously from provider usage logs." tone="neutral" />
      </div>
    </Card>
  );
}

function MvpAvailabilityCard() {
  const unavailable = ["Change email", "Change password", "Profile editing", "Self-serve checkout"];

  return (
    <Card className="rounded-2xl border-slate-200 bg-slate-50 p-5">
      <CardHeader className="mb-4 items-start">
        <div>
          <CardTitle>Not available in this MVP</CardTitle>
          <CardDescription>
            These settings are not user-editable yet. SLAI currently supports session-based access, API key management, usage visibility, and ledger-backed billing.
          </CardDescription>
        </div>
      </CardHeader>
      <div className="flex flex-wrap gap-2">
        {unavailable.map((item) => (
          <span key={item} className="rounded-md bg-white px-2.5 py-1 text-xs font-semibold text-slate-600 shadow-sm ring-1 ring-slate-200">
            {item}
          </span>
        ))}
      </div>
    </Card>
  );
}

function AccountActionsCard({ loggingOut, onLogout }: { loggingOut: boolean; onLogout: () => void }) {
  return (
    <Card className="rounded-2xl p-5">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle>Account actions</CardTitle>
          <CardDescription>Need account changes? Contact an administrator.</CardDescription>
        </div>
        <button className={primaryButton} type="button" onClick={onLogout} disabled={loggingOut}>
          {loggingOut ? "Signing out" : "Sign out"}
        </button>
      </div>
    </Card>
  );
}

export default function SettingsPage() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [loggingOut, setLoggingOut] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    api.auth
      .me()
      .then((response) => setUser(response.user))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  async function logout() {
    setLoggingOut(true);
    await api.auth.logout();
    router.push("/login");
    router.refresh();
  }

  useEffect(load, []);

  return (
    <DashboardShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Settings</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Account settings</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Manage your SLAI developer account, session, and product access.</p>
        </div>
      </section>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading account" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
      </div>

      {!loading && !error && user ? (
        <>
          <section className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,0.85fr)] 2xl:grid-cols-[minmax(0,1.45fr)_minmax(380px,0.85fr)]">
            <div className="space-y-6">
              <AccountProfileCard user={user} />
              <MvpAvailabilityCard />
            </div>
            <div className="space-y-6">
              <SessionSecurityCard loggingOut={loggingOut} onLogout={logout} />
              <ProductRulesCard />
            </div>
          </section>

          <div className="mt-6">
            <AccountActionsCard loggingOut={loggingOut} onLogout={logout} />
          </div>
        </>
      ) : null}
    </DashboardShell>
  );
}
