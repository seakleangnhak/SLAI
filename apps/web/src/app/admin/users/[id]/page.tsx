"use client";

import { FormEvent, useEffect, useState } from "react";
import { useParams } from "next/navigation";

import { AdminShell } from "@/components/AdminShell";
import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { Input, Textarea } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, isNotFound, readableError, type AdminAPIKey } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

export default function AdminUserDetailPage() {
  const params = useParams<{ id: string }>();
  const userId = decodeURIComponent(params.id);
  const [apiKey, setApiKey] = useState<AdminAPIKey | null>(null);
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
    api.admin.apiKeys
      .getForUser(userId)
      .then((response) => setApiKey(response.api_key))
      .catch((err) => {
        if (isNotFound(err)) {
          setApiKey(null);
          return;
        }
        setError(readableError(err));
      })
      .finally(() => setLoading(false));
  }

  useEffect(load, [userId]);

  async function keyAction(action: "suspend" | "resume" | "revoke") {
    const confirmed = action === "resume" || window.confirm(`${action} this user's API key?`);
    if (!confirmed) {
      return;
    }
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.admin.apiKeys[action](userId);
      setApiKey(response.api_key);
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
      setNotice("Credit adjustment created.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  return (
    <AdminShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin user detail</p>
        <h1 className="mt-2 break-all text-3xl font-semibold tracking-normal text-slate-950">{userId}</h1>
      </div>

      <Card className="mt-8 border-amber-200 bg-amber-50">
        <CardTitle>Limited user detail</CardTitle>
        <CardDescription>
          The backend does not expose admin user profile or arbitrary user balance endpoints yet. Available actions use existing API key, top-up, and adjustment endpoints.
        </CardDescription>
      </Card>

      <div className="mt-6 grid gap-6 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>API key</CardTitle>
              <CardDescription>Admin metadata never includes raw keys.</CardDescription>
            </div>
            {apiKey ? <Badge tone={statusTone(apiKey.status)}>{apiKey.status}</Badge> : null}
          </CardHeader>
          {loading ? <LoadingState label="Loading API key" /> : null}
          {error ? <ErrorState message={error} onRetry={load} /> : null}
          {!loading && !error && !apiKey ? <p className="text-sm text-slate-500">No API key found for this user.</p> : null}
          {apiKey ? (
            <>
              <dl className="grid gap-4 text-sm sm:grid-cols-2">
                <div><dt className="font-medium text-slate-500">Prefix</dt><dd className="mt-1 font-mono text-slate-950">{apiKey.key_prefix}</dd></div>
                <div><dt className="font-medium text-slate-500">OmniRoute key ID</dt><dd className="mt-1 break-all text-slate-950">{apiKey.omniroute_key_id ?? "-"}</dd></div>
                <div><dt className="font-medium text-slate-500">Created</dt><dd className="mt-1 text-slate-950">{formatDateTime(apiKey.created_at)}</dd></div>
                <div><dt className="font-medium text-slate-500">Revoked</dt><dd className="mt-1 text-slate-950">{formatDateTime(apiKey.revoked_at)}</dd></div>
              </dl>
              <div className="mt-6 flex flex-wrap gap-3">
                <Button type="button" variant="secondary" disabled={actionLoading || apiKey.status === "SUSPENDED"} onClick={() => keyAction("suspend")}>Suspend</Button>
                <Button type="button" variant="secondary" disabled={actionLoading || apiKey.status === "ACTIVE"} onClick={() => keyAction("resume")}>Resume</Button>
                <Button type="button" variant="danger" disabled={actionLoading || apiKey.status === "REVOKED"} onClick={() => keyAction("revoke")}>Revoke</Button>
              </div>
            </>
          ) : null}
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
      </div>

      {notice ? <p className="mt-6 rounded-md bg-cyan-50 px-3 py-2 text-sm text-cyan-800">{notice}</p> : null}
    </AdminShell>
  );
}
