"use client";

import { FormEvent, useEffect, useState } from "react";

import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { CopyButton } from "@/components/CopyButton";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, isNotFound, readableError, type PublicAPIKey } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/format";

type ConfirmAction = "rotate" | "revoke" | null;

const primaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg bg-slate-950 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-300";
const secondaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-400";
const dangerButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-red-700 disabled:cursor-not-allowed disabled:bg-red-300";

function safeDate(value?: string | null, fallback = "Never") {
  if (!value) {
    return fallback;
  }
  return formatDateTime(value);
}

function keyStatusLabel(apiKey: PublicAPIKey | null) {
  return apiKey?.status ?? "None";
}

function modeLabel(apiKey: PublicAPIKey | null) {
  if (!apiKey) {
    return "Managed by SLAI";
  }
  return "Managed by SLAI";
}

function quickstartCurl(rawKey: string | null) {
  return [
    "curl https://YOUR_SLAI_GATEWAY_DOMAIN/v1/chat/completions \\",
    '  -H "Authorization: Bearer ' + (rawKey ?? "YOUR_API_KEY") + '" \\',
    '  -H "Content-Type: application/json"'
  ].join("\n");
}

function SummaryCard({ label, value, hint, children }: { label: string; value: string; hint: string; children?: React.ReactNode }) {
  return (
    <Card className="min-h-36 rounded-2xl p-5 shadow-sm transition hover:shadow-md">
      <div className="flex h-full flex-col">
        <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">{label}</p>
        <p className="mt-3 truncate text-2xl font-semibold tracking-normal text-slate-950">{value}</p>
        <p className="mt-2 text-sm leading-6 text-slate-500">{hint}</p>
        {children ? <div className="mt-auto pt-3">{children}</div> : null}
      </div>
    </Card>
  );
}

function Alert({ tone = "info", children }: { tone?: "info" | "success" | "warning" | "danger"; children: React.ReactNode }) {
  const tones = {
    info: "border-blue-200 bg-blue-50 text-blue-800",
    success: "border-emerald-200 bg-emerald-50 text-emerald-800",
    warning: "border-amber-200 bg-amber-50 text-amber-900",
    danger: "border-red-200 bg-red-50 text-red-800"
  };
  return <div className={cn("rounded-xl border px-4 py-3 text-sm leading-6", tones[tone])}>{children}</div>;
}

function EmptyKeyState({ onCreateFocus }: { onCreateFocus: () => void }) {
  return (
    <div className="rounded-2xl border border-dashed border-slate-200 bg-slate-50/80 px-5 py-8 text-center">
      <span className="mx-auto grid size-10 place-items-center rounded-full bg-white font-mono text-sm font-semibold text-blue-700 shadow-sm ring-1 ring-slate-200">sk</span>
      <h3 className="mt-4 text-base font-semibold text-slate-950">No API key</h3>
      <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-slate-500">Create your one active key to send SLAI-billed AI requests.</p>
      <Button className="mt-5 rounded-lg" type="button" onClick={onCreateFocus}>Create API key</Button>
    </div>
  );
}

function KeyDetail({ label, value, mono = false }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5 shadow-sm">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</dt>
      <dd className={cn("mt-1 truncate text-sm font-medium text-slate-950", mono && "font-mono")}>{value}</dd>
    </div>
  );
}

