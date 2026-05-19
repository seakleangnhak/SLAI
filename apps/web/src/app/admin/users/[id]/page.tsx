"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { useParams } from "next/navigation";

import { AdminShell } from "@/components/AdminShell";
import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { Input, Textarea } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type AdminUserDetail, type UserStatus } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactCredits, formatCompactUnits, formatDateTime, formatDelta, formatMoney, truncateId } from "@/lib/format";

export default function AdminUserDetailPage() {
  const params = useParams<{ id: string }>();
  const userId = decodeURIComponent(params.id);
  const [detail, setDetail] = useState<AdminUserDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [topUpCredits, setTopUpCredits] = useState("1000");
  const [topUpAmount, setTopUpAmount] = useState("1000");
  const [topUpCurrency, setTopUpCurrency] = useState("USD");
  const [topUpPackageId, setTopUpPackageId] = useState("");
  const [topUpProviderRef, setTopUpProviderRef] = useState("");
  const [topUpNote, setTopUpNote] = useState("Manual admin top-up");
  const [adjustType, setAdjustType] = useState<"credit" | "debit">("credit");
  const [adjustUnits, setAdjustUnits] = useState("100");
  const [adjustReason, setAdjustReason] = useState("");

  async function load({ silent = false }: { silent?: boolean } = {}) {
    if (!silent) {
      setLoading(true);
    }
    setError(null);
    try {
      const response = await api.admin.users.get(userId);
      setDetail(response);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, [userId]);

  async function updateUserStatus(status: UserStatus) {
    const confirmed = status === "SUSPENDED"
      ? window.confirm("Suspend this user? Their active API key will also be suspended.")
      : window.confirm("Activate this user? Their API key will not be resumed automatically.");
    if (!confirmed) {
      return;
    }
    await runAction(async () => {
      const response = await api.admin.users.updateStatus(userId, status);
      setDetail(response);
      setNotice(`User status updated to ${status}.`);
    });
  }

  async function keyAction(action: "suspend" | "resume" | "revoke") {
    const confirmed = action === "resume" || window.confirm(`${action} this user's API key?`);
    if (!confirmed) {
      return;
    }
    await runAction(async () => {
      await api.admin.apiKeys[action](userId);
      await load({ silent: true });
      setNotice(`API key ${action} succeeded.`);
    });
  }

  async function submitTopUp(event: FormEvent) {
    event.preventDefault();
    await runAction(async () => {
      await api.admin.payments.manualTopUp({
        userId,
        packageId: topUpPackageId.trim() || null,
        amountMinor: Number(topUpAmount),
        currency: topUpCurrency.trim().toUpperCase() || "USD",
        creditUnits: Number(topUpCredits),
        note: topUpNote,
        idempotencyKey: topUpProviderRef.trim() || `ui-topup-${userId}-${Date.now()}`
      });
      await load({ silent: true });
      setNotice("Manual top-up created.");
    });
  }

  async function submitAdjustment(event: FormEvent) {
    event.preventDefault();
    const amount = Math.abs(Number(adjustUnits));
    const deltaUnits = adjustType === "debit" ? -amount : amount;
    if (deltaUnits < 0 && !window.confirm("Create a debit adjustment for this user?")) {
      return;
    }
    await runAction(async () => {
      await api.admin.ledger.adjustment({
        userId,
        deltaUnits,
        reason: adjustReason,
        idempotencyKey: `ui-adjustment-${userId}-${Date.now()}`
      });
      await load({ silent: true });
      setNotice("Credit adjustment created.");
    });
  }

  async function runAction(action: () => Promise<void>) {
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      await action();
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  return (
    <AdminShell>
      <div className="mb-6">
        <Link className="inline-flex items-center text-sm font-semibold text-blue-700 hover:text-blue-800" href="/admin/users">Back to users</Link>
      </div>

      {loading ? <LoadingState label="Loading user detail" /> : null}
      {error ? <div className="mb-4"><ErrorState message={error} onRetry={() => load()} /></div> : null}
      {notice ? <p className="mb-4 rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-sm text-blue-800">{notice}</p> : null}

      {detail ? (
        <>
          <header className="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
            <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge dot tone={statusTone(detail.status)}>{detail.status}</Badge>
                  <Badge dot tone={statusTone(detail.role)}>{detail.role}</Badge>
                  <Badge tone="neutral">{detail.auth_provider === "google" ? "Google sign-in" : "Password sign-in"}</Badge>
                </div>
                <h1 className="mt-3 break-words text-3xl font-semibold tracking-[-0.02em] text-slate-950">{detail.email}</h1>
                <p className="mt-2 font-mono text-xs text-slate-400">User ID {truncateId(detail.id, 12, 8)}</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="secondary" disabled={actionLoading || detail.status === "SUSPENDED"} onClick={() => updateUserStatus("SUSPENDED")}>Suspend user</Button>
                <Button type="button" variant="secondary" disabled={actionLoading || detail.status === "ACTIVE"} onClick={() => updateUserStatus("ACTIVE")}>Activate user</Button>
              </div>
            </div>
          </header>

          <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
            <main className="space-y-6">
              <BalanceCard detail={detail} />
              <RecentUsage detail={detail} />
              <RecentLedger detail={detail} />
              <RecentPayments detail={detail} />
            </main>

            <aside className="space-y-6">
              <APIKeyCard detail={detail} actionLoading={actionLoading} onAction={keyAction} />
              <TopUpCard
                actionLoading={actionLoading}
                amount={topUpAmount}
                credits={topUpCredits}
                currency={topUpCurrency}
                note={topUpNote}
                packageId={topUpPackageId}
                providerRef={topUpProviderRef}
                setAmount={setTopUpAmount}
                setCredits={setTopUpCredits}
                setCurrency={setTopUpCurrency}
                setNote={setTopUpNote}
                setPackageId={setTopUpPackageId}
                setProviderRef={setTopUpProviderRef}
                onSubmit={submitTopUp}
              />
              <AdjustmentCard
                actionLoading={actionLoading}
                adjustType={adjustType}
                reason={adjustReason}
                units={adjustUnits}
                setAdjustType={setAdjustType}
                setReason={setAdjustReason}
                setUnits={setAdjustUnits}
                onSubmit={submitAdjustment}
              />
            </aside>
          </div>
        </>
      ) : null}
    </AdminShell>
  );
}

function BalanceCard({ detail }: { detail: AdminUserDetail }) {
  const purchased = detail.balance.lifetime_purchased_units;
  const used = detail.balance.lifetime_used_units;
  const usedPercent = purchased > 0 ? Math.min(100, Math.round((used / purchased) * 100)) : 0;

  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Balance</CardTitle>
          <CardDescription>Credit wallet state from the ledger-backed balance row.</CardDescription>
        </div>
        <span className="rounded-md bg-slate-100 px-2 py-1 font-mono text-xs text-slate-600">v{detail.balance.version}</span>
      </CardHeader>
      <div className="grid gap-3 md:grid-cols-3">
        <BalanceMetric label="Available" value={formatCompactCredits(detail.balance.available_units)} tone="blue" />
        <BalanceMetric label="Purchased" value={formatCompactCredits(purchased)} tone="green" />
        <BalanceMetric label="Used" value={formatCompactCredits(used)} tone="red" />
      </div>
      {purchased > 0 ? (
        <div className="mt-4">
          <div className="mb-2 flex justify-between text-xs text-slate-500"><span>Lifetime usage</span><span>{usedPercent}%</span></div>
          <div className="h-1.5 overflow-hidden rounded-full bg-slate-100"><div className="h-full rounded-full bg-blue-600" style={{ width: `${usedPercent}%` }} /></div>
        </div>
      ) : null}
      <p className="mt-4 text-xs text-slate-500">Updated {formatDateTime(detail.balance.updated_at)}</p>
    </Card>
  );
}

function BalanceMetric({ label, value, tone }: { label: string; value: string; tone: "blue" | "green" | "red" }) {
  const toneClass = {
    blue: "bg-blue-50 text-blue-700",
    green: "bg-emerald-50 text-emerald-700",
    red: "bg-red-50 text-red-700"
  }[tone];

  return (
    <div className="rounded-lg border border-slate-200 bg-slate-50 p-3">
      <p className="text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">{label}</p>
      <p className="mt-2 text-2xl font-semibold tracking-[-0.02em] text-slate-950">{value}</p>
      <span className={cn("mt-3 inline-flex rounded-md px-2 py-1 text-[11px] font-semibold", toneClass)}>credits</span>
    </div>
  );
}

function APIKeyCard({ detail, actionLoading, onAction }: { detail: AdminUserDetail; actionLoading: boolean; onAction: (action: "suspend" | "resume" | "revoke") => void }) {
  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>API Key</CardTitle>
          <CardDescription>Metadata only. Raw keys and hashes are never exposed.</CardDescription>
        </div>
        {detail.api_key ? <Badge dot tone={statusTone(detail.api_key.status)}>{detail.api_key.status}</Badge> : null}
      </CardHeader>
      {detail.api_key ? (
        <>
          <dl className="space-y-3 text-sm">
            <DetailRow label="Prefix" value={detail.api_key.key_prefix} mono />
            <DetailRow label="OmniRoute" value={detail.api_key.omniroute_key_id ? "Linked" : "Not linked"} />
            <DetailRow label="OmniRoute ID" value={truncateId(detail.api_key.omniroute_key_id, 10, 6)} mono />
            <DetailRow label="Created" value={formatDateTime(detail.api_key.created_at)} />
            <DetailRow label="Last used" value={formatDateTime(detail.api_key.last_used_at)} />
            <DetailRow label="Revoked" value={formatDateTime(detail.api_key.revoked_at)} />
          </dl>
          <div className="mt-5 grid gap-2">
            <Button type="button" variant="secondary" disabled={actionLoading || detail.api_key.status === "SUSPENDED"} onClick={() => onAction("suspend")}>Suspend key</Button>
            <Button type="button" variant="secondary" disabled={actionLoading || detail.api_key.status === "ACTIVE" || detail.api_key.status === "REVOKED"} onClick={() => onAction("resume")}>Resume key</Button>
            <Button type="button" variant="danger" disabled={actionLoading || detail.api_key.status === "REVOKED"} onClick={() => onAction("revoke")}>Revoke key</Button>
          </div>
        </>
      ) : <EmptyState title="No API key" message="This user has not created an API key yet." />}
    </Card>
  );
}

