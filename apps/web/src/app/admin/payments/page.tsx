"use client";

import { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type AdminPaymentItem } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCredits, formatDateTime, formatMoney, formatUnits, truncateId } from "@/lib/format";

const LIMIT = 50;
const secondaryButton = "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50 disabled:text-slate-400";

function statusTone(status: string) {
  if (status === "paid") return "green" as const;
  if (status === "pending_payment") return "blue" as const;
  if (status === "pending_review") return "yellow" as const;
  if (status === "expired") return "yellow" as const;
  if (status === "needs_review" || status === "rejected" || status === "cancelled") return "red" as const;
  if (status === "pending_proof") return "neutral" as const;
  return "neutral" as const;
}

export default function AdminPaymentsPage() {
  const [items, setItems] = useState<AdminPaymentItem[]>([]);
  const [status, setStatus] = useState("pending_payment");
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const [selected, setSelected] = useState<AdminPaymentItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  function load(nextOffset = offset, nextStatus = status) {
    setLoading(true);
    setError(null);
    api.admin.payments
      .list({ status: nextStatus || undefined, limit: LIMIT, offset: nextOffset })
      .then((response) => {
        setItems(response.items);
        setTotal(response.total);
        setOffset(response.offset);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => load(0, status), []);

  function applyStatus(nextStatus: string) {
    setStatus(nextStatus);
    load(0, nextStatus);
  }

  async function refreshSelected(id: string) {
    const response = await api.admin.payments.get(id);
    setSelected(response.payment);
    return response.payment;
  }

  async function openPayment(payment: AdminPaymentItem) {
    setSelected(payment);
    setActionError(null);
    setNotice(null);
    try {
      const response = await api.admin.payments.get(payment.id);
      setSelected(response.payment);
    } catch (err) {
      setActionError(readableError(err));
    }
  }

  const summary = useMemo(() => {
    const pending = items.filter((item) => item.status === "pending_payment" || item.status === "pending_review").length;
    const paid = items.filter((item) => item.status === "paid").length;
    const rejected = items.filter((item) => item.status === "rejected").length;
    const paidMinor = items.filter((item) => item.status === "paid").reduce((sum, item) => sum + item.amount_minor, 0);
    return { pending, paid, rejected, paidMinor };
  }, [items]);

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Payment review</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Payments</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Monitor Bakong KHQR checkouts, confirmed payments, and provider exceptions.</p>
          </div>
          <Button className="rounded-lg" type="button" variant="secondary" onClick={() => load(offset)} disabled={loading}>Refresh</Button>
        </div>
      </section>

      <section className="mt-6 grid gap-4 md:grid-cols-4">
        <Summary label="Pending" value={formatUnits(summary.pending)} />
        <Summary label="Paid" value={formatUnits(summary.paid)} />
        <Summary label="Rejected" value={formatUnits(summary.rejected)} />
        <Summary label="Paid amount" value={formatMoney(summary.paidMinor, "USD")} />
      </section>

      <Card className="mt-6 rounded-2xl p-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="block min-w-56">
            <span className="text-sm font-medium text-slate-700">Status</span>
            <select className="mt-2 h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm" value={status} onChange={(event) => applyStatus(event.target.value)}>
              <option value="pending_payment">Pending payment</option>
              <option value="paid">Paid</option>
              <option value="needs_review">Needs review</option>
              <option value="expired">Expired</option>
              <option value="pending_review">Legacy pending review</option>
              <option value="pending_proof">Legacy pending proof</option>
              <option value="rejected">Rejected</option>
              <option value="">All</option>
            </select>
          </label>
        </div>
      </Card>

      <div className="mt-6">
        {loading ? <LoadingState label="Loading payments" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(offset)} /> : null}
        {!loading && !error ? <PaymentsTable items={items} onSelect={openPayment} /> : null}
        {!loading && !error ? (
          <div className="mt-4 flex flex-col gap-3 rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-500 shadow-sm sm:flex-row sm:items-center sm:justify-between">
            <span>{total === 0 ? "No payments" : `Showing ${offset + 1}-${offset + items.length} of ${total}`}</span>
            <div className="flex gap-2">
              <Button type="button" variant="secondary" className="rounded-lg" disabled={offset === 0 || loading} onClick={() => load(Math.max(0, offset - LIMIT))}>Previous</Button>
              <Button type="button" variant="secondary" className="rounded-lg" disabled={offset + items.length >= total || loading} onClick={() => load(offset + LIMIT)}>Next</Button>
            </div>
          </div>
        ) : null}
      </div>

      <PaymentDrawer
        actionError={actionError}
        actionLoading={actionLoading}
        notice={notice}
        onApprove={async (payment, reference, note) => {
          setActionLoading(true); setActionError(null); setNotice(null);
          try { await api.admin.payments.approve(payment.id, reference, note); setNotice("Payment approved and credits added."); await refreshSelected(payment.id); load(offset); }
          catch (err) { setActionError(readableError(err)); }
          finally { setActionLoading(false); }
        }}
        onClose={() => { setSelected(null); setActionError(null); setNotice(null); }}
        onReject={async (payment, reason) => {
          setActionLoading(true); setActionError(null); setNotice(null);
          try { await api.admin.payments.reject(payment.id, reason); setNotice("Payment rejected."); await refreshSelected(payment.id); load(offset); }
          catch (err) { setActionError(readableError(err)); }
          finally { setActionLoading(false); }
        }}
        payment={selected}
      />
    </AdminShell>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  return <Card className="rounded-2xl p-5"><p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">{label}</p><p className="mt-3 text-3xl font-semibold text-slate-950">{value}</p><p className="mt-2 text-sm text-slate-500">Current result page</p></Card>;
}

function PaymentsTable({ items, onSelect }: { items: AdminPaymentItem[]; onSelect: (payment: AdminPaymentItem) => void }) {
  if (items.length === 0) return <Card className="rounded-2xl border-dashed p-8 text-center"><CardTitle>No payments match this view</CardTitle><CardDescription className="mt-2">Package checkouts and provider-confirmed payments will appear here.</CardDescription></Card>;
  return (
    <Card className="overflow-hidden rounded-2xl p-0">
      <div className="overflow-x-auto"><table className="min-w-full divide-y divide-slate-200 text-sm"><thead className="bg-slate-50"><tr>{["Date","User","Package","Amount","Credits","Status","Provider status","Reference","Actions"].map((h)=><th className="whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500" key={h}>{h}</th>)}</tr></thead><tbody className="divide-y divide-slate-100 bg-white">
        {items.map((item)=><tr className="hover:bg-slate-50" key={item.id}>
          <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(item.created_at)}</td>
          <td className="whitespace-nowrap px-4 py-3 text-slate-700">{item.user_email}</td>
          <td className="whitespace-nowrap px-4 py-3 font-medium text-slate-950">{item.package_name ?? "Manual package"}</td>
          <td className="whitespace-nowrap px-4 py-3 font-semibold text-slate-950">{formatMoney(item.amount_minor, item.currency)}</td>
          <td className="whitespace-nowrap px-4 py-3 text-slate-700">{formatCredits(item.credit_units)}</td>
          <td className="whitespace-nowrap px-4 py-3"><Badge dot tone={statusTone(item.status)}>{item.status}</Badge></td>
          <td className="whitespace-nowrap px-4 py-3">{item.provider_status ? <Badge dot tone={statusTone(item.status)}>{item.provider_status}</Badge> : item.proof_uploaded ? <Badge dot tone={item.duplicate_proof_count > 0 ? "yellow" : "green"}>{item.duplicate_proof_count > 0 ? "Duplicate" : "Uploaded"}</Badge> : <Badge dot tone="neutral">-</Badge>}</td>
          <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-slate-600">{truncateId(item.provider_transaction_id ?? item.admin_payment_reference ?? item.checkout_reference ?? item.provider_ref ?? item.id, 10, 4)}</td>
          <td className="whitespace-nowrap px-4 py-3"><button className="font-semibold text-blue-700 hover:text-blue-800" type="button" onClick={()=>onSelect(item)}>View</button></td>
        </tr>)}
      </tbody></table></div>
    </Card>
  );
}