function CurrentKeyCard({
  apiKey,
  actionLoading,
  pendingConfirm,
  onConfirm,
  onCancelConfirm,
  onRotateRequest,
  onRevokeRequest,
  onCreateFocus
}: {
  apiKey: PublicAPIKey | null;
  actionLoading: boolean;
  pendingConfirm: ConfirmAction;
  onConfirm: () => void;
  onCancelConfirm: () => void;
  onRotateRequest: () => void;
  onRevokeRequest: () => void;
  onCreateFocus: () => void;
}) {
  const keyRevoked = apiKey?.status === "REVOKED";

  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-5 items-start sm:items-center">
        <div>
          <CardTitle>Current key</CardTitle>
          <CardDescription>Only safe metadata is visible after create or rotate.</CardDescription>
        </div>
        {apiKey ? <Badge dot tone={statusTone(apiKey.status)}>{apiKey.status}</Badge> : <Badge dot tone="neutral">None</Badge>}
      </CardHeader>

      {apiKey ? (
        <div className="space-y-5">
          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">Key prefix</p>
              <span className="rounded-md bg-white px-2 py-1 text-xs font-medium text-slate-500 ring-1 ring-slate-200">raw key hidden</span>
            </div>
            <p className="mt-3 break-all font-mono text-sm font-semibold text-slate-950">{apiKey.key_prefix}</p>
          </div>

          <dl className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <KeyDetail label="Name" value={apiKey.name || "Default key"} />
            <KeyDetail label="Created" value={safeDate(apiKey.created_at, "-")} />
            <KeyDetail label="Last used" value={safeDate(apiKey.last_used_at)} />
            <KeyDetail label="Revoked" value={safeDate(apiKey.revoked_at, "-")} />
          </dl>

          {pendingConfirm ? (
            <ConfirmPanel action={pendingConfirm} actionLoading={actionLoading} onCancel={onCancelConfirm} onConfirm={onConfirm} />
          ) : null}

          <div className="flex flex-wrap gap-3">
            <Button type="button" variant="secondary" className="rounded-lg" onClick={onRotateRequest} disabled={actionLoading || keyRevoked}>
              Rotate key
            </Button>
            <Button type="button" variant="danger" className="rounded-lg" onClick={onRevokeRequest} disabled={actionLoading || keyRevoked}>
              Revoke key
            </Button>
          </div>
        </div>
      ) : (
        <EmptyKeyState onCreateFocus={onCreateFocus} />
      )}
    </Card>
  );
}

function ConfirmPanel({ action, actionLoading, onCancel, onConfirm }: { action: Exclude<ConfirmAction, null>; actionLoading: boolean; onCancel: () => void; onConfirm: () => void }) {
  const isRotate = action === "rotate";
  return (
    <Alert tone={isRotate ? "warning" : "danger"}>
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <p className="font-semibold text-slate-950">{isRotate ? "Rotate this key?" : "Revoke this key?"}</p>
          <p className="mt-1">
            {isRotate
              ? "Rotating revokes the current key. Existing integrations using the old key will stop working."
              : "Revoking this key will disable API access for this account."}
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <button className={secondaryButton} type="button" onClick={onCancel} disabled={actionLoading}>Cancel</button>
          <button className={isRotate ? primaryButton : dangerButton} type="button" onClick={onConfirm} disabled={actionLoading}>
            {actionLoading ? "Working" : isRotate ? "Rotate key" : "Revoke key"}
          </button>
        </div>
      </div>
    </Alert>
  );
}

function RawKeyRevealCard({ rawKey, onDismiss }: { rawKey: string | null; onDismiss: () => void }) {
  if (!rawKey) {
    return null;
  }

  return (
    <Card className="rounded-2xl border-amber-200 bg-gradient-to-br from-amber-50 to-white p-5 shadow-sm">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Copy your API key now</CardTitle>
          <CardDescription className="text-amber-800/80">This key is shown only once. SLAI cannot show it again.</CardDescription>
        </div>
        <Badge tone="yellow" dot>Shown once</Badge>
      </CardHeader>
      <div className="rounded-2xl border border-amber-200 bg-white p-3 shadow-sm">
        <code className="block max-h-40 overflow-auto break-all font-mono text-sm leading-6 text-slate-950">{rawKey}</code>
      </div>
      <div className="mt-4 flex flex-wrap gap-3">
        <CopyButton value={rawKey} />
        <Button type="button" variant="secondary" className="rounded-lg" onClick={onDismiss}>I have copied this key</Button>
      </div>
    </Card>
  );
}