function TopUpCard({
  actionLoading,
  amount,
  credits,
  currency,
  note,
  packageId,
  providerRef,
  setAmount,
  setCredits,
  setCurrency,
  setNote,
  setPackageId,
  setProviderRef,
  onSubmit
}: {
  actionLoading: boolean;
  amount: string;
  credits: string;
  currency: string;
  note: string;
  packageId: string;
  providerRef: string;
  setAmount: (value: string) => void;
  setCredits: (value: string) => void;
  setCurrency: (value: string) => void;
  setNote: (value: string) => void;
  setPackageId: (value: string) => void;
  setProviderRef: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
}) {
  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Manual Top-up</CardTitle>
          <CardDescription>Create an admin payment and credit ledger entry.</CardDescription>
        </div>
      </CardHeader>
      <form className="space-y-3" onSubmit={onSubmit}>
        <Input label="Credits" type="number" min="0.000001" step="0.000001" value={credits} onChange={(event) => setCredits(event.target.value)} required />
        <div className="grid grid-cols-[1fr_96px] gap-3">
          <Input label="Amount minor" type="number" min="0" value={amount} onChange={(event) => setAmount(event.target.value)} required />
          <Input label="Currency" value={currency} onChange={(event) => setCurrency(event.target.value)} required />
        </div>
        <Input label="Package ID" value={packageId} onChange={(event) => setPackageId(event.target.value)} hint="Optional package UUID." />
        <Input label="Provider ref" value={providerRef} onChange={(event) => setProviderRef(event.target.value)} hint="Optional. Stored as the manual provider reference/idempotency key." />
        <Textarea label="Note" value={note} onChange={(event) => setNote(event.target.value)} />
        <Button className="w-full" type="submit" disabled={actionLoading}>{actionLoading ? "Submitting" : "Create top-up"}</Button>
      </form>
    </Card>
  );
}

