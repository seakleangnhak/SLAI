"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useParams } from "next/navigation";

import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { Input, Textarea } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, apiAssetUrl, readableError, type Payment, type PaymentSettings } from "@/lib/api";
import { formatCredits, formatDateTime, formatMoney, formatUnits, truncateId } from "@/lib/format";

const secondaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-400";

type StatusTone = "blue" | "green" | "red" | "yellow" | "neutral";

type StatusCopy = {
  title: string;
  text: string;
  tone: Exclude<StatusTone, "neutral">;
  nextStep: string;
};

function paymentTone(status: string): StatusTone {
  if (status === "paid") return "green";
  if (status === "pending_payment" || status === "pending_proof") return "blue";
  if (status === "pending_review") return "yellow";
  if (status === "expired") return "yellow";
  if (status === "needs_review" || status === "rejected" || status === "cancelled") return "red";
  return "neutral";
}

function statusLabel(status: string) {
  return status.replaceAll("_", " ");
}

function statusCopy(payment: Payment): StatusCopy {
  if (payment.status === "paid") {
    return {
      title: "Payment approved",
      text: "Your payment was approved and credits were added to your SLAI balance.",
      tone: "green",
      nextStep: "Open billing or return to your dashboard to review the updated balance."
    };
  }
  if (payment.status === "pending_review") {
    return {
      title: "Proof submitted",
      text: "Your payment proof has been submitted and is waiting for admin review.",
      tone: "yellow",
      nextStep: "No action is needed unless an admin asks for a clearer proof or reference."
    };
  }
  if (payment.status === "pending_proof") {
    return {
      title: "Waiting for proof upload",
      text: "Complete the payment in your banking app, then upload the transaction screenshot or receipt.",
      tone: "blue",
      nextStep: "Pay the exact amount, save the receipt, and submit it below."
    };
  }
  if (payment.status === "pending_payment") {
    return {
      title: "Waiting for payment confirmation",
      text: "Scan the KHQR and complete payment in your banking app. SLAI checks this checkout with the payment provider.",
      tone: "blue",
      nextStep: "Keep this page open and use Check status if the payment does not update after a moment."
    };
  }
  if (payment.status === "expired") {
    return {
      title: "Payment expired",
      text: "This checkout is no longer payable. Choose the package again to generate a fresh KHQR.",
      tone: "yellow",
      nextStep: "Go back to billing and start a new checkout for the package."
    };
  }
  if (payment.status === "needs_review") {
    return {
      title: "Payment needs review",
      text: "The payment provider could not safely match this payment automatically. SLAI has not credited the balance yet.",
      tone: "red",
      nextStep: "Contact an administrator with the checkout reference if this payment should be reviewed."
    };
  }
  if (payment.status === "rejected") {
    return {
      title: "Payment rejected",
      text: payment.rejectionReason ?? "The submitted proof could not be approved.",
      tone: "red",
      nextStep: "Upload a clearer proof if you paid this checkout, or start a new checkout from billing."
    };
  }
  if (payment.status === "cancelled") {
    return {
      title: "Payment cancelled",
      text: "This checkout has been cancelled and credits were not added.",
      tone: "red",
      nextStep: "Choose the package again if you still need credits."
    };
  }
  return {
    title: "Checkout in progress",
    text: "Review this payment and follow the available next step.",
    tone: "blue",
    nextStep: "Refresh the checkout status if anything looks stale."
  };
}

function canUploadProof(payment: Payment) {
  return payment.status === "pending_proof" || payment.status === "rejected";
}

function canViewProof(payment: Payment) {
  return payment.proofUploaded && ["pending_review", "rejected", "paid"].includes(payment.status);
}

function isAutoPayment(payment: Payment) {
  return Boolean(payment.externalPaymentId || payment.qrImageDataUri || payment.qrPayload || payment.status === "pending_payment");
}

function fileSizeLabel(file: File | null) {
  if (!file) {
    return "";
  }
  if (file.size >= 1024 * 1024) {
    return (file.size / 1024 / 1024).toFixed(2) + " MB";
  }
  return formatUnits(Math.ceil(file.size / 1024)) + " KB";
}