function CreateOrSecurityCard({
  apiKey,
  name,
  setName,
  actionLoading,
  onCreate,
  createFormId
}: {
  apiKey: PublicAPIKey | null;
  name: string;
  setName: (value: string) => void;
  actionLoading: boolean;
  onCreate: (event: FormEvent) => void;
  createFormId: string;
}) {
  const canCreate = !apiKey || apiKey.status === "REVOKED";

  if (canCreate) {
    return (
      <Card className="rounded-2xl p-5" id={createFormId}>
        <CardHeader className="mb-4">
          <div>
            <CardTitle>Create key</CardTitle>
            <CardDescription>MVP allows one active key per user.</CardDescription>
          </div>
        </CardHeader>
        <form className="space-y-4" onSubmit={onCreate}>
          <Input
            label="Key name"
            hint="The raw key will be shown once after creation."
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
          />
          <Button type="submit" className="w-full rounded-lg" disabled={actionLoading || name.trim().length === 0}>
            {actionLoading ? "Creating" : "Create key"}
          </Button>
        </form>
      </Card>
    );
  }

  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Key actions</CardTitle>
          <CardDescription>Use rotation and revocation to control access.</CardDescription>
        </div>
      </CardHeader>
      <div className="space-y-3 text-sm leading-6 text-slate-600">
        <SecurityLine title="One active key" text="The MVP supports one active API key per account." />
        <SecurityLine title="Rotate if exposed" text="Rotation revokes the old key and reveals a new raw key once." />
        <SecurityLine title="Revoke to stop traffic" text="Revocation disables API access for this account." />
      </div>
    </Card>
  );
}

function QuickstartCard({ rawKey }: { rawKey: string | null }) {
  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Quickstart</CardTitle>
          <CardDescription>Use your key from a trusted server-side environment.</CardDescription>
        </div>
      </CardHeader>
      <div className="overflow-hidden rounded-2xl border border-slate-800 bg-slate-950 shadow-sm">
        <div className="flex items-center justify-between border-b border-white/10 px-4 py-2.5">
          <span className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-400">curl</span>
          <span className="rounded-md bg-white/10 px-2 py-1 text-xs font-medium text-slate-300">SLAI API</span>
        </div>
        <pre className="overflow-x-auto p-4 text-xs leading-6 text-slate-100 sm:text-sm"><code>{quickstartCurl(rawKey)}</code></pre>
      </div>
      <div className="mt-4 space-y-2 text-sm leading-6 text-slate-500">
        <p>Do not expose this key in frontend or mobile apps.</p>
        <p>Store it in environment variables on your server.</p>
      </div>
      <div className="mt-4 grid gap-2 sm:grid-cols-2">
        <a className={secondaryButton} href="/dashboard/usage">Open usage</a>
        <a className={secondaryButton} href="/dashboard/billing">Open billing</a>
      </div>
    </Card>
  );
}

function SecurityGuidanceCard() {
  return (
    <Card className="rounded-2xl p-5">
      <CardHeader className="mb-4">
        <div>
          <CardTitle>Security guidance</CardTitle>
          <CardDescription>Safe defaults for production integrations.</CardDescription>
        </div>
      </CardHeader>
      <div className="space-y-3">
        <SecurityLine title="Treat it like a password" text="Anyone with the raw key can spend credits through your account." />
        <SecurityLine title="Server-side only" text="Keep the key out of browser, mobile, and public repositories." />
        <SecurityLine title="Rotate leaked keys" text="Create a new key immediately if you suspect exposure." />
        <SecurityLine title="Shown once" text="SLAI stores only safe metadata and cannot reveal raw keys again." />
      </div>
    </Card>
  );
}

function SecurityLine({ title, text }: { title: string; text: string }) {
  return (
    <div className="flex gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2.5">
      <span className="mt-1 size-2 shrink-0 rounded-full bg-blue-600" />
      <div>
        <p className="text-sm font-semibold text-slate-950">{title}</p>
        <p className="mt-0.5 text-sm leading-6 text-slate-500">{text}</p>
      </div>
    </div>
  );
}