function AdjustmentCard({
  actionLoading,
  adjustType,
  reason,
  units,
  setAdjustType,
  setReason,
  setUnits,
  onSubmit
}: {
  actionLoading: boolean;
  adjustType: "credit" | "debit";
  reason: string;
  units: string;
  setAdjustType: (value: "credit" | "debit") => void;
  setReason: (value: string) => void;
  setUnits: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
}) {
  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Credit Adjustment</CardTitle>
          <CardDescription>Credit or debit with a required audit reason.</CardDescription>
        </div>
      </CardHeader>
      <form className="space-y-3" onSubmit={onSubmit}>
        <div>
          <span className="text-sm font-medium text-slate-700">Type</span>
          <div className="mt-2 grid grid-cols-2 rounded-md border border-slate-200 bg-slate-100 p-1">
            {(["credit", "debit"] as const).map((type) => (
              <button className={cn("rounded px-3 py-2 text-sm font-semibold capitalize", adjustType === type ? "bg-white text-slate-950 shadow-sm" : "text-slate-500")} key={type} onClick={() => setAdjustType(type)} type="button">
                {type}
              </button>
            ))}
          </div>
        </div>
        <Input label="Credits" type="number" min="0.000001" step="0.000001" value={units} onChange={(event) => setUnits(event.target.value)} required />
        <Textarea label="Reason" value={reason} onChange={(event) => setReason(event.target.value)} required />
        <Button className="w-full" type="submit" variant={adjustType === "debit" ? "danger" : "primary"} disabled={actionLoading || !reason.trim()}>{actionLoading ? "Submitting" : "Create adjustment"}</Button>
      </form>
    </Card>
  );
}

