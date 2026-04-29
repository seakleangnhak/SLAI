"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type AuditLogFilter, type AuditLogItem, type AuditLogListResponse } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatDateTime, formatUnits, truncateId } from "@/lib/format";

const LIMIT = 50;

const emptyFormFilters: AuditLogFilter = {
  admin_id: "",
  action: "",
  target_type: "",
  target_id: "",
  from: "",
  to: "",
  limit: LIMIT,
  offset: 0
};

const emptyRequestFilters: AuditLogFilter = { limit: LIMIT, offset: 0 };

type AuditSummary = {
  uniqueAdmins: number;
  uniqueTargetTypes: number;
  latestEvent: string | null;
};

export default function AdminAuditPage() {
  const [data, setData] = useState<AuditLogListResponse | null>(null);
  const [filters, setFilters] = useState<AuditLogFilter>(emptyRequestFilters);
  const [formFilters, setFormFilters] = useState<AuditLogFilter>(emptyFormFilters);
  const [selectedLog, setSelectedLog] = useState<AuditLogItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filterError, setFilterError] = useState<string | null>(null);

  const items = data?.items ?? [];
  const activeFilters = useMemo(() => activeFilterLabels(filters), [filters]);
  const hasActiveFilters = activeFilters.length > 0;
  const summary = useMemo(() => summarizeAuditLogs(items), [items]);

  function load(nextFilters: AuditLogFilter = filters) {
    const normalized = normalizeRequestFilters(nextFilters);
    setLoading(true);
    setError(null);
    api.admin.auditLogs
      .list(normalized)
      .then((response) => {
        setData(response);
        setFilters(normalized);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load(emptyRequestFilters);
    // Initial load only. Filter, quick-range, refresh, and pagination drive later requests.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function applyFilters(event: FormEvent) {
    event.preventDefault();
    const validationError = validateRange(formFilters);
    if (validationError) {
      setFilterError(validationError);
      return;
    }
    setFilterError(null);
    load(formFiltersToRequest(formFilters, 0));
  }

  function clearFilters() {
    setFormFilters(emptyFormFilters);
    setFilterError(null);
    load(emptyRequestFilters);
  }

  function applyQuickRange(days: number) {
    const now = new Date();
    const from = new Date(now.getTime() - days * 24 * 60 * 60 * 1000);
    const nextForm = {
      ...formFilters,
      from: toDatetimeLocalValue(from),
      to: toDatetimeLocalValue(now),
      limit: LIMIT,
      offset: 0
    };
    setFormFilters(nextForm);
    setFilterError(null);
    load(formFiltersToRequest(nextForm, 0));
  }

  function refresh() {
    load(filters);
  }

  function goToOffset(offset: number) {
    load({ ...filters, limit: LIMIT, offset: Math.max(0, offset) });
  }

  return (
    <AdminShell>
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">Admin audit</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-0.02em] text-slate-950">Audit logs</h1>
          <p className="mt-1 text-sm text-slate-500">Track admin actions, sensitive changes, and operational events.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="secondary" onClick={refresh} disabled={loading}>Refresh</Button>
          {hasActiveFilters ? <Button type="button" variant="secondary" onClick={clearFilters} disabled={loading}>Clear filters</Button> : null}
        </div>
      </div>

      <section className="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <AuditSummaryCard label="Events" value={formatUnits(data?.total ?? 0)} hint="Matching current filters" tone="blue" />
        <AuditSummaryCard label="Admin actions" value={formatUnits(summary.uniqueAdmins)} hint="Unique admins on page" tone="green" />
        <AuditSummaryCard label="Target types" value={formatUnits(summary.uniqueTargetTypes)} hint="Unique target types on page" tone="purple" />
        <AuditSummaryCard label="Latest event" value={summary.latestEvent ? formatDateTime(summary.latestEvent) : "-"} hint="Current result page" tone="slate" />
      </section>

      <Card className="mt-6 p-4">
        <CardHeader className="mb-4">
          <div>
            <CardTitle>Filters</CardTitle>
            <CardDescription>Filter activity by admin, action, target, and time range.</CardDescription>
          </div>
        </CardHeader>
        <form className="space-y-4" onSubmit={applyFilters}>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <FilterInput
              label="Action"
              onChange={(value) => setFormFilters((current) => ({ ...current, action: value }))}
              placeholder="manual_topup_created"
              value={formFilters.action ?? ""}
            />
            <FilterInput
              label="Target type"
              onChange={(value) => setFormFilters((current) => ({ ...current, target_type: value }))}
              placeholder="user, api_key, payment"
              value={formFilters.target_type ?? ""}
            />
            <FilterInput
              label="Admin ID"
              onChange={(value) => setFormFilters((current) => ({ ...current, admin_id: value }))}
              placeholder="admin uuid"
              value={formFilters.admin_id ?? ""}
            />
            <FilterInput
              label="Target ID"
              onChange={(value) => setFormFilters((current) => ({ ...current, target_id: value }))}
              placeholder="target uuid"
              value={formFilters.target_id ?? ""}
            />
          </div>

          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_auto_auto] xl:items-end">
            <FilterInput
              label="From"
              onChange={(value) => setFormFilters((current) => ({ ...current, from: value }))}
              type="datetime-local"
              value={formFilters.from ?? ""}
            />
            <FilterInput
              label="To"
              onChange={(value) => setFormFilters((current) => ({ ...current, to: value }))}
              type="datetime-local"
              value={formFilters.to ?? ""}
            />
            <div className="flex flex-wrap items-end gap-2">
              <Button type="button" variant="ghost" className="min-h-10 px-3" disabled={loading} onClick={() => applyQuickRange(1)}>Last 24h</Button>
              <Button type="button" variant="ghost" className="min-h-10 px-3" disabled={loading} onClick={() => applyQuickRange(7)}>Last 7d</Button>
              <Button type="button" variant="ghost" className="min-h-10 px-3" disabled={loading} onClick={() => applyQuickRange(30)}>Last 30d</Button>
            </div>
            <div className="flex gap-2">
              <Button className="flex-1" type="submit" disabled={loading}>Apply</Button>
              <Button className="flex-1" type="button" variant="secondary" onClick={clearFilters} disabled={loading && !hasActiveFilters}>Clear</Button>
            </div>
          </div>

          {filterError ? <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{filterError}</p> : null}

          <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
            <span className="font-semibold uppercase tracking-[0.08em]">Active filters</span>
            {hasActiveFilters ? (
              activeFilters.map((filter) => <span className="rounded-md border border-slate-200 bg-slate-50 px-2 py-1 font-mono text-slate-700" key={filter}>{filter}</span>)
            ) : (
              <span className="rounded-md border border-slate-200 bg-white px-2 py-1 text-slate-500">No active filters</span>
            )}
          </div>
        </form>
      </Card>

      <div className="mt-6">
        {loading ? <LoadingState label="Loading audit logs" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(filters)} /> : null}
        {!loading && !error && items.length === 0 ? <AuditEmptyState filtered={hasActiveFilters} onClear={clearFilters} /> : null}
        {!loading && !error && items.length > 0 ? (
          <>
            <AuditTable items={items} onSelect={setSelectedLog} />
            <Pagination
              count={items.length}
              limit={data?.limit ?? LIMIT}
              offset={data?.offset ?? 0}
              total={data?.total ?? 0}
              loading={loading}
              onNext={() => goToOffset((data?.offset ?? 0) + (data?.limit ?? LIMIT))}
              onPrevious={() => goToOffset((data?.offset ?? 0) - (data?.limit ?? LIMIT))}
            />
          </>
        ) : null}
      </div>

      {selectedLog ? <AuditDetailsDrawer log={selectedLog} onClose={() => setSelectedLog(null)} /> : null}
    </AdminShell>
  );
}

