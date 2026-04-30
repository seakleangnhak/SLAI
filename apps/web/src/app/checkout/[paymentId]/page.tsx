"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { useParams } from "next/navigation";

import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type Payment } from "@/lib/api";
import { formatCredits, formatDateTime, formatMoney, truncateId } from "@/lib/format";

const secondaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-400";

function paymentTone(status: string) {
  if (status === "paid") return "green" as const;
  if (status === "pending_payment") return "blue" as const;
  if (status === "expired") return "yellow" as const;
  if (status === "needs_review" || status === "rejected" || status === "cancelled") return "red" as const;
  return "neutral" as const;
}

function statusCopy(payment: Payment) {
  if (payment.status === "paid") {
    return {
      title: "Payment confirmed",
      text: "Credits have been added to your ledger-backed balance.",
      tone: "green" as const
    };
  }
  if (payment.status === "expired") {
    return {
      title: "Payment expired",
      text: "This checkout is no longer payable. Choose the package again to generate a fresh KHQR.",
      tone: "yellow" as const
    };
  }
  if (payment.status === "needs_review") {
    return {
      title: "Payment needs review",
      text: "The payment provider could not safely match this payment automatically. SLAI has not credited the balance.",
      tone: "red" as const
    };
  }
  if (payment.status === "cancelled" || payment.status === "rejected") {
    return {
      title: "Payment closed",
      text: payment.rejectionReason ?? "This checkout cannot be completed. Choose the package again if you still need credits.",
      tone: "red" as const
    };
  }
  if (payment.status === "pending_proof" || payment.status === "pending_review") {
    return {
      title: "Legacy manual payment",
      text: "This payment was created before automatic payment confirmation. Contact an administrator if it still needs review.",
      tone: "yellow" as const
    };
  }
  return {
    title: "Waiting for payment",
    text: "Scan the KHQR and complete payment in your banking app. SLAI updates this checkout after payment confirmation.",
    tone: "blue" as const
  };
}

