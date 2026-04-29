"use client";

import { FormEvent, useEffect, useState } from "react";

import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { CopyButton } from "@/components/CopyButton";
import { DashboardShell } from "@/components/DashboardShell";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, isNotFound, readableError, type PublicAPIKey } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

export default function APIKeyPage() {
  const [apiKey, setApiKey] = useState<PublicAPIKey | null>(null);
  const [rawKey, setRawKey] = useState<string | null>(null);
  const [name, setName] = useState("Default key");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

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

  async function create(event: FormEvent) {
    event.preventDefault();
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.apiKeys.create(name);
      setApiKey(response.api_key);
      setRawKey(response.raw_api_key);
      setNotice("API key created. Save the raw key now; it will not be shown again.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function rotate() {
    if (!window.confirm("Rotate this key? The current key will be revoked and the new raw key will be shown once.")) {
      return;
    }
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.apiKeys.rotate();
      setApiKey(response.api_key);
      setRawKey(response.raw_api_key);
      setNotice("API key rotated. Save the new raw key now; it will not be shown again.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function revoke() {
    if (!window.confirm("Revoke this key? Applications using it will stop working.")) {
      return;
    }
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.apiKeys.revoke();
      setApiKey(response.api_key);
      setRawKey(null);
      setNotice("API key revoked.");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  return (
    <DashboardShell>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Developer key</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">API key</h1>
        </div>
      </div>

      <div className="mt-8 grid gap-6 lg:grid-cols-[1fr_0.85fr]">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>Current key</CardTitle>
              <CardDescription>Only the display prefix and metadata are stored by SLAI.</CardDescription>
            </div>
            {apiKey ? <Badge tone={statusTone(apiKey.status)}>{apiKey.status}</Badge> : null}
          </CardHeader>

          {loading ? <LoadingState label="Loading API key" /> : null}
          {error ? <ErrorState message={error} onRetry={load} /> : null}
          {!loading && !error && !apiKey ? <EmptyState title="No API key" message="Create your MVP key to call OmniRoute directly." /> : null}

          {apiKey ? (
            <dl className="grid gap-4 text-sm sm:grid-cols-2">
              <div>
                <dt className="font-medium text-slate-500">Name</dt>
                <dd className="mt-1 text-slate-950">{apiKey.name}</dd>
              </div>
              <div>
                <dt className="font-medium text-slate-500">Prefix</dt>
                <dd className="mt-1 font-mono text-slate-950">{apiKey.key_prefix}</dd>
              </div>
              <div>
                <dt className="font-medium text-slate-500">Created</dt>
                <dd className="mt-1 text-slate-950">{formatDateTime(apiKey.created_at)}</dd>
              </div>
              <div>
                <dt className="font-medium text-slate-500">OmniRoute linked</dt>
                <dd className="mt-1 text-slate-950">{apiKey.omniroute_linked ? "Yes" : "Local/dev mode"}</dd>
              </div>
            </dl>
          ) : null}

          {rawKey ? (
            <div className="mt-6 rounded-lg border border-amber-200 bg-amber-50 p-4">
              <p className="text-sm font-semibold text-amber-900">Raw API key shown once</p>
              <p className="mt-1 text-sm text-amber-800">Copy it now. SLAI stores only a hash and cannot show it again.</p>
              <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center">
                <code className="min-w-0 flex-1 break-all rounded-md bg-white px-3 py-2 text-sm text-slate-950">{rawKey}</code>
                <CopyButton value={rawKey} />
              </div>
            </div>
          ) : null}

          {notice ? <p className="mt-4 rounded-md bg-cyan-50 px-3 py-2 text-sm text-cyan-800">{notice}</p> : null}

          {apiKey ? (
            <div className="mt-6 flex flex-wrap gap-3">
              <Button type="button" variant="secondary" onClick={rotate} disabled={actionLoading}>
                Rotate key
              </Button>
              <Button type="button" variant="danger" onClick={revoke} disabled={actionLoading || apiKey.status === "REVOKED"}>
                Revoke key
              </Button>
            </div>
          ) : null}
        </Card>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>Create key</CardTitle>
              <CardDescription>MVP allows one active key per user.</CardDescription>
            </div>
          </CardHeader>
          <form className="space-y-4" onSubmit={create}>
            <Input label="Key name" value={name} onChange={(event) => setName(event.target.value)} required />
            <Button type="submit" disabled={actionLoading || !!apiKey?.status && apiKey.status === "ACTIVE"}>
              {actionLoading ? "Working" : "Create key"}
            </Button>
            {apiKey?.status === "ACTIVE" ? (
              <p className="text-sm text-slate-500">Revoke or rotate the current active key before creating another one.</p>
            ) : null}
          </form>
        </Card>
      </div>
    </DashboardShell>
  );
}
