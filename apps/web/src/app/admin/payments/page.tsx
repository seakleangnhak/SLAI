"use client";

import Link from "next/link";
import { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { ApiError, api, readableError, type AdminPaymentFilter, type AdminPaymentItem } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCredits, formatDateTime, formatMoney, formatUnits, truncateId } from "@/lib/format";

const LIMIT = 50;
const defaultFilters = { status: "", userId: "", provider: "", from: "", to: "" };
const secondaryLink =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50";

type FilterState = typeof defaultFilters;
type BadgeTone = "neutral" | "green" | "red" | "yellow" | "cyan" | "blue" | "purple";

function statusTone(status: string): BadgeTone {
  if (status === "paid") return "green";
  if (status === "pending_review") return "yellow";
  if (status === "pending_payment") return "blue";
  if (status === "pending_proof") return "blue";
  if (status === "rejected") return "red";
  if (status === "cancelled") return "neutral";
  if (status === "expired") return "yellow";
  if (status === "needs_review") return "red";
  return "neutral";
}

function proofTone(payment: AdminPaymentItem): BadgeTone {
  if (payment.duplicate_proof_count > 0) return "yellow";
  if (payment.proof_uploaded) return "green";
  return "neutral";
}

function statusLabel(status: string) {
  return status.replaceAll("_", " ");
}

function formatProvider(provider: string) {
  if (provider === "bakong_khqr") return "Bakong KHQR";
  if (provider === "manual") return "Manual";
  return provider || "-";
}

function toAPIISO(value: string) {
  if (!value) return undefined;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return undefined;
  return date.toISOString();
}

function toAPIFilter(filters: FilterState, offset: number): AdminPaymentFilter {
  return {
    status: filters.status || undefined,
    user_id: filters.userId.trim() || undefined,
    provider: filters.provider.trim() || undefined,
    from: toAPIISO(filters.from),
    to: toAPIISO(filters.to),
    limit: LIMIT,
    offset
  };
}

function validateDateRange(filters: FilterState) {
  if (!filters.from || !filters.to) return null;
  const from = new Date(filters.from).getTime();
  const to = new Date(filters.to).getTime();
  if (Number.isNaN(from) || Number.isNaN(to)) return "Enter valid date filters.";
  if (from > to) return "From date must be before To date.";
  return null;
}