export default function CheckoutPage() {
  const params = useParams<{ paymentId: string }>();
  const paymentId = params.paymentId;
  const [payment, setPayment] = useState<Payment | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    api.payments
      .get(paymentId)
      .then((response) => setPayment(response.payment))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }, [paymentId]);

  const refreshPayment = useCallback(async (quiet = false) => {
    if (!quiet) {
      setRefreshing(true);
      setNotice(null);
      setError(null);
    }
    try {
      const response = await api.payments.refresh(paymentId);
      setPayment(response.payment);
      if (!quiet) {
        setNotice(response.payment.status === "paid" ? "Payment confirmed and credits added." : "Payment status refreshed.");
      }
    } catch (err) {
      if (!quiet) {
        setError(readableError(err));
      }
    } finally {
      if (!quiet) {
        setRefreshing(false);
      }
    }
  }, [paymentId]);

  useEffect(load, [load]);

  useEffect(() => {
    if (payment?.status !== "pending_payment") {
      return undefined;
    }
    const timer = window.setInterval(() => void refreshPayment(true), 8000);
    return () => window.clearInterval(timer);
  }, [payment?.status, refreshPayment]);

  const status = payment ? statusCopy(payment) : null;
  const payable = payment?.status === "pending_payment";

  return (
    <DashboardShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Checkout</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Bakong KHQR payment</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Scan the KHQR, complete payment, and SLAI will credit your account after confirmation.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button className="rounded-lg" type="button" variant="secondary" onClick={() => void refreshPayment()} disabled={refreshing || loading}>
              {refreshing ? "Checking" : "Check status"}
            </Button>
            <Link className={secondaryButton} href="/dashboard/billing">Back to billing</Link>
          </div>
        </div>
      </section>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading checkout" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
      </div>

      {!loading && payment ? (
        <section className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(360px,0.55fr)]">
          <div className="space-y-6">
            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4">
                <div>
                  <CardTitle>{payment.packageName ?? "Credit package"}</CardTitle>
                  <CardDescription>Credits are added only after the payment provider confirms the checkout.</CardDescription>
                </div>
                <Badge dot tone={paymentTone(payment.status)}>{payment.status}</Badge>
              </CardHeader>
              <div className="grid gap-3 sm:grid-cols-3">
                <Metric label="Amount" value={formatMoney(payment.amountMinor, payment.currency)} />
                <Metric label="Credits" value={formatCredits(payment.creditUnits)} />
                <Metric label="Created" value={formatDateTime(payment.createdAt)} />
              </div>
              {status ? <StatusPanel title={status.title} text={status.text} tone={status.tone} /> : null}
              {notice ? <div className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{notice}</div> : null}
            </Card>

            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4">
                <div>
                  <CardTitle>Payment details</CardTitle>
                  <CardDescription>Use the reference shown here when checking payment status with support.</CardDescription>
                </div>
              </CardHeader>
              <div className="grid gap-3 sm:grid-cols-2">
                <Info label="Checkout reference" value={payment.checkoutReference ?? payment.providerRef ?? "-"} mono />
                <Info label="Provider status" value={payment.providerStatus ?? "-"} />
                <Info label="Expires" value={formatDateTime(payment.expiresAt)} />
                <Info label="Paid" value={formatDateTime(payment.paidAt)} />
                <Info label="Transaction ID" value={payment.providerTransactionId ? truncateId(payment.providerTransactionId, 12, 6) : "-"} mono />
                <Info label="APV" value={payment.providerApv ?? "-"} mono />
              </div>
            </Card>
          </div>

          <div className="space-y-6">
            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4">
                <div>
                  <CardTitle>Scan to pay</CardTitle>
                  <CardDescription>{payable ? "Open your Bakong or bank app and scan this KHQR." : "This QR is retained for checkout reference."}</CardDescription>
                </div>
              </CardHeader>
              {payment.qrImageDataUri ? (
                <img alt="Bakong KHQR" className="w-full rounded-2xl border border-slate-200 bg-white object-contain p-3" src={payment.qrImageDataUri} />
              ) : payment.qrPayload ? (
                <pre className="overflow-x-auto rounded-2xl bg-slate-950 p-4 text-xs leading-6 text-slate-100"><code>{payment.qrPayload}</code></pre>
              ) : (
                <div className="rounded-2xl border border-dashed border-slate-200 bg-slate-50 p-8 text-center text-sm text-slate-500">QR data is not available for this payment.</div>
              )}
              <dl className="mt-4 space-y-3 text-sm">
                <Info label="Reference" value={payment.checkoutReference ?? payment.providerRef ?? "-"} mono />
                <Info label="Amount" value={formatMoney(payment.amountMinor, payment.currency)} />
              </dl>
              <p className="mt-4 rounded-xl border border-blue-100 bg-blue-50 px-3 py-2 text-sm leading-6 text-blue-900/80">
                Keep this checkout open after paying. If the status does not update automatically, use Check status.
              </p>
            </Card>
          </div>
        </section>
      ) : null}
    </DashboardShell>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-3"><p className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</p><p className="mt-2 text-lg font-semibold text-slate-950">{value}</p></div>;
}

function Info({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="flex justify-between gap-4 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2"><dt className="text-slate-500">{label}</dt><dd className={mono ? "font-mono text-xs font-medium text-slate-950" : "font-medium text-slate-950"}>{value}</dd></div>;
}

function StatusPanel({ title, text, tone = "blue" }: { title: string; text: string; tone?: "blue" | "green" | "red" | "yellow" }) {
  const styles = tone === "green" ? "border-emerald-200 bg-emerald-50 text-emerald-800" : tone === "red" ? "border-red-200 bg-red-50 text-red-800" : tone === "yellow" ? "border-amber-200 bg-amber-50 text-amber-900" : "border-blue-200 bg-blue-50 text-blue-800";
  return <div className={"mt-5 rounded-xl border px-4 py-3 text-sm leading-6 " + styles}><p className="font-semibold">{title}</p><p>{text}</p></div>;
}
