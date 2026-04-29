"use client";

import { FormEvent, useEffect, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { Table, Td, Th } from "@/components/Table";
import { api, readableError, type UsageEvent, type UsageFilter } from "@/lib/api";
import { formatDateTime, formatUnits } from "@/lib/format";

const LIMIT = 50;

export default function AdminUsagePage() {
  const [usage, setUsage] = useState<UsageEvent[]>([]);
  const [filters, setFilters] = useState<UsageFilter>({ limit: LIMIT, offset: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load(nextFilters = filters) {
    setLoading(true);
    setError(null);
    api.admin.usage
      .list(nextFilters)
      .then((response) => {
        setUsage(response.usage);
        setFilters(nextFilters);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => load({ limit: LIMIT, offset: 0 }), []);

  function submit(event: FormEvent) {
    event.preventDefault();
    load({ ...filters, limit: LIMIT, offset: 0 });
  }

  return (
    <AdminShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin usage</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Usage events</h1>
      </div>

      <Card className="mt-8">
        <CardHeader>
          <div>
            <CardTitle>Filters</CardTitle>
            <CardDescription>Filter synced OmniRoute and mock usage events.</CardDescription>
          </div>
        </CardHeader>
        <form className="grid gap-4 md:grid-cols-3" onSubmit={submit}>
          <Input label="User ID" value={filters.user_id ?? ""} onChange={(event) => setFilters({ ...filters, user_id: event.target.value })} />
          <Input label="API key ID" value={filters.api_key_id ?? ""} onChange={(event) => setFilters({ ...filters, api_key_id: event.target.value })} />
          <Input label="Status" value={filters.status ?? ""} onChange={(event) => setFilters({ ...filters, status: event.target.value })} />
          <Input label="Model" value={filters.model ?? ""} onChange={(event) => setFilters({ ...filters, model: event.target.value })} />
          <Input label="Provider" value={filters.provider ?? ""} onChange={(event) => setFilters({ ...filters, provider: event.target.value })} />
          <div className="self-end"><Button type="submit" disabled={loading}>Apply filters</Button></div>
        </form>
      </Card>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading usage" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(filters)} /> : null}
        {!loading && !error && usage.length === 0 ? <EmptyState title="No usage events" message="No usage matched the selected filters." /> : null}
        {!loading && !error && usage.length > 0 ? (
          <>
            <Table>
              <thead className="bg-slate-50"><tr><Th>User</Th><Th>API key</Th><Th>Model</Th><Th>Provider</Th><Th>Tokens</Th><Th>Cost</Th><Th>Status</Th><Th>Occurred</Th></tr></thead>
              <tbody className="divide-y divide-slate-100">
                {usage.map((event) => (
                  <tr key={event.id}>
                    <Td className="font-mono text-xs">{event.user_id}</Td>
                    <Td className="font-mono text-xs">{event.api_key_id}</Td>
                    <Td>{event.model ?? "-"}</Td>
                    <Td>{event.provider ?? "-"}</Td>
                    <Td>{formatUnits(event.total_tokens)}</Td>
                    <Td>{formatUnits(event.cost_units)}</Td>
                    <Td><Badge tone={statusTone(event.status)}>{event.status}</Badge></Td>
                    <Td>{formatDateTime(event.occurred_at)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <div className="mt-4 flex items-center justify-between">
              <Button type="button" variant="secondary" disabled={(filters.offset ?? 0) === 0 || loading} onClick={() => load({ ...filters, offset: Math.max(0, (filters.offset ?? 0) - LIMIT) })}>
                Previous
              </Button>
              <span className="text-sm text-slate-500">Offset {filters.offset ?? 0}</span>
              <Button type="button" variant="secondary" disabled={usage.length < LIMIT || loading} onClick={() => load({ ...filters, offset: (filters.offset ?? 0) + LIMIT })}>
                Next
              </Button>
            </div>
          </>
        ) : null}
      </div>
    </AdminShell>
  );
}