export default function AdminPaymentsPage() {
  const [items, setItems] = useState<AdminPaymentItem[]>([]);
  const [draftFilters, setDraftFilters] = useState<FilterState>(defaultFilters);
  const [appliedFilters, setAppliedFilters] = useState<FilterState>(defaultFilters);
  const [filterError, setFilterError] = useState<string | null>(null);
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const [selected, setSelected] = useState<AdminPaymentItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  function load(nextOffset = offset, filters = appliedFilters) {
    const validation = validateDateRange(filters);
    if (validation) {
      setFilterError(validation);
      return;
    }
    setFilterError(null);
    setLoading(true);
    setError(null);
    api.admin.payments
      .list(toAPIFilter(filters, nextOffset))
      .then((response) => {
        setItems(response.items);
        setTotal(response.total);
        setOffset(response.offset);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => load(0, defaultFilters), []);

  function applyFilters(event?: FormEvent) {
    event?.preventDefault();
    const validation = validateDateRange(draftFilters);
    if (validation) {
      setFilterError(validation);
      return;
    }
    setAppliedFilters(draftFilters);
    load(0, draftFilters);
  }

  function quickFilter(status: string) {
    const next = { ...draftFilters, status };
    setDraftFilters(next);
    setAppliedFilters(next);
    load(0, next);
  }

  function clearFilters() {
    setDraftFilters(defaultFilters);
    setAppliedFilters(defaultFilters);
    load(0, defaultFilters);
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
    const pending = items.filter((item) => item.status === "pending_review" || item.status === "needs_review" || item.status === "pending_payment").length;
    const paid = items.filter((item) => item.status === "paid").length;
    const rejected = items.filter((item) => item.status === "rejected").length;
    const paidMinor = items.filter((item) => item.status === "paid").reduce((sum, item) => sum + item.amount_minor, 0);
    return { pending, paid, rejected, paidMinor };
  }, [items]);

  const pendingReviewCount = items.filter((item) => item.status === "pending_review" || item.status === "needs_review").length;

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Payment review</p>
              {pendingReviewCount > 0 ? <Badge dot tone="yellow">{formatUnits(pendingReviewCount)} need review</Badge> : null}
            </div>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Payments</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Monitor Bakong KHQR checkouts, proof uploads, and admin-reviewed credits.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button className="rounded-lg" type="button" variant="secondary" onClick={() => load(offset)} disabled={loading}>Refresh</Button>
            <Link className={secondaryLink} href="/admin/settings/payments">Payment settings</Link>
          </div>
        </div>
      </section>

      <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard label="Pending review" value={formatUnits(summary.pending)} hint="Current result page" tone="yellow" />
        <SummaryCard label="Paid" value={formatUnits(summary.paid)} hint="Current result page" tone="green" />
        <SummaryCard label="Rejected" value={formatUnits(summary.rejected)} hint="Current result page" tone="red" />
        <SummaryCard label="Paid amount" value={formatMoney(summary.paidMinor, "USD")} hint="Current result page" tone="blue" />
      </section>

      <Card className="mt-5 rounded-2xl border-amber-200 bg-gradient-to-r from-amber-50 via-white to-blue-50 p-4 shadow-sm">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex gap-3">
            <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-full bg-amber-100 text-sm font-bold text-amber-700">V</span>
            <div>
              <CardTitle>Manual verification required</CardTitle>
              <CardDescription className="mt-1 text-amber-900/80">Verify the payment in Bakong before approving. Enter the verified payment reference to prevent duplicate or reused proofs.</CardDescription>
            </div>
          </div>
          <Link className={secondaryLink} href="/admin/settings/payments">Payment settings</Link>
        </div>
      </Card>

      <FilterBar
        error={filterError}
        filters={draftFilters}
        loading={loading}
        onApply={applyFilters}
        onChange={setDraftFilters}
        onClear={clearFilters}
        onQuickFilter={quickFilter}
      />

      <div className="mt-6">
        {loading ? <LoadingState label="Loading payments" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(offset)} /> : null}
        {!loading && !error ? <PaymentsTable filtersActive={JSON.stringify(appliedFilters) !== JSON.stringify(defaultFilters)} items={items} onClear={clearFilters} onSelect={openPayment} /> : null}
        {!loading && !error ? (
          <div className="mt-4 flex flex-col gap-3 rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-500 shadow-sm sm:flex-row sm:items-center sm:justify-between">
            <span>{total === 0 ? "No payments" : "Showing " + (offset + 1) + "-" + (offset + items.length) + " of " + total}</span>
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
        onApprove={async (currentPayment, reference, note) => {
          setActionLoading(true);
          setActionError(null);
          setNotice(null);
          try {
            await api.admin.payments.approve(currentPayment.id, reference, note);
            setNotice("Payment approved and credits added.");
            await refreshSelected(currentPayment.id);
            load(offset);
          } catch (err) {
            if (err instanceof ApiError && err.code === "duplicate_payment_reference") {
              setActionError("This verified payment reference has already been used.");
            } else {
              setActionError(readableError(err));
            }
          } finally {
            setActionLoading(false);
          }
        }}
        onClose={() => { setSelected(null); setActionError(null); setNotice(null); }}
        onReject={async (currentPayment, reason) => {
          if (!window.confirm("Reject this payment? Credits will not be added.")) return;
          setActionLoading(true);
          setActionError(null);
          setNotice(null);
          try {
            await api.admin.payments.reject(currentPayment.id, reason);
            setNotice("Payment rejected.");
            await refreshSelected(currentPayment.id);
            load(offset);
          } catch (err) {
            setActionError(readableError(err));
          } finally {
            setActionLoading(false);
          }
        }}
        payment={selected}
      />
    </AdminShell>
  );
}

function SummaryCard({ label, value, hint, tone }: { label: string; value: string; hint: string; tone: BadgeTone }) {
  const accents: Record<BadgeTone, string> = {
    neutral: "border-slate-200",
    green: "border-emerald-200 bg-gradient-to-br from-white to-emerald-50/60",
    red: "border-red-200 bg-gradient-to-br from-white to-red-50/60",
    yellow: "border-amber-200 bg-gradient-to-br from-white to-amber-50/70",
    cyan: "border-cyan-200 bg-gradient-to-br from-white to-cyan-50/60",
    blue: "border-blue-200 bg-gradient-to-br from-white to-blue-50/60",
    purple: "border-violet-200 bg-gradient-to-br from-white to-violet-50/60"
  };
  return (
    <Card className={cn("rounded-2xl p-5 shadow-sm", accents[tone])}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">{label}</p>
          <p className="mt-3 text-3xl font-semibold text-slate-950">{value}</p>
          <p className="mt-2 text-sm text-slate-500">{hint}</p>
        </div>
        <Badge dot tone={tone}>Live</Badge>
      </div>
    </Card>
  );
}

function FilterBar({ filters, error, loading, onApply, onChange, onClear, onQuickFilter }: { filters: FilterState; error: string | null; loading: boolean; onApply: (event?: FormEvent) => void; onChange: (filters: FilterState) => void; onClear: () => void; onQuickFilter: (status: string) => void }) {
  return (
    <Card className="mt-6 rounded-2xl p-4 shadow-sm">
      <form className="space-y-4" onSubmit={onApply}>
        <div className="flex flex-wrap gap-2">
          <button className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs font-semibold text-amber-800 transition hover:bg-amber-100" type="button" onClick={() => onQuickFilter("review_queue")}>Review queue</button>
          <button className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs font-semibold text-red-800 transition hover:bg-red-100" type="button" onClick={() => onQuickFilter("needs_review")}>Needs review</button>
          <button className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs font-semibold text-slate-700 transition hover:bg-slate-50" type="button" onClick={() => onQuickFilter("pending_review")}>Pending review</button>
          <button className="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs font-semibold text-emerald-800 transition hover:bg-emerald-100" type="button" onClick={() => onQuickFilter("paid")}>Paid</button>
          <button className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs font-semibold text-red-800 transition hover:bg-red-100" type="button" onClick={() => onQuickFilter("rejected")}>Rejected</button>
        </div>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-6">
          <label className="block">
            <span className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">Status</span>
            <select className="mt-2 h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-800 outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4" value={filters.status} onChange={(event) => onChange({ ...filters, status: event.target.value })}>
              <option value="">All</option>
              <option value="review_queue">Review queue</option>
              <option value="needs_review">Needs review</option>
              <option value="pending_proof">Pending proof</option>
              <option value="pending_review">Pending review</option>
              <option value="pending_payment">Pending payment</option>
              <option value="paid">Paid</option>
              <option value="rejected">Rejected</option>
              <option value="cancelled">Cancelled</option>
            </select>
          </label>
          <label className="block">
            <span className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">User ID</span>
            <input className="mt-2 h-10 w-full rounded-lg border border-slate-300 px-3 text-sm outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4" value={filters.userId} onChange={(event) => onChange({ ...filters, userId: event.target.value })} placeholder="User UUID" />
          </label>
          <label className="block">
            <span className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">Provider</span>
            <input className="mt-2 h-10 w-full rounded-lg border border-slate-300 px-3 text-sm outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4" value={filters.provider} onChange={(event) => onChange({ ...filters, provider: event.target.value })} placeholder="bakong_khqr" />
          </label>
          <label className="block">
            <span className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">From</span>
            <input className="mt-2 h-10 w-full rounded-lg border border-slate-300 px-3 text-sm outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4" type="datetime-local" value={filters.from} onChange={(event) => onChange({ ...filters, from: event.target.value })} />
          </label>
          <label className="block">
            <span className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">To</span>
            <input className="mt-2 h-10 w-full rounded-lg border border-slate-300 px-3 text-sm outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4" type="datetime-local" value={filters.to} onChange={(event) => onChange({ ...filters, to: event.target.value })} />
          </label>
          <div className="flex items-end gap-2">
            <Button className="flex-1 rounded-lg" type="submit" disabled={loading}>Apply</Button>
            <Button className="rounded-lg" type="button" variant="secondary" onClick={onClear} disabled={loading}>Clear</Button>
          </div>
        </div>
        {error ? <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}
      </form>
    </Card>
  );
}

function PaymentsTable({ items, filtersActive, onClear, onSelect }: { items: AdminPaymentItem[]; filtersActive: boolean; onClear: () => void; onSelect: (payment: AdminPaymentItem) => void }) {
  if (items.length === 0) {
    return (
      <Card className="rounded-2xl border-dashed p-8 text-center">
        <span className="mx-auto grid size-10 place-items-center rounded-full bg-blue-50 text-sm font-bold text-blue-700 ring-1 ring-blue-100">P</span>
        <CardTitle className="mt-4">{filtersActive ? "No payments match these filters" : "No payments"}</CardTitle>
        <CardDescription className="mx-auto mt-2 max-w-lg">{filtersActive ? "Try clearing filters or expanding the date range." : "Bakong KHQR checkout submissions will appear here."}</CardDescription>
        <div className="mt-5 flex justify-center gap-2">
          {filtersActive ? <Button className="rounded-lg" type="button" variant="secondary" onClick={onClear}>Clear filters</Button> : <Link className={secondaryLink} href="/admin/settings/payments">Open payment settings</Link>}
        </div>
      </Card>
    );
  }

  return (
    <Card className="overflow-hidden rounded-2xl p-0 shadow-sm">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50">
            <tr>{["Date", "User", "Package", "Amount", "Credits", "Status", "Proof", "Verified reference", "Actions"].map((heading) => <th className="whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500" key={heading}>{heading}</th>)}</tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {items.map((item) => {
              const reviewLabel = item.status === "pending_review" || item.status === "needs_review" ? "Review" : "View";
              return (
                <tr className="hover:bg-slate-50" key={item.id}>
                  <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(item.created_at)}</td>
                  <td className="px-4 py-3">
                    <p className="whitespace-nowrap font-medium text-slate-900">{item.user_email}</p>
                    <p className="mt-1 font-mono text-xs text-slate-500">{truncateId(item.user_id, 8, 4)}</p>
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 font-medium text-slate-950">{item.package_name ?? "Manual package"}</td>
                  <td className="whitespace-nowrap px-4 py-3 font-semibold text-slate-950">{formatMoney(item.amount_minor, item.currency)}</td>
                  <td className="whitespace-nowrap px-4 py-3 text-slate-700">{formatCredits(item.credit_units)}</td>
                  <td className="whitespace-nowrap px-4 py-3">
                    <div className="flex flex-col items-start gap-1">
                      <Badge dot tone={statusTone(item.status)}>{statusLabel(item.status)}</Badge>
                      {item.provider_status ? <span className="text-xs text-slate-400">Provider: {item.provider_status}</span> : null}
                    </div>
                  </td>
                  <td className="whitespace-nowrap px-4 py-3"><Badge dot tone={proofTone(item)}>{item.proof_uploaded ? (item.duplicate_proof_count > 0 ? "Duplicate" : "Uploaded") : "Missing"}</Badge></td>
                  <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-slate-600">{truncateId(item.admin_payment_reference ?? item.provider_transaction_id ?? item.checkout_reference ?? item.provider_ref ?? null, 10, 4)}</td>
                  <td className="whitespace-nowrap px-4 py-3"><button className="font-semibold text-blue-700 hover:text-blue-800" type="button" onClick={() => onSelect(item)}>{reviewLabel}</button></td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
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

  const proof = currentPayment.proof;
  const proofUrl = api.admin.payments.proofUrl(currentPayment.id);
  const proofIsPdf = proof?.file_mime === "application/pdf";
  const proofIsImage = Boolean(proof?.file_mime?.startsWith("image/"));
  const canApprove = currentPayment.status === "pending_review" || currentPayment.status === "needs_review";
  const canReject = currentPayment.status === "pending_review" || currentPayment.status === "pending_proof" || currentPayment.status === "needs_review";

  function approve(event: FormEvent) {
    event.preventDefault();
    void onApprove(currentPayment, reference, note);
  }

  function reject(event: FormEvent) {
    event.preventDefault();
    void onReject(currentPayment, reason);
  }

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-slate-950/30 backdrop-blur-sm" onMouseDown={onClose}>
      <aside className="h-full w-full max-w-3xl overflow-y-auto border-l border-slate-200 bg-white shadow-2xl" onMouseDown={(event) => event.stopPropagation()}>
        <div className="sticky top-0 z-10 border-b border-slate-200 bg-white/95 px-5 py-4 backdrop-blur">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Payment review</p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <h2 className="text-xl font-semibold text-slate-950">Payment review</h2>
                <Badge dot tone={statusTone(currentPayment.status)}>{statusLabel(currentPayment.status)}</Badge>
              </div>
              <p className="mt-1 font-mono text-xs text-slate-500">{truncateId(currentPayment.id, 12, 8)}</p>
            </div>
            <button className={secondaryLink} type="button" onClick={onClose}>Close</button>
          </div>
        </div>

        <div className="space-y-5 p-5">
          {notice ? <Alert tone="green">{notice}</Alert> : null}
          {actionError ? <Alert tone="red">{actionError}</Alert> : null}
          {currentPayment.duplicate_proof_count > 0 ? <Alert tone="yellow">This proof file hash appears on another payment. Verify carefully before approval.</Alert> : null}
          {currentPayment.status === "needs_review" ? <Alert tone="yellow">This payment was flagged for review by the payment provider. Verify the exact payment amount, account, and reference before approving credits.</Alert> : null}

          <DrawerSection title="Payment summary" description="Expected package amount and account credit details.">
            <div className="grid gap-3 sm:grid-cols-2">
              <Detail label="User" value={currentPayment.user_email} />
              <Detail label="User ID" value={currentPayment.user_id} mono />
              <Detail label="Package" value={currentPayment.package_name ?? "-"} />
              <Detail label="Expected amount" value={formatMoney(currentPayment.amount_minor, currentPayment.currency)} />
              <Detail label="Credit units" value={formatCredits(currentPayment.credit_units)} />
              <Detail label="Provider" value={formatProvider(currentPayment.provider)} />
              <Detail label="Created" value={formatDateTime(currentPayment.created_at)} />
              <Detail label="Status" value={statusLabel(currentPayment.status)} />
              {currentPayment.provider_status ? <Detail label="Provider status" value={currentPayment.provider_status} /> : null}
              {currentPayment.checkout_reference ? <Detail label="Checkout reference" value={currentPayment.checkout_reference} mono /> : null}
            </div>
          </DrawerSection>

          <DrawerSection title="User-submitted proof" description="Review uploaded evidence before approving any credits.">
            <div className="grid gap-3 sm:grid-cols-2">
              <Detail label="Proof" value={currentPayment.proof_uploaded ? "Uploaded" : "Missing"} />
              <Detail label="Duplicate proof count" value={formatUnits(currentPayment.duplicate_proof_count)} />
              {proof ? <Detail label="Uploaded" value={formatDateTime(proof.uploaded_at)} /> : null}
              {proof ? <Detail label="File" value={proof.file_name + " / " + proof.file_mime} /> : null}
              {proof ? <Detail label="File size" value={formatUnits(proof.file_size) + " bytes"} /> : null}
              {proof?.user_transaction_ref ? <Detail label="User transaction ref" value={proof.user_transaction_ref} mono /> : null}
              {proof?.user_note ? <Detail label="User note" value={proof.user_note} /> : null}
              {currentPayment.proof_file_sha256 ? <Detail label="Proof hash" value={truncateId(currentPayment.proof_file_sha256, 12, 8)} mono /> : null}
            </div>
            {currentPayment.proof_uploaded ? (
              <div className="mt-4 overflow-hidden rounded-2xl border border-slate-200 bg-slate-50">
                {proofIsImage ? <img alt="Payment proof" className="max-h-[520px] w-full object-contain" src={proofUrl} /> : null}
                {proofIsPdf ? <div className="p-4"><a className={secondaryLink} href={proofUrl} target="_blank" rel="noreferrer">Open proof PDF</a></div> : null}
                {!proofIsImage && !proofIsPdf ? <div className="p-4"><a className={secondaryLink} href={proofUrl} target="_blank" rel="noreferrer">View proof</a></div> : null}
              </div>
            ) : <p className="mt-4 rounded-xl border border-dashed border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-500">No proof has been uploaded for this payment.</p>}
          </DrawerSection>

          <DrawerSection title="Admin verification" description="Approve only after matching the payment in Bakong.">
            {canApprove ? (
              <form className="rounded-2xl border border-slate-200 bg-slate-50 p-4" onSubmit={approve}>
                <CardTitle>Approve payment</CardTitle>
                <CardDescription className="mt-1">Enter the reference shown in Bakong after you verify the payment.</CardDescription>
                <div className="mt-4 space-y-3">
                  <Input label="Verified payment reference" value={reference} onChange={(event) => setReference(event.target.value)} required />
                  <Input label="Admin note optional" value={note} onChange={(event) => setNote(event.target.value)} />
                  <Button className="rounded-lg" type="submit" disabled={actionLoading || !reference.trim()}>{actionLoading ? "Working" : "Approve and credit"}</Button>
                </div>
              </form>
            ) : null}

            {currentPayment.status === "paid" ? (
              <div className="grid gap-3 sm:grid-cols-2">
                <Detail label="Verified reference" value={currentPayment.admin_payment_reference ?? currentPayment.provider_transaction_id ?? "-"} mono />
                <Detail label="Reviewed by" value={currentPayment.reviewed_by_admin_email ?? "-"} />
                <Detail label="Reviewed at" value={formatDateTime(currentPayment.reviewed_at)} />
                <Detail label="Paid at" value={formatDateTime(currentPayment.paid_at)} />
                <div className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-sm text-emerald-800 sm:col-span-2">Ledger credit has been applied for this payment.</div>
              </div>
            ) : null}

            {currentPayment.status === "rejected" ? (
              <div className="grid gap-3 sm:grid-cols-2">
                <Detail label="Rejection reason" value={currentPayment.rejection_reason ?? "-"} />
                <Detail label="Reviewed by" value={currentPayment.reviewed_by_admin_email ?? "-"} />
                <Detail label="Reviewed at" value={formatDateTime(currentPayment.reviewed_at)} />
              </div>
            ) : null}

            {canReject ? (
              <form className="mt-4 rounded-2xl border border-red-200 bg-red-50 p-4" onSubmit={reject}>
                <CardTitle>Reject payment</CardTitle>
                <CardDescription className="mt-1 text-red-800/80">Rejecting never credits the account.</CardDescription>
                <div className="mt-4 space-y-3">
                  <Input label="Rejection reason" value={reason} onChange={(event) => setReason(event.target.value)} required />
                  <Button className="rounded-lg" type="submit" variant="danger" disabled={actionLoading || !reason.trim()}>{actionLoading ? "Working" : "Reject"}</Button>
                </div>
              </form>
            ) : null}
          </DrawerSection>
        </div>
      </aside>
    </div>
  );
}

function DrawerSection({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="mb-4">
        <h3 className="text-sm font-semibold text-slate-950">{title}</h3>
        <p className="mt-1 text-sm leading-6 text-slate-500">{description}</p>
      </div>
      {children}
    </section>
  );
}

function Detail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2.5"><dt className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</dt><dd className={cn("mt-1 break-words text-sm font-medium text-slate-950", mono && "font-mono text-xs")}>{value}</dd></div>;
}

function Alert({ tone, children }: { tone: "green" | "red" | "yellow"; children: ReactNode }) {
  const styles = tone === "green" ? "border-emerald-200 bg-emerald-50 text-emerald-800" : tone === "red" ? "border-red-200 bg-red-50 text-red-800" : "border-amber-200 bg-amber-50 text-amber-900";
  return <div className={"rounded-xl border px-4 py-3 text-sm leading-6 " + styles}>{children}</div>;
}