function PaymentDrawer({ payment, onClose, onApprove, onReject, actionLoading, actionError, notice }: { payment: AdminPaymentItem | null; onClose: () => void; onApprove: (payment: AdminPaymentItem, reference: string, note: string) => Promise<void>; onReject: (payment: AdminPaymentItem, reason: string) => Promise<void>; actionLoading: boolean; actionError: string | null; notice: string | null }) {
  const [reference, setReference] = useState("");
  const [note, setNote] = useState("");
  const [reason, setReason] = useState("");

  useEffect(() => { setReference(""); setNote(""); setReason(""); }, [payment?.id]);
  if (!payment) return null;
  const currentPayment = payment;
  const canReview = currentPayment.status === "pending_review";
  const proof = currentPayment.proof;
  const proofUrl = api.admin.payments.proofUrl(currentPayment.id);
  const proofIsPdf = proof?.file_mime === "application/pdf";

  function approve(event: FormEvent) { event.preventDefault(); void onApprove(currentPayment, reference, note); }
  function reject(event: FormEvent) { event.preventDefault(); void onReject(currentPayment, reason); }

  return <div className="fixed inset-0 z-50 flex justify-end bg-slate-950/30 backdrop-blur-sm" onMouseDown={onClose}><aside className="h-full w-full max-w-2xl overflow-y-auto border-l border-slate-200 bg-white shadow-2xl" onMouseDown={(event)=>event.stopPropagation()}>
    <div className="sticky top-0 z-10 border-b border-slate-200 bg-white/95 px-5 py-4 backdrop-blur"><div className="flex items-start justify-between gap-4"><div><p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Payment</p><h2 className="mt-1 text-xl font-semibold text-slate-950">Payment detail</h2></div><button className={secondaryButton} type="button" onClick={onClose}>Close</button></div></div>
    <div className="space-y-5 p-5">
      {notice ? <Alert tone="green">{notice}</Alert> : null}{actionError ? <Alert tone="red">{actionError}</Alert> : null}
      {currentPayment.duplicate_proof_count > 0 ? <Alert tone="yellow">This proof file hash appears on {currentPayment.duplicate_proof_count} other payment(s). Verify carefully before approval.</Alert> : null}
      {currentPayment.status === "needs_review" ? <Alert tone="yellow">Automatic confirmation could not safely match this payment. Check the provider dashboard before taking support action.</Alert> : null}
      <div className="grid gap-3 sm:grid-cols-2"><Detail label="User" value={currentPayment.user_email}/><Detail label="Package" value={currentPayment.package_name ?? "-"}/><Detail label="Amount" value={formatMoney(currentPayment.amount_minor, currentPayment.currency)}/><Detail label="Credits" value={formatCredits(currentPayment.credit_units)}/><Detail label="Status" value={currentPayment.status}/><Detail label="Provider status" value={currentPayment.provider_status ?? "-"}/><Detail label="Checkout reference" value={currentPayment.checkout_reference ?? currentPayment.provider_ref ?? "-"} mono/><Detail label="External payment ID" value={currentPayment.external_payment_id ?? "-"} mono/><Detail label="Transaction ID" value={currentPayment.provider_transaction_id ?? "-"} mono/><Detail label="APV" value={currentPayment.provider_apv ?? "-"} mono/><Detail label="Expires" value={formatDateTime(currentPayment.expires_at)}/><Detail label="Payment ID" value={currentPayment.id} mono/>{proof ? <Detail label="User transaction ref" value={proof.user_transaction_ref ?? "-"} mono/> : null}{proof ? <Detail label="User note" value={proof.user_note ?? "-"}/> : null}</div>
      {currentPayment.proof_uploaded ? <div><h3 className="text-sm font-semibold text-slate-950">Proof</h3>{currentPayment.proof_file_sha256 ? <p className="mt-1 font-mono text-xs text-slate-500">sha256 {truncateId(currentPayment.proof_file_sha256, 12, 8)}</p> : null}{proof ? <p className="mt-1 text-xs text-slate-500">{proof.file_name} - {proof.file_mime} - {formatUnits(proof.file_size)} bytes</p> : null}<div className="mt-3 overflow-hidden rounded-2xl border border-slate-200 bg-slate-50">{proofIsPdf ? <div className="p-4"><a className={secondaryButton} href={proofUrl} target="_blank" rel="noreferrer">Open proof PDF</a></div> : <img alt="Payment proof" className="max-h-[520px] w-full object-contain" src={proofUrl} />}</div></div> : null}
      {canReview ? <form className="rounded-2xl border border-slate-200 bg-slate-50 p-4" onSubmit={approve}><CardTitle>Approve payment</CardTitle><CardDescription className="mt-1">Enter the verified Bakong payment reference. SLAI blocks duplicate references.</CardDescription><div className="mt-4 space-y-3"><Input label="Verified payment reference" value={reference} onChange={(event)=>setReference(event.target.value)} required/><Input label="Admin note optional" value={note} onChange={(event)=>setNote(event.target.value)}/><Button className="rounded-lg" type="submit" disabled={actionLoading || !reference.trim()}>{actionLoading ? "Working" : "Approve and credit"}</Button></div></form> : null}
      {(currentPayment.status === "pending_review" || currentPayment.status === "pending_proof") ? <form className="rounded-2xl border border-red-200 bg-red-50 p-4" onSubmit={reject}><CardTitle>Reject payment</CardTitle><CardDescription className="mt-1 text-red-800/80">Rejecting never credits the account.</CardDescription><div className="mt-4 space-y-3"><Input label="Rejection reason" value={reason} onChange={(event)=>setReason(event.target.value)} required/><Button className="rounded-lg" type="submit" variant="danger" disabled={actionLoading || !reason.trim()}>{actionLoading ? "Working" : "Reject"}</Button></div></form> : null}
    </div>
  </aside></div>;
}

function Detail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5"><dt className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</dt><dd className={cn("mt-1 break-words text-sm font-medium text-slate-950", mono && "font-mono")}>{value}</dd></div>;
}

function Alert({ tone, children }: { tone: "green" | "red" | "yellow"; children: ReactNode }) {
  const styles = tone === "green" ? "border-emerald-200 bg-emerald-50 text-emerald-800" : tone === "red" ? "border-red-200 bg-red-50 text-red-800" : "border-amber-200 bg-amber-50 text-amber-900";
  return <div className={`rounded-xl border px-4 py-3 text-sm leading-6 ${styles}`}>{children}</div>;
}