function AuditTable({ items, onSelect }: { items: AuditLogItem[]; onSelect: (item: AuditLogItem) => void }) {
  return (
    <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="sticky top-0 bg-slate-50/95 backdrop-blur">
            <tr>
              <DenseTh>Time</DenseTh>
              <DenseTh>Admin</DenseTh>
              <DenseTh>Action</DenseTh>
              <DenseTh>Target</DenseTh>
              <DenseTh>Metadata</DenseTh>
              <DenseTh>Actions</DenseTh>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {items.map((log) => (
              <tr className="transition-colors hover:bg-slate-50" key={log.id}>
                <DenseTd>
                  <p className="font-medium text-slate-900">{formatDateTime(log.created_at)}</p>
                  <p className="mt-1 font-mono text-xs text-slate-400">{truncateId(log.id)}</p>
                </DenseTd>
                <DenseTd>
                  <p className="font-medium text-slate-950">{log.admin_email}</p>
                  <p className="mt-1 font-mono text-xs text-slate-400">{truncateId(log.admin_id)}</p>
                </DenseTd>
                <DenseTd>
                  <Badge dot tone={auditActionTone(log.action)}>{formatActionLabel(log.action)}</Badge>
                  <p className="mt-1 max-w-56 truncate font-mono text-xs text-slate-400">{log.action}</p>
                </DenseTd>
                <DenseTd>
                  <div className="flex flex-col items-start gap-1">
                    <Badge dot tone={targetTone(log.target_type)}>{log.target_type ?? "none"}</Badge>
                    <span className="max-w-56 truncate font-mono text-xs text-slate-500">{truncateId(log.target_id)}</span>
                  </div>
                </DenseTd>
                <DenseTd>
                  <MetadataPreview metadata={log.metadata} />
                </DenseTd>
                <DenseTd>
                  <button
                    className="inline-flex min-h-8 items-center rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:border-blue-200 hover:text-blue-700"
                    onClick={() => onSelect(log)}
                    type="button"
                  >
                    View details
                  </button>
                </DenseTd>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function AuditDetailsDrawer({ log, onClose }: { log: AuditLogItem; onClose: () => void }) {
  const metadata = JSON.stringify(redactSensitivePayload(log.metadata ?? {}), null, 2);

  return (
    <div className="fixed inset-0 z-50 bg-slate-950/40 backdrop-blur-sm" onClick={onClose}>
      <div className="ml-auto flex h-full w-full max-w-[560px] flex-col bg-white shadow-2xl ring-1 ring-slate-200" onClick={(event) => event.stopPropagation()}>
        <div className="sticky top-0 z-10 border-b border-slate-200 bg-white/95 px-5 py-4 backdrop-blur">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">Audit event</p>
              <h2 className="mt-2 text-xl font-semibold text-slate-950">{formatActionLabel(log.action)}</h2>
              <p className="mt-1 text-sm text-slate-500">{formatDateTime(log.created_at)}</p>
            </div>
            <Button type="button" variant="ghost" onClick={onClose}>Close</Button>
          </div>
        </div>

        <div className="flex-1 space-y-5 overflow-y-auto px-5 py-5">
          <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">Action</p>
                <p className="mt-2 font-mono text-sm text-slate-950">{log.action}</p>
              </div>
              <Badge dot tone={auditActionTone(log.action)}>{formatActionLabel(log.action)}</Badge>
            </div>
          </section>

          <section className="grid gap-3 sm:grid-cols-2">
            <DetailField label="Audit log ID" value={log.id} mono />
            <DetailField label="Created at" value={formatDateTime(log.created_at)} />
            <DetailField label="Admin email" value={log.admin_email} />
            <DetailField label="Admin ID" value={log.admin_id} mono />
            <DetailField label="Target type" value={log.target_type ?? "-"} />
            <DetailField label="Target ID" value={log.target_id ?? "-"} mono />
          </section>

          <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
            <div className="mb-3">
              <h3 className="text-sm font-semibold text-slate-950">Metadata</h3>
              <p className="mt-1 text-xs text-slate-500">Sensitive metadata keys are defensively redacted in the browser.</p>
            </div>
            <pre className="max-h-[30rem] overflow-auto rounded-lg bg-slate-950 p-3 text-xs leading-5 text-slate-100">{metadata}</pre>
          </section>
        </div>
      </div>
    </div>
  );
}

function MetadataPreview({ metadata }: { metadata: Record<string, unknown> }) {
  const redacted = redactSensitivePayload(metadata ?? {});
  const keys = Object.keys(redacted as Record<string, unknown>);
  if (keys.length === 0) {
    return <span className="text-slate-400">No metadata</span>;
  }
  const preview = JSON.stringify(redacted);
  return (
    <div className="max-w-sm">
      <p className="text-xs text-slate-500">{formatUnits(keys.length)} field{keys.length === 1 ? "" : "s"}</p>
      <p className="mt-1 truncate font-mono text-xs text-slate-700">{preview}</p>
    </div>
  );
}

function AuditSummaryCard({ label, value, hint, tone }: { label: string; value: string; hint: string; tone: "blue" | "green" | "purple" | "slate" }) {
  const marker = {
    blue: "bg-blue-500",
    green: "bg-emerald-500",
    purple: "bg-violet-500",
    slate: "bg-slate-300"
  }[tone];

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</p>
        <span className={cn("size-2 rounded-full", marker)} />
      </div>
      <p className="mt-3 text-2xl font-semibold tracking-[-0.02em] text-slate-950">{value}</p>
      <p className="mt-1 text-xs leading-5 text-slate-500">{hint}</p>
    </Card>
  );
}

function AuditEmptyState({ filtered, onClear }: { filtered: boolean; onClear: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-10 text-center shadow-sm">
      <h3 className="text-base font-semibold text-slate-950">{filtered ? "No audit logs match these filters" : "No audit logs yet"}</h3>
      <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-500">
        {filtered ? "Adjust or clear filters to review more admin activity." : "Admin actions will appear here after changes are made."}
      </p>
      {filtered ? <Button className="mt-4" type="button" variant="secondary" onClick={onClear}>Clear filters</Button> : null}
    </div>
  );
}

function Pagination({
  count,
  limit,
  loading,
  offset,
  onNext,
  onPrevious,
  total
}: {
  count: number;
  limit: number;
  loading: boolean;
  offset: number;
  onNext: () => void;
  onPrevious: () => void;
  total: number;
}) {
  const start = count === 0 ? 0 : offset + 1;
  const end = offset + count;

  return (
    <div className="mt-4 flex flex-col gap-3 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm text-slate-500 shadow-sm sm:flex-row sm:items-center sm:justify-between">
      <span>Showing {formatUnits(start)}-{formatUnits(end)} of {formatUnits(total)}</span>
      <div className="flex gap-2">
        <Button type="button" variant="secondary" disabled={offset === 0 || loading} onClick={onPrevious}>Previous</Button>
        <Button type="button" variant="secondary" disabled={offset + count >= total || count < limit || loading} onClick={onNext}>Next</Button>
      </div>
    </div>
  );
}

function FilterInput({ label, onChange, type = "text", value, placeholder }: { label: string; onChange: (value: string) => void; type?: string; value: string; placeholder?: string }) {
  return (
    <label className="block">
      <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">{label}</span>
      <input
        className="mt-2 h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-950 outline-none ring-blue-600/20 transition placeholder:text-slate-400 focus:border-blue-600 focus:ring-4"
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        type={type}
        value={value}
      />
    </label>
  );
}

function DetailField({ label, mono = false, value }: { label: string; mono?: boolean; value: string | number }) {
  return (
    <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
      <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">{label}</p>
      <p className={cn("mt-1 break-all text-sm text-slate-900", mono && "font-mono text-xs")}>{String(value)}</p>
    </div>
  );
}

function DenseTh({ children }: { children: React.ReactNode }) {
  return <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">{children}</th>;
}

function DenseTd({ children }: { children: React.ReactNode }) {
  return <td className="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{children}</td>;
}

function summarizeAuditLogs(items: AuditLogItem[]): AuditSummary {
  const adminIDs = new Set(items.map((item) => item.admin_id).filter(Boolean));
  const targetTypes = new Set(items.map((item) => item.target_type).filter(Boolean));
  const latestEvent = items.reduce<string | null>((latest, item) => {
    if (!latest) {
      return item.created_at;
    }
    return new Date(item.created_at).getTime() > new Date(latest).getTime() ? item.created_at : latest;
  }, null);
  return { uniqueAdmins: adminIDs.size, uniqueTargetTypes: targetTypes.size, latestEvent };
}

function formFiltersToRequest(form: AuditLogFilter, offset: number): AuditLogFilter {
  const request: AuditLogFilter = {
    admin_id: trimmed(form.admin_id),
    action: trimmed(form.action),
    target_type: trimmed(form.target_type),
    target_id: trimmed(form.target_id),
    limit: LIMIT,
    offset
  };
  if (form.from) {
    request.from = new Date(form.from).toISOString();
  }
  if (form.to) {
    request.to = new Date(form.to).toISOString();
  }
  return normalizeRequestFilters(request);
}

function normalizeRequestFilters(filters: AuditLogFilter): AuditLogFilter {
  return {
    admin_id: trimmed(filters.admin_id),
    action: trimmed(filters.action),
    target_type: trimmed(filters.target_type),
    target_id: trimmed(filters.target_id),
    from: trimmed(filters.from),
    to: trimmed(filters.to),
    limit: filters.limit ?? LIMIT,
    offset: filters.offset ?? 0
  };
}

function validateRange(filters: AuditLogFilter) {
  if (!filters.from || !filters.to) {
    return null;
  }
  const from = new Date(filters.from).getTime();
  const to = new Date(filters.to).getTime();
  if (!Number.isFinite(from) || !Number.isFinite(to)) {
    return "Use valid from and to date/time values.";
  }
  if (from > to) {
    return "From date/time must be before or equal to To date/time.";
  }
  return null;
}

function activeFilterLabels(filters: AuditLogFilter) {
  const labels: string[] = [];
  if (filters.admin_id) labels.push("admin:" + truncateId(filters.admin_id));
  if (filters.action) labels.push("action:" + filters.action);
  if (filters.target_type) labels.push("target:" + filters.target_type);
  if (filters.target_id) labels.push("target_id:" + truncateId(filters.target_id));
  if (filters.from) labels.push("from:" + formatDateTime(filters.from));
  if (filters.to) labels.push("to:" + formatDateTime(filters.to));
  return labels;
}

function auditActionTone(action: string) {
  const normalized = action.toLowerCase();
  if (normalized.includes("revoke") || normalized.includes("suspend") || normalized.includes("delete")) {
    return "red" as const;
  }
  if (normalized.includes("topup") || normalized.includes("payment")) {
    return "green" as const;
  }
  if (normalized.includes("adjust")) {
    return "yellow" as const;
  }
  if (normalized.includes("user") || normalized.includes("api_key") || normalized.includes("key")) {
    return "blue" as const;
  }
  if (normalized.includes("package")) {
    return "purple" as const;
  }
  return "neutral" as const;
}

function targetTone(targetType?: string | null) {
  switch (targetType) {
    case "user":
      return "blue" as const;
    case "api_key":
      return "red" as const;
    case "payment":
      return "green" as const;
    case "package":
    case "credit_package":
      return "purple" as const;
    default:
      return "neutral" as const;
  }
}

function formatActionLabel(action: string) {
  return action
    .split("_")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function redactSensitivePayload(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => redactSensitivePayload(item));
  }
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(([key, nested]) => [
        key,
        isSensitiveKey(key) ? "[redacted]" : redactSensitivePayload(nested)
      ])
    );
  }
  return value;
}

function isSensitiveKey(key: string) {
  return /(password|token|secret|key|authorization|pepper)/i.test(key);
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

function trimmed(value?: string | null) {
  const next = value?.trim();
  return next ? next : undefined;
}
