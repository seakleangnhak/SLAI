"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type UsageEvent, type UsageFilter } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactCredits, formatCompactUnits, formatCredits, formatDateTime, formatUnits, truncateId } from "@/lib/format";

const LIMIT = 50;
const usageStatuses = ["pending", "billed", "duplicate", "failed", "ignored"] as const;

type UsageFormFilters = {
  user_id?: string;
  api_key_id?: string;
  model?: string;
  provider?: string;
  status?: string;
  from?: string;
  to?: string;
};

type Summary = {
  billed: number;
  pending: number;
  failed: number;
  ignored: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  costUnits: number;
};

const emptyFormFilters: UsageFormFilters = {
  user_id: "",
  api_key_id: "",
  model: "",
  provider: "",
  status: "",
  from: "",
  to: ""
};

export default function AdminUsagePage() {
  const [usage, setUsage] = useState<UsageEvent[]>([]);
  const [filters, setFilters] = useState<UsageFilter>({ limit: LIMIT, offset: 0 });
  const [formFilters, setFormFilters] = useState<UsageFormFilters>(emptyFormFilters);
  const [selectedEvent, setSelectedEvent] = useState<UsageEvent | null>(null);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filterError, setFilterError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const summary = useMemo(() => summarizeUsage(usage), [usage]);
  const activeFilters = useMemo(() => activeFilterLabels(filters), [filters]);
  const hasActiveFilters = activeFilters.length > 0;

  function load(nextFilters: UsageFilter = filters) {
    const normalized = normalizeUsageFilter(nextFilters);
    setLoading(true);
    setError(null);
    api.admin.usage
      .list(normalized)
      .then((response) => {
        setUsage(response.usage);
        setFilters(normalized);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load({ limit: LIMIT, offset: 0 });
    // Run once on mount. Later loads are driven by filters, pagination, and refresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function applyFilters(event: FormEvent) {
    event.preventDefault();
    const validationError = validateDateRange(formFilters);
    if (validationError) {
      setFilterError(validationError);
      return;
    }
    setFilterError(null);
    setNotice(null);
    load(formFiltersToApiFilter(formFilters, 0));
  }

  function clearFilters() {
    setFormFilters(emptyFormFilters);
    setFilterError(null);
    setNotice(null);
    load({ limit: LIMIT, offset: 0 });
  }

  function refresh() {
    setNotice(null);
    load(filters);
  }

  async function syncUsage() {
    setSyncing(true);
    setError(null);
    setNotice(null);
    try {
      const response = await api.admin.usage.sync();
      setNotice(syncSummaryText(response.sync));
      load({ ...filters, offset: 0 });
    } catch (err) {
      setError(readableError(err));
    } finally {
      setSyncing(false);
    }
  }

  function goToOffset(offset: number) {
    setNotice(null);
    load({ ...filters, offset: Math.max(0, offset), limit: LIMIT });
  }

  return (
    <AdminShell>
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">Admin usage</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-0.02em] text-slate-950">Usage events</h1>
          <p className="mt-1 text-sm text-slate-500">Inspect OmniRoute usage, credit billing, and event status.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="secondary" onClick={refresh} disabled={loading || syncing}>Refresh</Button>
          <Button type="button" onClick={syncUsage} disabled={loading || syncing}>{syncing ? "Syncing" : "Sync usage"}</Button>
          <Link className="inline-flex min-h-10 items-center rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-semibold text-slate-800 transition hover:bg-slate-50" href="/admin/sync">
            Sync status
          </Link>
        </div>
      </div>

      <section className="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <UsageSummaryCard
          label="Events"
          value={formatUnits(usage.length)}
          hint={`${formatUnits(summary.billed)} billed / ${formatUnits(summary.pending)} pending`}
          scope="Current page"
          tone="blue"
        />
        <UsageSummaryCard
          label="Tokens"
          value={formatCompactUnits(summary.totalTokens)}
          hint={`${formatCompactUnits(summary.inputTokens)} input / ${formatCompactUnits(summary.outputTokens)} output`}
          scope="Current page"
          tone="slate"
        />
        <UsageSummaryCard
          label="Credit cost"
          value={formatCompactCredits(summary.costUnits)}
          hint="Deducted credits"
          scope="Current page"
          tone="green"
        />
        <UsageSummaryCard
          label="Exceptions"
          value={formatUnits(summary.failed + summary.ignored)}
          hint={`${formatUnits(summary.failed)} failed / ${formatUnits(summary.ignored)} ignored`}
          scope={summary.failed + summary.ignored > 0 ? "Needs review" : "OK"}
          tone={summary.failed + summary.ignored > 0 ? "red" : "green"}
        />
      </section>

      {notice ? <p className="mt-6 rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-sm text-blue-800">{notice}</p> : null}

      <Card className="mt-6 p-4">
        <CardHeader className="mb-4">
          <div>
            <CardTitle>Filters</CardTitle>
            <CardDescription>Filter synced OmniRoute call logs and local mock usage events.</CardDescription>
          </div>
        </CardHeader>
        <form className="space-y-4" onSubmit={applyFilters}>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <FilterInput
              label="User ID"
              onChange={(value) => setFormFilters((current) => ({ ...current, user_id: value }))}
              placeholder="user uuid"
              value={formFilters.user_id ?? ""}
            />
            <FilterInput
              label="API key ID"
              onChange={(value) => setFormFilters((current) => ({ ...current, api_key_id: value }))}
              placeholder="api key uuid"
              value={formFilters.api_key_id ?? ""}
            />
            <FilterInput
              label="Model"
              onChange={(value) => setFormFilters((current) => ({ ...current, model: value }))}
              placeholder="gpt-5.5"
              value={formFilters.model ?? ""}
            />
            <FilterInput
              label="Provider"
              onChange={(value) => setFormFilters((current) => ({ ...current, provider: value }))}
              placeholder="openai"
              value={formFilters.provider ?? ""}
            />
            <label className="block">
              <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">Status</span>
              <select
                className="mt-2 h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-950 outline-none ring-blue-600/20 transition focus:border-blue-600 focus:ring-4"
                onChange={(event) => setFormFilters((current) => ({ ...current, status: event.target.value }))}
                value={formFilters.status ?? ""}
              >
                <option value="">All</option>
                {usageStatuses.map((status) => <option key={status} value={status}>{status}</option>)}
              </select>
            </label>
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
            <div className="flex items-end gap-2">
              <Button className="flex-1" type="submit" disabled={loading}>Apply filters</Button>
              <Button className="flex-1" type="button" variant="secondary" onClick={clearFilters} disabled={loading && !hasActiveFilters}>Clear</Button>
            </div>
          </div>
          {filterError ? <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{filterError}</p> : null}
          {hasActiveFilters ? (
            <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
              <span className="font-semibold uppercase tracking-[0.08em]">Active filters</span>
              {activeFilters.map((filter) => (
                <span className="rounded-md border border-slate-200 bg-slate-50 px-2 py-1 font-mono text-slate-700" key={filter}>{filter}</span>
              ))}
            </div>
          ) : null}
        </form>
      </Card>

      <div className="mt-6">
        {loading ? <LoadingState label="Loading usage" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(filters)} /> : null}
        {!loading && !error && usage.length === 0 ? <UsageEmptyState filtered={hasActiveFilters} onClear={clearFilters} /> : null}
        {!loading && !error && usage.length > 0 ? (
          <>
            <UsageTable events={usage} onSelect={setSelectedEvent} />
            <Pagination
              count={usage.length}
              limit={LIMIT}
              offset={filters.offset ?? 0}
              onNext={() => goToOffset((filters.offset ?? 0) + LIMIT)}
              onPrevious={() => goToOffset((filters.offset ?? 0) - LIMIT)}
              loading={loading}
            />
          </>
        ) : null}
      </div>

      {selectedEvent ? <UsageDetailsDrawer event={selectedEvent} onClose={() => setSelectedEvent(null)} /> : null}
    </AdminShell>
  );
}

function UsageTable({ events, onSelect }: { events: UsageEvent[]; onSelect: (event: UsageEvent) => void }) {
  return (
    <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="sticky top-0 bg-slate-50/95 backdrop-blur">
            <tr>
              <DenseTh>Time</DenseTh>
              <DenseTh>User</DenseTh>
              <DenseTh>API key</DenseTh>
              <DenseTh>Model</DenseTh>
              <DenseTh>Provider</DenseTh>
              <DenseTh>Tokens</DenseTh>
              <DenseTh>Cost</DenseTh>
              <DenseTh>Status</DenseTh>
              <DenseTh>Source / External ID</DenseTh>
              <DenseTh>Actions</DenseTh>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {events.map((event) => (
              <tr className="transition-colors hover:bg-slate-50" key={event.id}>
                <DenseTd>
                  <p className="font-medium text-slate-900">{formatDateTime(event.occurred_at)}</p>
                  <p className="mt-1 text-xs text-slate-400">Synced {formatDateTime(event.created_at)}</p>
                </DenseTd>
                <DenseTd>
                  <Link className="font-mono text-xs text-blue-700 hover:underline" href={`/admin/users/${event.user_id}`}>
                    {truncateId(event.user_id)}
                  </Link>
                </DenseTd>
                <DenseTd>
                  <p className="font-mono text-xs text-slate-700">{truncateId(event.api_key_id)}</p>
                  {event.omniroute_key_id ? <p className="mt-1 font-mono text-[11px] text-slate-400">OR {truncateId(event.omniroute_key_id)}</p> : null}
                </DenseTd>
                <DenseTd><span className="font-medium text-slate-900">{event.model ?? "-"}</span></DenseTd>
                <DenseTd>{event.provider ?? "-"}</DenseTd>
                <DenseTd>
                  <p className="font-mono text-slate-900">{formatCompactUnits(event.total_tokens)}</p>
                  <p className="mt-1 text-xs text-slate-500">{formatCompactUnits(event.input_tokens)} in / {formatCompactUnits(event.output_tokens)} out</p>
                </DenseTd>
                <DenseTd><span className="font-mono text-slate-900">{formatCompactCredits(event.cost_units)}</span></DenseTd>
                <DenseTd><Badge dot tone={usageStatusTone(event.status)}>{event.status}</Badge></DenseTd>
                <DenseTd>
                  <p className="text-xs text-slate-600">{event.external_source}</p>
                  <p className="mt-1 max-w-48 truncate font-mono text-xs text-slate-400">{event.external_event_id}</p>
                </DenseTd>
                <DenseTd>
                  <button
                    className="inline-flex min-h-8 items-center rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:border-blue-200 hover:text-blue-700"
                    onClick={() => onSelect(event)}
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

function UsageDetailsDrawer({ event, onClose }: { event: UsageEvent; onClose: () => void }) {
  const rawJSON = event.raw_json ? JSON.stringify(redactSensitivePayload(event.raw_json), null, 2) : null;

  return (
    <div className="fixed inset-0 z-50 bg-slate-950/40 backdrop-blur-sm" onClick={onClose}>
      <div className="ml-auto flex h-full w-full max-w-[560px] flex-col bg-white shadow-2xl ring-1 ring-slate-200" onClick={(clickEvent) => clickEvent.stopPropagation()}>
        <div className="sticky top-0 z-10 border-b border-slate-200 bg-white/95 px-5 py-4 backdrop-blur">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">Usage event</p>
              <h2 className="mt-2 text-xl font-semibold text-slate-950">{truncateId(event.id, 12, 6)}</h2>
              <p className="mt-1 text-sm text-slate-500">OmniRoute usage metadata and SLAI billing result.</p>
            </div>
            <Button type="button" variant="ghost" onClick={onClose}>Close</Button>
          </div>
        </div>

        <div className="flex-1 space-y-5 overflow-y-auto px-5 py-5">
          <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-semibold uppercase tracking-[0.1em] text-slate-500">Billing status</p>
                <p className="mt-2 text-sm text-slate-600">{formatDateTime(event.occurred_at)}</p>
              </div>
              <Badge dot tone={usageStatusTone(event.status)}>{event.status}</Badge>
            </div>
          </section>

          <section className="grid gap-3 sm:grid-cols-2">
            <DetailField label="Event ID" value={event.id} mono />
            <DetailField label="External event ID" value={event.external_event_id} mono />
            <DetailField label="External source" value={event.external_source} />
            <DetailField label="Created" value={formatDateTime(event.created_at)} />
            <DetailField label="User ID" value={event.user_id} mono href={`/admin/users/${event.user_id}`} />
            <DetailField label="API key ID" value={event.api_key_id} mono />
            <DetailField label="OmniRoute key ID" value={event.omniroute_key_id ?? "-"} mono />
            <DetailField label="Model" value={event.model ?? "-"} />
            <DetailField label="Provider" value={event.provider ?? "-"} />
            <DetailField label="Input tokens" value={formatUnits(event.input_tokens)} mono />
            <DetailField label="Output tokens" value={formatUnits(event.output_tokens)} mono />
            <DetailField label="Total tokens" value={formatUnits(event.total_tokens)} mono />
            <DetailField label="Credit cost" value={formatCredits(event.cost_units)} mono />
          </section>

          <section className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold text-slate-950">Raw payload</h3>
                <p className="mt-1 text-xs text-slate-500">Sensitive key, token, password, authorization, and secret fields are redacted.</p>
              </div>
            </div>
            {rawJSON ? (
              <pre className="max-h-96 overflow-auto rounded-lg bg-slate-950 p-3 text-xs leading-5 text-slate-100">{rawJSON}</pre>
            ) : (
              <p className="rounded-lg border border-dashed border-slate-300 bg-slate-50 px-3 py-4 text-sm text-slate-500">No raw payload is available for this event.</p>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

function UsageSummaryCard({
  label,
  value,
  hint,
  scope,
  tone
}: {
  label: string;
  value: string;
  hint: string;
  scope: string;
  tone: "blue" | "green" | "red" | "slate";
}) {
  const marker = {
    blue: "bg-blue-500",
    green: "bg-emerald-500",
    red: "bg-red-500",
    slate: "bg-slate-300"
  }[tone];

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</p>
        <span className={cn("size-2 rounded-full", marker)} />
      </div>
      <p className="mt-3 text-2xl font-semibold tracking-[-0.02em] text-slate-950">{value}</p>
      <p className="mt-1 text-xs text-slate-500">{hint}</p>
      <p className="mt-3 text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-400">{scope}</p>
    </Card>
  );
}

function UsageEmptyState({ filtered, onClear }: { filtered: boolean; onClear: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-10 text-center shadow-sm">
      <h3 className="text-base font-semibold text-slate-950">{filtered ? "No usage matches these filters" : "No usage events"}</h3>
      <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-500">
        {filtered ? "Adjust or clear filters to inspect more usage events." : "Usage will appear after OmniRoute call logs are synced."}
      </p>
      <div className="mt-4 flex flex-wrap justify-center gap-2">
        {filtered ? <Button type="button" variant="secondary" onClick={onClear}>Clear filters</Button> : null}
        <Link className="inline-flex min-h-10 items-center justify-center rounded-md bg-slate-950 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800" href="/admin/sync">
          Open Sync Status
        </Link>
      </div>
    </div>
  );
}

function Pagination({
  count,
  limit,
  loading,
  offset,
  onNext,
  onPrevious
}: {
  count: number;
  limit: number;
  loading: boolean;
  offset: number;
  onNext: () => void;
  onPrevious: () => void;
}) {
  const start = count === 0 ? 0 : offset + 1;
  const end = offset + count;

  return (
    <div className="mt-4 flex flex-col gap-3 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm text-slate-500 shadow-sm sm:flex-row sm:items-center sm:justify-between">
      <span>Showing {formatUnits(start)}-{formatUnits(end)}{count === limit ? "+" : ""}</span>
      <div className="flex gap-2">
        <Button type="button" variant="secondary" disabled={offset === 0 || loading} onClick={onPrevious}>Previous</Button>
        <Button type="button" variant="secondary" disabled={count < limit || loading} onClick={onNext}>Next</Button>
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

function DetailField({ href, label, mono = false, value }: { href?: string; label: string; mono?: boolean; value: string | number }) {
  const content = String(value);
  const className = cn("mt-1 break-all text-sm text-slate-900", mono && "font-mono text-xs");

  return (
    <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
      <p className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">{label}</p>
      {href ? <Link className={cn(className, "block text-blue-700 hover:underline")} href={href}>{content}</Link> : <p className={className}>{content}</p>}
    </div>
  );
}

function DenseTh({ children }: { children: React.ReactNode }) {
  return <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">{children}</th>;
}

function DenseTd({ children }: { children: React.ReactNode }) {
  return <td className="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{children}</td>;
}

function summarizeUsage(events: UsageEvent[]): Summary {
  return events.reduce(
    (summary, event) => ({
      billed: summary.billed + (event.status === "billed" ? 1 : 0),
      pending: summary.pending + (event.status === "pending" ? 1 : 0),
      failed: summary.failed + (event.status === "failed" ? 1 : 0),
      ignored: summary.ignored + (event.status === "ignored" ? 1 : 0),
      inputTokens: summary.inputTokens + event.input_tokens,
      outputTokens: summary.outputTokens + event.output_tokens,
      totalTokens: summary.totalTokens + event.total_tokens,
      costUnits: summary.costUnits + event.cost_units
    }),
    { billed: 0, pending: 0, failed: 0, ignored: 0, inputTokens: 0, outputTokens: 0, totalTokens: 0, costUnits: 0 }
  );
}

function validateDateRange(form: UsageFormFilters) {
  if (!form.from || !form.to) {
    return null;
  }
  const fromTime = new Date(form.from).getTime();
  const toTime = new Date(form.to).getTime();
  if (!Number.isFinite(fromTime) || !Number.isFinite(toTime)) {
    return "Use valid from and to date/time values.";
  }
  if (fromTime > toTime) {
    return "From date/time must be before or equal to To date/time.";
  }
  return null;
}

function formFiltersToApiFilter(form: UsageFormFilters, offset: number): UsageFilter {
  const filter: UsageFilter = {
    user_id: trimmed(form.user_id),
    api_key_id: trimmed(form.api_key_id),
    model: trimmed(form.model),
    provider: trimmed(form.provider),
    status: trimmed(form.status),
    limit: LIMIT,
    offset
  };
  if (form.from) {
    filter.from = new Date(form.from).toISOString();
  }
  if (form.to) {
    filter.to = new Date(form.to).toISOString();
  }
  return normalizeUsageFilter(filter);
}

function normalizeUsageFilter(filter: UsageFilter): UsageFilter {
  return {
    user_id: trimmed(filter.user_id),
    api_key_id: trimmed(filter.api_key_id),
    model: trimmed(filter.model),
    provider: trimmed(filter.provider),
    status: trimmed(filter.status),
    from: trimmed(filter.from),
    to: trimmed(filter.to),
    limit: filter.limit ?? LIMIT,
    offset: filter.offset ?? 0
  };
}

function activeFilterLabels(filter: UsageFilter) {
  const labels: string[] = [];
  if (filter.user_id) labels.push(`user:${truncateId(filter.user_id)}`);
  if (filter.api_key_id) labels.push(`key:${truncateId(filter.api_key_id)}`);
  if (filter.model) labels.push(`model:${filter.model}`);
  if (filter.provider) labels.push(`provider:${filter.provider}`);
  if (filter.status) labels.push(`status:${filter.status}`);
  if (filter.from) labels.push(`from:${formatDateTime(filter.from)}`);
  if (filter.to) labels.push(`to:${formatDateTime(filter.to)}`);
  return labels;
}

function usageStatusTone(status?: string) {
  switch (status) {
    case "billed":
      return "green" as const;
    case "pending":
      return "blue" as const;
    case "duplicate":
      return "neutral" as const;
    case "ignored":
      return "yellow" as const;
    case "failed":
      return "red" as const;
    default:
      return "neutral" as const;
  }
}

function syncSummaryText(result: { fetched: number; billed: number; duplicate: number; ignored: number; failed: number; suspended_keys: number }) {
  return `Sync complete: ${formatUnits(result.fetched)} fetched, ${formatUnits(result.billed)} billed, ${formatUnits(result.duplicate)} duplicate, ${formatUnits(result.ignored)} ignored, ${formatUnits(result.failed)} failed, ${formatUnits(result.suspended_keys)} suspended keys.`;
}

function trimmed(value?: string | null) {
  const next = value?.trim();
  return next ? next : undefined;
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
  return /(key|token|secret|authorization|password)/i.test(key);
}
