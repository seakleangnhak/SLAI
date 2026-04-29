"use client";

import { FormEvent, useEffect, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
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
const EMPTY_FILTERS: AuditLogFilter = { limit: LIMIT, offset: 0 };

export default function AdminAuditPage() {
  const [data, setData] = useState<AuditLogListResponse | null>(null);
  const [filters, setFilters] = useState<AuditLogFilter>(EMPTY_FILTERS);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load(nextFilters = filters) {
    const validationError = validateRange(nextFilters);
    if (validationError) {
      setError(validationError);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);
    api.admin.auditLogs
      .list(toRequestFilters(nextFilters))
      .then((response) => {
        setData(response);
        setFilters(nextFilters);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => load(EMPTY_FILTERS), []);

  function submit(event: FormEvent) {
    event.preventDefault();
    load({ ...filters, limit: LIMIT, offset: 0 });
  }

  function clearFilters() {
    setFilters(EMPTY_FILTERS);
    load(EMPTY_FILTERS);
  }

  function applyQuickRange(days: number) {
    const now = new Date();
    const from = new Date(now.getTime() - days * 24 * 60 * 60 * 1000);
    const nextFilters = {
      ...filters,
      from: toDatetimeLocalValue(from),
      to: toDatetimeLocalValue(now),
      limit: LIMIT,
      offset: 0
    };
    setFilters(nextFilters);
    load(nextFilters);
  }

  const items = data?.items ?? [];
  const activeFilters = activeFilterLabels(filters);

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
            <CardDescription>Filter admin audit events by admin, action, target, and time range.</CardDescription>
          </div>
        </CardHeader>
        <form className="grid gap-4 md:grid-cols-2 xl:grid-cols-4" onSubmit={submit}>
          <Input label="Admin ID" value={filters.admin_id ?? ""} onChange={(event) => setFilters({ ...filters, admin_id: event.target.value })} />
          <Input label="Action" value={filters.action ?? ""} onChange={(event) => setFilters({ ...filters, action: event.target.value })} />
          <Input label="Target type" value={filters.target_type ?? ""} onChange={(event) => setFilters({ ...filters, target_type: event.target.value })} />
          <Input label="Target ID" value={filters.target_id ?? ""} onChange={(event) => setFilters({ ...filters, target_id: event.target.value })} />
          <Input label="From date/time" type="datetime-local" value={filters.from ?? ""} onChange={(event) => setFilters({ ...filters, from: event.target.value })} />
          <Input label="To date/time" type="datetime-local" value={filters.to ?? ""} onChange={(event) => setFilters({ ...filters, to: event.target.value })} />
          <div className="flex flex-wrap items-end gap-2 self-end xl:col-span-2">
            <Button type="submit" disabled={loading}>Apply filters</Button>
            <Button type="button" variant="secondary" disabled={loading} onClick={clearFilters}>Clear filters</Button>
          </div>
        </form>
        <div className="mt-5 flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium text-slate-600">Quick ranges</span>
          <Button type="button" variant="ghost" className="min-h-8 px-3 py-1" disabled={loading} onClick={() => applyQuickRange(1)}>Last 24h</Button>
          <Button type="button" variant="ghost" className="min-h-8 px-3 py-1" disabled={loading} onClick={() => applyQuickRange(7)}>Last 7d</Button>
          <Button type="button" variant="ghost" className="min-h-8 px-3 py-1" disabled={loading} onClick={() => applyQuickRange(30)}>Last 30d</Button>
        </div>
        <div className="mt-5 flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium text-slate-600">Active filters</span>
          {activeFilters.length > 0 ? (
            activeFilters.map((label) => <Badge key={label} tone="cyan">{label}</Badge>)
          ) : (
            <span className="text-sm text-slate-500">None</span>
          )}
        </div>
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

function toRequestFilters(filters: AuditLogFilter): AuditLogFilter {
  return {
    admin_id: normalizeFilterValue(filters.admin_id),
    action: normalizeFilterValue(filters.action),
    target_type: normalizeFilterValue(filters.target_type),
    target_id: normalizeFilterValue(filters.target_id),
    from: filters.from ? toISOString(filters.from) : undefined,
    to: filters.to ? toISOString(filters.to) : undefined,
    limit: filters.limit ?? LIMIT,
    offset: filters.offset ?? 0
  };
}

function normalizeFilterValue(value?: string) {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

function validateRange(filters: AuditLogFilter) {
  const from = filters.from ? new Date(filters.from) : null;
  const to = filters.to ? new Date(filters.to) : null;

  if (from && Number.isNaN(from.getTime())) {
    return "Enter a valid from date/time.";
  }
  if (to && Number.isNaN(to.getTime())) {
    return "Enter a valid to date/time.";
  }
  if (from && to && from > to) {
    return "From date/time must be before or equal to To date/time.";
  }
  return null;
}

function toISOString(value: string) {
  return new Date(value).toISOString();
}

function toDatetimeLocalValue(date: Date) {
  const year = date.getFullYear();
  const month = padDatePart(date.getMonth() + 1);
  const day = padDatePart(date.getDate());
  const hours = padDatePart(date.getHours());
  const minutes = padDatePart(date.getMinutes());
  return year + "-" + month + "-" + day + "T" + hours + ":" + minutes;
}

function padDatePart(value: number) {
  return String(value).padStart(2, "0");
}

function formatLocalDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return formatDateTime(date.toISOString());
}

function activeFilterLabels(filters: AuditLogFilter) {
  const labels: string[] = [];
  if (filters.admin_id?.trim()) {
    labels.push("Admin: " + filters.admin_id.trim());
  }
  if (filters.action?.trim()) {
    labels.push("Action: " + filters.action.trim());
  }
  if (filters.target_type?.trim()) {
    labels.push("Target type: " + filters.target_type.trim());
  }
  if (filters.target_id?.trim()) {
    labels.push("Target ID: " + filters.target_id.trim());
  }
  if (filters.from) {
    labels.push("From: " + formatLocalDateTime(filters.from));
  }
  if (filters.to) {
    labels.push("To: " + formatLocalDateTime(filters.to));
  }
  return labels;
}
