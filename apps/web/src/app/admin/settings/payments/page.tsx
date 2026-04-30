"use client";

import { FormEvent, useEffect, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, apiAssetUrl, readableError, type PaymentProviderStatus, type PaymentSettings } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

export default function AdminPaymentSettingsPage() {
  const [settings, setSettings] = useState<PaymentSettings | null>(null);
  const [providerStatus, setProviderStatus] = useState<PaymentProviderStatus | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [displayName, setDisplayName] = useState("Bakong KHQR");
  const [accountName, setAccountName] = useState("");
  const [accountId, setAccountId] = useState("");
  const [instructions, setInstructions] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  function apply(next: PaymentSettings) {
    setSettings(next);
    setEnabled(next.enabled);
    setDisplayName(next.display_name || "Bakong KHQR");
    setAccountName(next.account_name ?? "");
    setAccountId(next.account_id ?? "");
    setInstructions(next.instructions ?? "");
  }

  function load() {
    setLoading(true);
    setError(null);
    Promise.all([api.admin.paymentSettings.bakong.get(), api.admin.paymentSettings.bakong.providerStatus()])
      .then(([settingsResponse, statusResponse]) => {
        apply(settingsResponse.settings);
        setProviderStatus(statusResponse.provider_status);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  async function save(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.admin.paymentSettings.bakong.update({
        enabled,
        display_name: displayName,
        account_name: accountName || null,
        account_id: accountId || null,
        instructions: instructions || null
      });
      apply(response.settings);
      setNotice("Payment settings saved.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setSaving(false);
    }
  }

  async function uploadImage() {
    if (!file) {
      setError("Select a KHQR image first.");
      return;
    }
    setUploading(true);
    setError(null);
    setNotice(null);
    const form = new FormData();
    form.set("file", file);
    try {
      const response = await api.admin.paymentSettings.bakong.uploadImage(form);
      apply(response.settings);
      setFile(null);
      setNotice("KHQR image uploaded.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setUploading(false);
    }
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Payment settings</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Bakong KHQR payments</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Monitor automatic KHQR checkout configuration. Static settings remain available as a fallback only.</p>
          </div>
          {providerStatus ? <Badge dot tone={providerStatus.enabled ? "green" : "neutral"}>{providerStatus.enabled ? "Automatic enabled" : "Automatic disabled"}</Badge> : null}
        </div>
      </section>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading payment settings" /> : null}
        {error ? <ErrorState message={error} onRetry={load} /> : null}
      </div>

      {!loading && settings ? (
        <>
          {providerStatus ? (
            <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <StatusCard label="Mode" value={providerStatus.mode.replaceAll("_", " ")} ok={providerStatus.enabled} />
              <StatusCard label="Base URL" value={providerStatus.base_url_configured ? "Configured" : "Missing"} ok={providerStatus.base_url_configured} />
              <StatusCard label="Callback HMAC" value={providerStatus.callback_secret_configured ? "Configured" : "Missing"} ok={providerStatus.callback_secret_configured} />
              <StatusCard label="Expiry" value={providerStatus.default_expiry_seconds ? Math.round(providerStatus.default_expiry_seconds / 60) + " min" : "-"} ok={providerStatus.default_expiry_seconds > 0} />
            </section>
          ) : null}

        <section className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
          <Card className="rounded-2xl p-5">
            <CardHeader className="mb-4">
              <div>
                <CardTitle>Fallback checkout details</CardTitle>
                <CardDescription>Used only if automatic slai-payment checkout is disabled. New automatic checkouts use provider-generated KHQR data.</CardDescription>
              </div>
            </CardHeader>
            {notice ? <div className="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{notice}</div> : null}
            <form className="space-y-4" onSubmit={save}>
              <label className="flex items-center gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-3 text-sm font-medium text-slate-700">
                <input checked={enabled} type="checkbox" onChange={(event) => setEnabled(event.target.checked)} />
                Enable fallback static KHQR checkout
              </label>
              <Input label="Display name" value={displayName} onChange={(event) => setDisplayName(event.target.value)} required />
              <div className="grid gap-4 sm:grid-cols-2">
                <Input label="Account name" value={accountName} onChange={(event) => setAccountName(event.target.value)} required={enabled} />
                <Input label="Account ID / phone / merchant ID" value={accountId} onChange={(event) => setAccountId(event.target.value)} required={enabled} />
              </div>
              <label className="block">
                <span className="text-sm font-medium text-slate-700">Instructions</span>
                <textarea className="mt-2 min-h-32 w-full rounded-xl border border-slate-300 px-3 py-2 text-sm outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4" value={instructions} onChange={(event) => setInstructions(event.target.value)} />
              </label>
              <Button className="rounded-lg" type="submit" disabled={saving}>{saving ? "Saving" : "Save settings"}</Button>
            </form>
          </Card>

          <div className="space-y-6">
            <Card className="rounded-2xl p-5">
              <CardHeader className="mb-4">
                <div>
                  <CardTitle>KHQR image</CardTitle>
                  <CardDescription>PNG, JPEG, or WebP. Max size is configured by the API.</CardDescription>
                </div>
              </CardHeader>
              {settings.khqr_image_url ? <img alt="Current Bakong KHQR" className="w-full rounded-2xl border border-slate-200 bg-white object-contain p-3" src={apiAssetUrl(settings.khqr_image_url)} /> : <div className="rounded-2xl border border-dashed border-slate-200 bg-slate-50 p-8 text-center text-sm text-slate-500">No KHQR image uploaded</div>}
              <div className="mt-4 space-y-3">
                <input className="block w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm" type="file" accept="image/png,image/jpeg,image/webp" onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
                <Button className="w-full rounded-lg" type="button" variant="secondary" disabled={uploading} onClick={uploadImage}>{uploading ? "Uploading" : "Upload image"}</Button>
              </div>
            </Card>

            <Card className="rounded-2xl border-amber-200 bg-amber-50 p-5">
              <CardTitle>Automatic confirmation</CardTitle>
              <CardDescription className="mt-2 text-amber-900/80">New checkouts are confirmed by signed slai-payment callbacks. The callback secret is never shown in the admin console.</CardDescription>
              <p className="mt-4 text-xs text-amber-900/70">Updated {formatDateTime(settings.updated_at)}</p>
            </Card>
          </div>
        </section>
        </>
      ) : null}
    </AdminShell>
  );
}

function StatusCard({ label, value, ok }: { label: string; value: string; ok: boolean }) {
  return (
    <Card className="rounded-2xl p-5">
      <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">{label}</p>
      <div className="mt-3 flex items-center justify-between gap-3">
        <p className="text-lg font-semibold capitalize text-slate-950">{value}</p>
        <Badge dot tone={ok ? "green" : "red"}>{ok ? "OK" : "Check"}</Badge>
      </div>
    </Card>
  );
}
