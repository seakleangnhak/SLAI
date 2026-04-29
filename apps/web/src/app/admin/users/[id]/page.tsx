"use client";

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
import { MetricCard } from "@/components/MetricCard";
import { Table, Td, Th } from "@/components/Table";
import { api, readableError, type AdminUserDetail, type UserStatus } from "@/lib/api";
import { formatDateTime, formatDelta, formatMoney, formatUnits } from "@/lib/format";

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
  const [topUpNote, setTopUpNote] = useState("Manual admin top-up");
  const [adjustDelta, setAdjustDelta] = useState("100");
  const [adjustReason, setAdjustReason] = useState("");

  function load() {
    setLoading(true);
    setError(null);
    api.admin.users
      .get(userId)
      .then((response) => setDetail(response))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, [userId]);

  async function updateUserStatus(status: UserStatus) {
    const confirmed = status === "ACTIVE" || window.confirm("Suspend this user? Their active API key will also be suspended.");
    if (!confirmed) {
      return;
    }
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.admin.users.updateStatus(userId, status);
      setDetail(response);
      setNotice(`User status updated to ${status}.`);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function keyAction(action: "suspend" | "resume" | "revoke") {
    const confirmed = action === "resume" || window.confirm(`${action} this user's API key?`);
    if (!confirmed) {
      return;
    }
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      await api.admin.apiKeys[action](userId);
      await api.admin.users.get(userId).then(setDetail);
      setNotice(`API key ${action} succeeded.`);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function submitTopUp(event: FormEvent) {
    event.preventDefault();
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      await api.admin.payments.manualTopUp({
        userId,
        packageId: null,
        amountMinor: Number(topUpAmount),
        currency: "USD",
        creditUnits: Number(topUpCredits),
        note: topUpNote,
        idempotencyKey: `ui-topup-${userId}-${Date.now()}`
      });
      await api.admin.users.get(userId).then(setDetail);
      setNotice("Manual top-up created.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function submitAdjustment(event: FormEvent) {
    event.preventDefault();
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      await api.admin.ledger.adjustment({
        userId,
        deltaUnits: Number(adjustDelta),
        reason: adjustReason,
        idempotencyKey: `ui-adjustment-${userId}-${Date.now()}`
      });
      await api.admin.users.get(userId).then(setDetail);
      setNotice("Credit adjustment created.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  return (
    <AdminShell>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin user detail</p>
          <h1 className="mt-2 break-all text-3xl font-semibold tracking-normal text-slate-950">{detail?.email ?? userId}</h1>
        </div>
        {detail ? <Badge tone={statusTone(detail.status)}>{detail.status}</Badge> : null}
      </div>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading user detail" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
        {notice ? <p className="rounded-md bg-cyan-50 px-3 py-2 text-sm text-cyan-800">{notice}</p> : null}
      </div>

      {detail ? (
        <>
          <section className="mt-8 grid gap-4 md:grid-cols-3">
            <MetricCard label="Available credits" value={formatUnits(detail.balance.available_units)} hint="Current balance" />
            <MetricCard label="Lifetime purchased" value={formatUnits(detail.balance.lifetime_purchased_units)} hint="Manual top-ups" />
            <MetricCard label="Lifetime used" value={formatUnits(detail.balance.lifetime_used_units)} hint="Usage billed" />
          </section>

          <section className="mt-8 grid gap-6 xl:grid-cols-2">
            <Card>
              <CardHeader>
                <div>
                  <CardTitle>User profile</CardTitle>
                  <CardDescription>Admin-safe fields only.</CardDescription>
                </div>
              </CardHeader>
              <dl className="grid gap-4 text-sm sm:grid-cols-2">
                <div><dt className="font-medium text-slate-500">User ID</dt><dd className="mt-1 break-all text-slate-950">{detail.id}</dd></div>
                <div><dt className="font-medium text-slate-500">Role</dt><dd className="mt-1 text-slate-950">{detail.role}</dd></div>
                <div><dt className="font-medium text-slate-500">Created</dt><dd className="mt-1 text-slate-950">{formatDateTime(detail.created_at)}</dd></div>
                <div><dt className="font-medium text-slate-500">Updated</dt><dd className="mt-1 text-slate-950">{formatDateTime(detail.updated_at)}</dd></div>
              </dl>
              <div className="mt-6 flex flex-wrap gap-3">
                <Button type="button" variant="secondary" disabled={actionLoading || detail.status === "SUSPENDED"} onClick={() => updateUserStatus("SUSPENDED")}>Suspend user</Button>
                <Button type="button" variant="secondary" disabled={actionLoading || detail.status === "ACTIVE"} onClick={() => updateUserStatus("ACTIVE")}>Activate user</Button>
              </div>
              <p className="mt-3 text-sm text-slate-500">Activating a user does not resume their API key. Use the API key resume action explicitly.</p>
            </Card>

            <Card>
              <CardHeader>
                <div>
                  <CardTitle>API key</CardTitle>
                  <CardDescription>Raw keys and hashes are never exposed.</CardDescription>
                </div>
                {detail.api_key ? <Badge tone={statusTone(detail.api_key.status)}>{detail.api_key.status}</Badge> : null}
              </CardHeader>
              {detail.api_key ? (
                <>
                  <dl className="grid gap-4 text-sm sm:grid-cols-2">
                    <div><dt className="font-medium text-slate-500">Prefix</dt><dd className="mt-1 font-mono text-slate-950">{detail.api_key.key_prefix}</dd></div>
                    <div><dt className="font-medium text-slate-500">OmniRoute key ID</dt><dd className="mt-1 break-all text-slate-950">{detail.api_key.omniroute_key_id ?? "-"}</dd></div>
                    <div><dt className="font-medium text-slate-500">Created</dt><dd className="mt-1 text-slate-950">{formatDateTime(detail.api_key.created_at)}</dd></div>
                    <div><dt className="font-medium text-slate-500">Revoked</dt><dd className="mt-1 text-slate-950">{formatDateTime(detail.api_key.revoked_at)}</dd></div>
                  </dl>
                  <div className="mt-6 flex flex-wrap gap-3">
                    <Button type="button" variant="secondary" disabled={actionLoading || detail.api_key.status === "SUSPENDED"} onClick={() => keyAction("suspend")}>Suspend key</Button>
                    <Button type="button" variant="secondary" disabled={actionLoading || detail.api_key.status === "ACTIVE"} onClick={() => keyAction("resume")}>Resume key</Button>
                    <Button type="button" variant="danger" disabled={actionLoading || detail.api_key.status === "REVOKED"} onClick={() => keyAction("revoke")}>Revoke key</Button>
                  </div>
                </>
              ) : (
                <EmptyState title="No API key" message="This user has not created an API key yet." />
              )}
            </Card>

            <Card>
              <CardHeader>
                <div>
                  <CardTitle>Manual top-up</CardTitle>
                  <CardDescription>Add credits through the admin-created manual payment flow.</CardDescription>
                </div>
              </CardHeader>
              <form className="space-y-4" onSubmit={submitTopUp}>
                <Input label="Credit units" type="number" min="1" value={topUpCredits} onChange={(event) => setTopUpCredits(event.target.value)} required />
                <Input label="Amount minor" type="number" min="0" value={topUpAmount} onChange={(event) => setTopUpAmount(event.target.value)} required />
                <Textarea label="Note" value={topUpNote} onChange={(event) => setTopUpNote(event.target.value)} />
                <Button type="submit" disabled={actionLoading}>Create top-up</Button>
              </form>
            </Card>

            <Card>
              <CardHeader>
                <div>
                  <CardTitle>Credit adjustment</CardTitle>
                  <CardDescription>Requires a reason. Negative values debit credits.</CardDescription>
                </div>
              </CardHeader>
              <form className="space-y-4" onSubmit={submitAdjustment}>
                <Input label="Delta units" type="number" value={adjustDelta} onChange={(event) => setAdjustDelta(event.target.value)} required />
                <Textarea label="Reason" value={adjustReason} onChange={(event) => setAdjustReason(event.target.value)} required />
                <Button type="submit" disabled={actionLoading}>Create adjustment</Button>
              </form>
            </Card>
          </section>

          <section className="mt-8 grid gap-6 xl:grid-cols-3">
            <RecentUsage detail={detail} />
            <RecentLedger detail={detail} />
            <RecentPayments detail={detail} />
          </section>
        </>
      ) : null}
    </AdminShell>
  );
}

function RecentUsage({ detail }: { detail: AdminUserDetail }) {
  if (detail.recent_usage.length === 0) {
    return <EmptyState title="No recent usage" message="Usage appears after OmniRoute logs are synced." />;
  }
  return (
    <div>
      <h2 className="mb-3 text-base font-semibold text-slate-950">Recent usage</h2>
      <Table>
        <thead className="bg-slate-50"><tr><Th>Model</Th><Th>Cost</Th><Th>Status</Th></tr></thead>
        <tbody className="divide-y divide-slate-100">
          {detail.recent_usage.map((event) => (
            <tr key={event.id}>
              <Td>{event.model ?? "-"}</Td>
              <Td>{formatUnits(event.cost_units)}</Td>
              <Td><Badge tone={statusTone(event.status)}>{event.status}</Badge></Td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}

function RecentLedger({ detail }: { detail: AdminUserDetail }) {
  if (detail.recent_ledger.length === 0) {
    return <EmptyState title="No recent ledger" message="Top-ups, usage, and adjustments appear here." />;
  }
  return (
    <div>
      <h2 className="mb-3 text-base font-semibold text-slate-950">Recent ledger</h2>
      <Table>
        <thead className="bg-slate-50"><tr><Th>Type</Th><Th>Delta</Th><Th>Balance</Th></tr></thead>
        <tbody className="divide-y divide-slate-100">
          {detail.recent_ledger.map((entry) => (
            <tr key={entry.id}>
              <Td>{entry.type}</Td>
              <Td>{formatDelta(entry.delta_units)}</Td>
              <Td>{formatUnits(entry.balance_after_units)}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}

function RecentPayments({ detail }: { detail: AdminUserDetail }) {
  if (detail.recent_payments.length === 0) {
    return <EmptyState title="No recent payments" message="Manual top-ups appear here." />;
  }
  return (
    <div>
      <h2 className="mb-3 text-base font-semibold text-slate-950">Recent payments</h2>
      <Table>
        <thead className="bg-slate-50"><tr><Th>Credits</Th><Th>Amount</Th><Th>Status</Th></tr></thead>
        <tbody className="divide-y divide-slate-100">
          {detail.recent_payments.map((payment) => (
            <tr key={payment.id}>
              <Td>{formatUnits(payment.credit_units)}</Td>
              <Td>{formatMoney(payment.amount_minor, payment.currency)}</Td>
              <Td><Badge tone={payment.status === "paid" ? "green" : "neutral"}>{payment.status}</Badge></Td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}