function referenceFor(payment: Payment) {
  return payment.checkoutReference ?? payment.providerRef ?? payment.externalPaymentId ?? payment.id;
}

export default function CheckoutPage() {
  const params = useParams<{ paymentId: string }>();
  const paymentId = params.paymentId;
  const [payment, setPayment] = useState<Payment | null>(null);
  const [settings, setSettings] = useState<PaymentSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [transactionRef, setTransactionRef] = useState("");
  const [note, setNote] = useState("");
  const [copiedReference, setCopiedReference] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      api.payments.get(paymentId),
      api.checkout.bakongSettings().then((response) => response.settings).catch(() => null)
    ])
      .then(([paymentResponse, loadedSettings]) => {
        setPayment(paymentResponse.payment);
        setSettings(loadedSettings);
      })
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
    const autoRefreshStatuses = ["pending_payment", "pending_review", "needs_review"];
    if (!payment?.status || !autoRefreshStatuses.includes(payment.status)) {
      return undefined;
    }
    const timer = window.setInterval(() => void refreshPayment(true), 3000);
    return () => window.clearInterval(timer);
  }, [payment?.status, refreshPayment]);

  const summary = payment ? statusCopy(payment) : null;
  const proofAllowed = payment ? canUploadProof(payment) : false;
  const proofViewable = payment ? canViewProof(payment) : false;
  const autoPayment = payment ? isAutoPayment(payment) : false;
  const qrImageSrc = useMemo(() => {
    if (payment?.qrImageDataUri) {
      return payment.qrImageDataUri;
    }
    if (settings?.khqr_image_url) {
      return apiAssetUrl(settings.khqr_image_url);
    }
    return null;
  }, [payment?.qrImageDataUri, settings?.khqr_image_url]);
  const checkoutReference = payment ? referenceFor(payment) : "";

  async function submitProof(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setUploadError(null);
    setNotice(null);
    if (!selectedFile) {
      setUploadError("Choose a screenshot, receipt image, or PDF before submitting proof.");
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.set("file", selectedFile);
      if (transactionRef.trim()) {
        formData.set("transaction_ref", transactionRef.trim());
      }
      if (note.trim()) {
        formData.set("note", note.trim());
      }
      const response = await api.payments.uploadProof(paymentId, formData);
      setPayment(response.payment);
      setSelectedFile(null);
      setTransactionRef("");
      setNote("");
      setNotice("Payment proof submitted. Your payment is waiting for admin review.");
    } catch (err) {
      setUploadError(readableError(err));
    } finally {
      setUploading(false);
    }
  }

  async function copyReference() {
    if (!checkoutReference) {
      return;
    }
    try {
      await navigator.clipboard.writeText(checkoutReference);
      setCopiedReference(true);
      window.setTimeout(() => setCopiedReference(false), 1600);
    } catch {
      setCopiedReference(false);
    }
  }

  return (
    <DashboardShell>
      <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
        <div className="relative p-5 sm:p-6">
          <div className="pointer-events-none absolute inset-x-0 top-0 h-1 bg-gradient-to-r from-blue-500 via-indigo-500 to-violet-500" />
          <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Checkout</p>
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <h1 className="text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Bakong KHQR payment</h1>
                {payment ? <Badge dot tone={paymentTone(payment.status)}>{statusLabel(payment.status)}</Badge> : null}
              </div>
              <p className="mt-3 max-w-3xl text-sm leading-6 text-slate-500">
                Scan the KHQR, complete the payment, upload proof when requested, and SLAI will credit your account after confirmation.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button className="rounded-lg" type="button" variant="secondary" onClick={() => void refreshPayment()} disabled={refreshing || loading}>
                {refreshing ? "Checking" : "Check status"}
              </Button>
              <Link className={secondaryButton} href="/dashboard/billing">Back to billing</Link>
            </div>
          </div>
        </div>
      </section>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading checkout" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
      </div>

      {!loading && payment && summary ? (
        <section className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,0.95fr)_minmax(360px,0.55fr)]">
          <div className="space-y-6">
            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4 flex-col gap-3 sm:flex-row sm:items-start">
                <div>
                  <CardTitle>{payment.packageName ?? "Credit package"}</CardTitle>
                  <CardDescription>Review the order before paying. Credits are added only after confirmation.</CardDescription>
                </div>
                <Badge dot tone={paymentTone(payment.status)}>{statusLabel(payment.status)}</Badge>
              </CardHeader>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <Metric label="Amount" value={formatMoney(payment.amountMinor, payment.currency)} accent />
                <Metric label="Total credits" value={formatCredits(payment.creditUnits)} />
                <Metric label="Created" value={formatDateTime(payment.createdAt)} />
                <Metric label="Status" value={statusLabel(payment.status)} muted />
              </div>
              <div className="mt-4 rounded-xl border border-blue-100 bg-blue-50 px-4 py-3 text-sm leading-6 text-blue-900/80">
                Credits are never added from the checkout screen alone. SLAI only updates your balance after a verified payment or admin approval.
              </div>
            </Card>

            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4 flex-col gap-3 sm:flex-row sm:items-start">
                <div>
                  <CardTitle>Payment status</CardTitle>
                  <CardDescription>Your next action depends on the current checkout state.</CardDescription>
                </div>
                <Badge dot tone={summary.tone}>{statusLabel(payment.status)}</Badge>
              </CardHeader>
              <StatusPanel title={summary.title} text={summary.text} tone={summary.tone} />
              <div className="mt-4 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm leading-6 text-slate-600">
                <p className="font-semibold text-slate-900">Next step</p>
                <p>{summary.nextStep}</p>
              </div>
              {payment.status === "paid" ? (
                <div className="mt-4 flex flex-wrap gap-2">
                  <Link className={secondaryButton} href="/dashboard/billing">Open billing</Link>
                  <Link className={secondaryButton} href="/dashboard">Open dashboard</Link>
                </div>
              ) : null}
              {notice ? <div className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{notice}</div> : null}
            </Card>

            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4 flex-col gap-3 sm:flex-row sm:items-start">
                <div>
                  <CardTitle>Upload payment proof</CardTitle>
                  <CardDescription>Use this only for manual checkouts that require admin review.</CardDescription>
                </div>
                {payment.proofUploaded ? <Badge dot tone="yellow">Proof uploaded</Badge> : <Badge tone="neutral">Manual review</Badge>}
              </CardHeader>

              {proofAllowed ? (
                <form className="space-y-4" onSubmit={submitProof}>
                  <label className="block rounded-2xl border border-dashed border-slate-300 bg-slate-50 p-4 transition hover:border-blue-300 hover:bg-blue-50/40">
                    <span className="text-sm font-semibold text-slate-900">Receipt or screenshot</span>
                    <span className="mt-1 block text-xs leading-5 text-slate-500">PNG, JPEG, WebP, or PDF. Upload the receipt after paying the exact amount.</span>
                    <input
                      className="mt-3 block w-full text-sm text-slate-600 file:mr-4 file:rounded-lg file:border-0 file:bg-slate-950 file:px-3 file:py-2 file:text-sm file:font-semibold file:text-white hover:file:bg-slate-800"
                      type="file"
                      accept="image/png,image/jpeg,image/webp,application/pdf"
                      onChange={(event) => setSelectedFile(event.target.files?.[0] ?? null)}
                    />
                    {selectedFile ? (
                      <span className="mt-3 block rounded-xl border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-600">
                        Selected: {selectedFile.name} ({fileSizeLabel(selectedFile)})
                      </span>
                    ) : null}
                  </label>
                  <Input
                    label="Transaction reference (optional)"
                    hint="This is a user note only. Admins verify the final payment reference separately."
                    value={transactionRef}
                    onChange={(event) => setTransactionRef(event.target.value)}
                    placeholder="Example: bank receipt reference"
                  />
                  <Textarea
                    label="Note (optional)"
                    hint="Add context if the receipt name or amount needs clarification."
                    value={note}
                    onChange={(event) => setNote(event.target.value)}
                    placeholder="Optional note for the admin reviewer"
                  />
                  {uploadError ? <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{uploadError}</div> : null}
                  <Button className="rounded-lg" type="submit" disabled={uploading}>
                    {uploading ? "Submitting proof" : "Submit proof"}
                  </Button>
                </form>
              ) : proofViewable ? (
                <div className="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm leading-6 text-amber-900">
                  <p className="font-semibold">Proof is already submitted</p>
                  <p>Your proof is attached to this checkout. You can view it while waiting for review.</p>
                  <a className="mt-3 inline-flex text-sm font-semibold text-amber-900 underline-offset-4 hover:underline" href={api.payments.proofUrl(payment.id)} target="_blank" rel="noreferrer">
                    View proof
                  </a>
                </div>
              ) : autoPayment ? (
                <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4 text-sm leading-6 text-slate-600">
                  <p className="font-semibold text-slate-900">Proof upload is not required</p>
                  <p>This checkout is handled by provider status checks. Use Check status if the payment does not update automatically.</p>
                </div>
              ) : (
                <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4 text-sm leading-6 text-slate-600">
                  <p className="font-semibold text-slate-900">Proof upload is closed</p>
                  <p>This checkout status does not currently accept a new proof upload.</p>
                </div>
              )}
            </Card>

            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4">
                <div>
                  <CardTitle>Payment details</CardTitle>
                  <CardDescription>Use these references when checking payment status with support.</CardDescription>
                </div>
              </CardHeader>
              <div className="grid gap-3 sm:grid-cols-2">
                <Info label="Checkout reference" value={checkoutReference} mono actionLabel={copiedReference ? "Copied" : "Copy"} onAction={() => void copyReference()} />
                <Info label="Provider status" value={payment.providerStatus ?? "-"} />
                <Info label="Provider" value={payment.provider} />
                <Info label="Expires" value={formatDateTime(payment.expiresAt)} />
                <Info label="Transaction ID" value={payment.providerTransactionId ? truncateId(payment.providerTransactionId, 12, 6) : "-"} mono />
                <Info label="Payment APV" value={payment.providerApv ?? "-"} mono />
                <Info label="Verified reference" value={payment.adminPaymentReference ?? "-"} mono />
                <Info label="Paid" value={formatDateTime(payment.paidAt)} />
                <Info label="Reviewed" value={formatDateTime(payment.reviewedAt)} />
                <Info label="Callback received" value={formatDateTime(payment.callbackReceivedAt)} />
              </div>
            </Card>

            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4">
                <div>
                  <CardTitle>What happens next</CardTitle>
                  <CardDescription>The checkout flow keeps payment and crediting separate.</CardDescription>
                </div>
              </CardHeader>
              <div className="grid gap-3 sm:grid-cols-2">
                <Step number="1" title="Scan and pay" text="Pay the exact amount with Bakong or your banking app." />
                <Step number="2" title="Submit proof if needed" text="Manual checkouts ask for a receipt before review." />
                <Step number="3" title="Confirmation" text="SLAI waits for provider confirmation or admin approval." />
                <Step number="4" title="Credits added" text="Approved payments create a ledger credit on your account." />
              </div>
            </Card>
          </div>

          <aside className="space-y-6 xl:sticky xl:top-24 xl:self-start">
            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4">
                <div>
                  <CardTitle>Scan to pay</CardTitle>
                  <CardDescription>Open Bakong or your banking app and scan this KHQR.</CardDescription>
                </div>
              </CardHeader>
              <div className="rounded-3xl border border-slate-200 bg-gradient-to-br from-slate-50 to-white p-4 shadow-inner">
                {qrImageSrc ? (
                  <img alt="Bakong KHQR" className="mx-auto aspect-square w-full max-w-sm rounded-2xl border border-slate-200 bg-white object-contain p-3 shadow-sm" src={qrImageSrc} />
                ) : payment.qrPayload ? (
                  <pre className="overflow-x-auto rounded-2xl bg-slate-950 p-4 text-xs leading-6 text-slate-100"><code>{payment.qrPayload}</code></pre>
                ) : (
                  <div className="rounded-2xl border border-dashed border-slate-200 bg-white p-8 text-center text-sm text-slate-500">QR data is not available for this payment.</div>
                )}
              </div>
              <div className="mt-4 space-y-3 text-sm">
                <Info label="Amount" value={formatMoney(payment.amountMinor, payment.currency)} />
                <Info label="Package" value={payment.packageName ?? "Credit package"} />
                <Info label="Reference" value={checkoutReference} mono />
                {settings?.account_name ? <Info label="Account name" value={settings.account_name} /> : null}
                {settings?.account_id ? <Info label="Account ID" value={settings.account_id} mono /> : null}
              </div>
              <div className="mt-4 rounded-xl border border-blue-100 bg-blue-50 px-4 py-3 text-sm leading-6 text-blue-900/80">
                <p className="font-semibold">Payment instructions</p>
                {settings?.instructions ? <p className="whitespace-pre-line">{settings.instructions}</p> : <p>Pay the exact amount shown, save the receipt, and keep this checkout open until your status is updated.</p>}
              </div>
            </Card>

            <Card className="rounded-2xl border-amber-200 bg-amber-50/80 p-5 shadow-sm">
              <CardTitle>Keep this checkout open</CardTitle>
              <CardDescription className="mt-2 text-amber-900/80">
                If the status does not update after payment, use Check status. For manual proof states, upload your receipt and wait for admin review.
              </CardDescription>
            </Card>
          </aside>
        </section>
      ) : null}
    </DashboardShell>
  );
}