function RecentUsage({ detail }: { detail: AdminUserDetail }) {
  return (
    <DataCard title="Recent Usage" description="Latest usage events for this user.">
      {detail.recent_usage.length === 0 ? <EmptyState title="No recent usage" message="Usage appears after OmniRoute logs are synced." /> : (
        <CompactTable headers={["Model", "Provider", "Tokens", "Cost", "Status", "Occurred"]}>
          {detail.recent_usage.map((event) => (
            <tr className="hover:bg-slate-50" key={event.id}>
              <DenseTd>{event.model ?? "-"}</DenseTd>
              <DenseTd>{event.provider ?? "-"}</DenseTd>
              <DenseTd mono>{formatCompactUnits(event.total_tokens)}</DenseTd>
              <DenseTd mono>{formatCompactCredits(event.cost_units)}</DenseTd>
              <DenseTd><Badge dot tone={statusTone(event.status)}>{event.status}</Badge></DenseTd>
              <DenseTd>{formatDateTime(event.occurred_at)}</DenseTd>
            </tr>
          ))}
        </CompactTable>
      )}
    </DataCard>
  );
}

function RecentLedger({ detail }: { detail: AdminUserDetail }) {
  return (
    <DataCard title="Recent Ledger" description="Ledger-backed balance mutations.">
      {detail.recent_ledger.length === 0 ? <EmptyState title="No recent ledger" message="Top-ups, usage, and adjustments appear here." /> : (
        <CompactTable headers={["Type", "Delta", "Balance", "Reason", "Created"]}>
          {detail.recent_ledger.map((entry) => (
            <tr className="hover:bg-slate-50" key={entry.id}>
              <DenseTd>{entry.type}</DenseTd>
              <DenseTd mono><span className={entry.delta_units < 0 ? "text-red-700" : "text-emerald-700"}>{formatDelta(entry.delta_units)}</span></DenseTd>
              <DenseTd mono>{formatCompactCredits(entry.balance_after_units)}</DenseTd>
              <DenseTd>{entry.reason ?? "-"}</DenseTd>
              <DenseTd>{formatDateTime(entry.created_at)}</DenseTd>
            </tr>
          ))}
        </CompactTable>
      )}
    </DataCard>
  );
}

function RecentPayments({ detail }: { detail: AdminUserDetail }) {
  return (
    <DataCard title="Recent Payments" description="Latest manual top-ups and payment rows.">
      {detail.recent_payments.length === 0 ? <EmptyState title="No recent payments" message="Manual top-ups appear here." /> : (
        <CompactTable headers={["Amount", "Credits", "Status", "Admin", "Created"]}>
          {detail.recent_payments.map((payment) => (
            <tr className="hover:bg-slate-50" key={payment.id}>
              <DenseTd mono>{formatMoney(payment.amount_minor, payment.currency)}</DenseTd>
              <DenseTd mono>{formatCompactCredits(payment.credit_units)}</DenseTd>
              <DenseTd><Badge dot tone={payment.status === "paid" ? "green" : "neutral"}>{payment.status}</Badge></DenseTd>
              <DenseTd mono>{truncateId(payment.admin_id, 8, 4)}</DenseTd>
              <DenseTd>{formatDateTime(payment.created_at)}</DenseTd>
            </tr>
          ))}
        </CompactTable>
      )}
    </DataCard>
  );
}

function DataCard({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return (
    <Card className="p-4">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
      </CardHeader>
      {children}
    </Card>
  );
}

function CompactTable({ headers, children }: { headers: string[]; children: React.ReactNode }) {
  return (
    <div className="overflow-hidden rounded-lg border border-slate-200">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50"><tr>{headers.map((header) => <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500" key={header}>{header}</th>)}</tr></thead>
          <tbody className="divide-y divide-slate-100">{children}</tbody>
        </table>
      </div>
    </div>
  );
}

function DenseTd({ children, mono = false }: { children: React.ReactNode; mono?: boolean }) {
  return <td className={cn("whitespace-nowrap px-4 py-3 text-slate-700", mono && "font-mono text-slate-900")}>{children}</td>;
}

function DetailRow({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-slate-100 pb-3 last:border-0 last:pb-0">
      <dt className="text-slate-500">{label}</dt>
      <dd className={cn("break-all text-right font-medium text-slate-950", mono && "font-mono text-xs")}>{value}</dd>
    </div>
  );
}