export default function APIKeyPage() {
  const [apiKey, setApiKey] = useState<PublicAPIKey | null>(null);
  const [rawKey, setRawKey] = useState<string | null>(null);
  const [name, setName] = useState("Default key");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [pendingConfirm, setPendingConfirm] = useState<ConfirmAction>(null);
  const createFormId = "create-key";

  function load() {
    setLoading(true);
    setError(null);
    api.apiKeys
      .get()
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

  useEffect(load, []);

  function focusCreateForm() {
    document.getElementById(createFormId)?.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  async function create(event: FormEvent) {
    event.preventDefault();
    setActionLoading(true);
    setError(null);
    setNotice(null);
    setPendingConfirm(null);
    try {
      const response = await api.apiKeys.create(name.trim() || "Default key");
      setApiKey(response.api_key);
      setRawKey(response.raw_api_key);
      setNotice("API key created. Copy the raw key now; it will not be shown again.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function rotate() {
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.apiKeys.rotate();
      setApiKey(response.api_key);
      setRawKey(response.raw_api_key);
      setNotice("API key rotated. Copy the new raw key now; it will not be shown again.");
      setPendingConfirm(null);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function revoke() {
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.apiKeys.revoke();
      setApiKey(response.api_key);
      setRawKey(null);
      setNotice("API key revoked. API access for this account is disabled until a new active key is created.");
      setPendingConfirm(null);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  function confirmPendingAction() {
    if (pendingConfirm === "rotate") {
      void rotate();
      return;
    }
    if (pendingConfirm === "revoke") {
      void revoke();
    }
  }

  const keyRevoked = apiKey?.status === "REVOKED";
  const headerActionLabel = apiKey && !keyRevoked ? "Rotate key" : "Create API key";

  return (
    <DashboardShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Developer key</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">API key</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Manage the key used to send SLAI-billed AI requests.</p>
          </div>
          {apiKey && !keyRevoked ? (
            <button className={primaryButton} type="button" onClick={() => setPendingConfirm("rotate")} disabled={actionLoading}>{headerActionLabel}</button>
          ) : (
            <button className={primaryButton} type="button" onClick={focusCreateForm}>{headerActionLabel}</button>
          )}
        </div>
      </section>

      {loading ? <div className="mt-8"><LoadingState label="Loading API key" /></div> : null}
      {error ? <div className="mt-8"><ErrorState message={error} onRetry={load} /></div> : null}

      {!loading && !error ? (
        <>
          <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <SummaryCard label="Key status" value={keyStatusLabel(apiKey)} hint="Current API access state">
              <Badge dot tone={apiKey ? statusTone(apiKey.status) : "neutral"}>{keyStatusLabel(apiKey)}</Badge>
            </SummaryCard>
            <SummaryCard label="Key prefix" value={apiKey?.key_prefix ?? "-"} hint="Safe display identifier" />
            <SummaryCard label="Last used" value={safeDate(apiKey?.last_used_at)} hint="From synced key metadata" />
            <SummaryCard label="Mode" value={modeLabel(apiKey)} hint="SLAI-billed API access" />
          </section>

          <div className="mt-6 space-y-3">
            {notice ? <Alert tone="success">{notice}</Alert> : null}
            <RawKeyRevealCard rawKey={rawKey} onDismiss={() => setRawKey(null)} />
          </div>

          <section className="mt-7 grid gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,0.85fr)] 2xl:grid-cols-[minmax(0,1.45fr)_minmax(380px,0.85fr)]">
            <div className="space-y-6">
              <CurrentKeyCard
                apiKey={apiKey}
                actionLoading={actionLoading}
                pendingConfirm={pendingConfirm}
                onCancelConfirm={() => setPendingConfirm(null)}
                onConfirm={confirmPendingAction}
                onCreateFocus={focusCreateForm}
                onRevokeRequest={() => setPendingConfirm("revoke")}
                onRotateRequest={() => setPendingConfirm("rotate")}
              />
              <QuickstartCard rawKey={rawKey} />
            </div>
            <div className="space-y-6">
              <CreateOrSecurityCard
                apiKey={apiKey}
                actionLoading={actionLoading}
                createFormId={createFormId}
                name={name}
                onCreate={create}
                setName={setName}
              />
              <SecurityGuidanceCard />
            </div>
          </section>
        </>
      ) : null}
    </DashboardShell>
  );
}
