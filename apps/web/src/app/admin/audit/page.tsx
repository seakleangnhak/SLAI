"use client";

import { FormEvent, useEffect, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { Table, Td, Th } from "@/components/Table";
import {
  api,
  readableError,
  type AuditLogFilter,
  type AuditLogItem,
  type AuditLogListResponse
} from "@/lib/api";
import { formatDateTime, formatUnits } from "@/lib/format";

const LIMIT = 50;

export default function AdminAuditPage() {
  const [data, setData] = useState<AuditLogListResponse | null>(null);
  const [filters, setFilters] = useState<AuditLogFilter>({ limit: LIMIT, offset: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load(nextFilters = filters) {
    setLoading(true);
    setError(null);
    api.admin.auditLogs
      .list(nextFilters)
      .then((response) => {
        setData(response);
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

  const items = data?.items ?? [];

  return (
    <AdminShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin audit</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Audit Logs</h1>
      </div>

      <Card className="mt-8">
        <CardHeader>
          <div>
            <CardTitle>Filters</CardTitle>
            <CardDescription>Filter admin audit events by action and target.</CardDescription>
          </div>
        </CardHeader>
        <form className="grid gap-4 md:grid-cols-4" onSubmit={submit}>
          <Input label="Action" value={filters.action ?? ""} onChange={(event) => setFilters({ ...filters, action: event.target.value })} />
          <Input label="Target type" value={filters.target_type ?? ""} onChange={(event) => setFilters({ ...filters, target_type: event.target.value })} />
          <Input label="Target ID" value={filters.target_id ?? ""} onChange={(event) => setFilters({ ...filters, target_id: event.target.value })} />
          <div className="self-end">
            <Button type="submit" disabled={loading}>Apply filters</Button>
          </div>
        </form>
      </Card>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading audit logs" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(filters)} /> : null}
        {!loading && !error && items.length === 0 ? <EmptyState title="No audit logs" message="No admin audit logs matched the selected filters." /> : null}
        {!loading && !error && items.length > 0 ? (
          <>
            <Table>
              <thead className="bg-slate-50">
                <tr>
                  <Th>Date</Th>
                  <Th>Admin</Th>
                  <Th>Action</Th>
                  <Th>Target Type</Th>
                  <Th>Target ID</Th>
                  <Th>Metadata</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {items.map((log) => (
                  <tr key={log.id}>
                    <Td>{formatDateTime(log.created_at)}</Td>
                    <Td>
                      <div className="font-medium text-slate-950">{log.admin_email}</div>
                      <div className="mt-1 max-w-48 truncate font-mono text-xs text-slate-400">{log.admin_id}</div>
                    </Td>
                    <Td className="font-mono text-xs">{log.action}</Td>
                    <Td>{log.target_type ?? "-"}</Td>
                    <Td className="max-w-56 truncate font-mono text-xs">{log.target_id ?? "-"}</Td>
                    <Td className="min-w-64 whitespace-normal">
                      <MetadataBlock log={log} />
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <div className="mt-4 flex items-center justify-between">
              <Button type="button" variant="secondary" disabled={(data?.offset ?? 0) === 0 || loading} onClick={() => load({ ...filters, offset: Math.max(0, (data?.offset ?? 0) - LIMIT) })}>
                Previous
              </Button>
              <span className="text-sm text-slate-500">Showing {items.length} of {formatUnits(data?.total ?? 0)} logs</span>
              <Button type="button" variant="secondary" disabled={items.length < LIMIT || loading} onClick={() => load({ ...filters, offset: (data?.offset ?? 0) + LIMIT })}>
                Next
              </Button>
            </div>
          </>
        ) : null}
      </div>
    </AdminShell>
  );
}

function MetadataBlock({ log }: { log: AuditLogItem }) {
  const metadata = log.metadata ?? {};
  const keys = Object.keys(metadata);

  if (keys.length === 0) {
    return <span className="text-slate-400">-</span>;
  }

  return (
    <details className="max-w-xl text-sm">
      <summary className="cursor-pointer font-semibold text-cyan-700">View JSON</summary>
<pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded-md bg-slate-950 p-3 text-xs leading-5 text-slate-50">{JSON.stringify(metadata, null, 2)}</pre>
    </details>
  );
}