function Metric({ label, value, accent = false, muted = false }: { label: string; value: string; accent?: boolean; muted?: boolean }) {
  const className = accent
    ? "rounded-xl border border-blue-200 bg-blue-50 px-3 py-3"
    : muted
      ? "rounded-xl border border-slate-200 bg-white px-3 py-3"
      : "rounded-xl border border-slate-200 bg-slate-50 px-3 py-3";
  return (
    <div className={className}>
      <p className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</p>
      <p className="mt-2 text-lg font-semibold text-slate-950">{value}</p>
    </div>
  );
}

function Info({ label, value, mono = false, actionLabel, onAction }: { label: string; value: string; mono?: boolean; actionLabel?: string; onAction?: () => void }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2">
      <dt className="text-sm text-slate-500">{label}</dt>
      <dd className="flex min-w-0 items-center gap-2 text-right">
        <span className={mono ? "truncate font-mono text-xs font-medium text-slate-950" : "truncate text-sm font-medium text-slate-950"}>{value}</span>
        {actionLabel && onAction ? (
          <button className="shrink-0 text-xs font-semibold text-blue-700 hover:text-blue-800" type="button" onClick={onAction}>{actionLabel}</button>
        ) : null}
      </dd>
    </div>
  );
}

function StatusPanel({ title, text, tone = "blue" }: { title: string; text: string; tone?: "blue" | "green" | "red" | "yellow" }) {
  const styles = tone === "green" ? "border-emerald-200 bg-emerald-50 text-emerald-800" : tone === "red" ? "border-red-200 bg-red-50 text-red-800" : tone === "yellow" ? "border-amber-200 bg-amber-50 text-amber-900" : "border-blue-200 bg-blue-50 text-blue-800";
  return <div className={"rounded-xl border px-4 py-3 text-sm leading-6 " + styles}><p className="font-semibold">{title}</p><p>{text}</p></div>;
}

function Step({ number, title, text }: { number: string; title: string; text: string }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50 p-4">
      <div className="flex items-start gap-3">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-slate-950 text-xs font-semibold text-white">{number}</span>
        <div>
          <p className="text-sm font-semibold text-slate-950">{title}</p>
          <p className="mt-1 text-sm leading-6 text-slate-500">{text}</p>
        </div>
      </div>
    </div>
  );
}
