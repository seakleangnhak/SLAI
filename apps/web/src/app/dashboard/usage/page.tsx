"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardTitle } from "@/components/Card";
import { DashboardShell } from "@/components/DashboardShell";
import { ErrorState } from "@/components/ErrorState";
import { Input } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type UsageEvent, type UserUsageFilter } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactUnits, formatCredits, formatDateTime, formatUnits, truncateId } from "@/lib/format";

const LIMIT = 50;
const primaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg bg-slate-950 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-300";
const secondaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-400";
const statuses = ["pending", "billed", "duplicate", "failed", "ignored"];
type FilterState = {
  model: string;
  status: string;
  from: string;
  to: string;
};

const emptyFilters: FilterState = {
  model: "",
  status: "",
  from: "",
  to: ""
};

function statusTone(status?: string) {
  if (status === "billed") {
    return "green" as const;
  }
  if (status === "failed") {
    return "red" as const;
  }
  if (status === "ignored") {
    return "yellow" as const;
  }
  if (status === "pending") {
    return "blue" as const;
  }
  return "neutral" as const;
}

function toISODateTime(value: string) {
  if (!value) {
    return undefined;
  }
  return new Date(value).toISOString();
}

function hasActiveFilters(filters: FilterState) {
  return Object.values(filters).some((value) => value.trim() !== "");
}

function filtersToQuery(filters: FilterState, offset: number): UserUsageFilter {
  return {
    model: filters.model.trim() || undefined,
    status: filters.status || undefined,
    from: toISODateTime(filters.from),
    to: toISODateTime(filters.to),
    limit: LIMIT,
    offset
  };
}

function validateFilters(filters: FilterState) {
  if (filters.from && filters.to && new Date(filters.from).getTime() > new Date(filters.to).getTime()) {
    return "From date/time must be before To date/time.";
  }
  return null;
}

function SummaryCard({ label, value, hint, children }: { label: string; value: string; hint: string; children?: React.ReactNode }) {
  return (
    <Card className="min-h-36 rounded-2xl p-5 shadow-sm transition hover:shadow-md">
      <div className="flex h-full flex-col">
        <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">{label}</p>
        <p className="mt-3 text-3xl font-semibold tracking-normal text-slate-950">{value}</p>
        <p className="mt-2 text-sm leading-6 text-slate-500">{hint}</p>
        {children ? <div className="mt-auto pt-3">{children}</div> : null}
      </div>
    </Card>
  );
}

function InfoBanner() {
  return (
    <Card className="slai-usage-async-card rounded-2xl border-blue-200 bg-gradient-to-r from-blue-50 to-white p-4 shadow-sm">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex gap-3">
          <span className="slai-usage-async-icon mt-0.5 grid size-9 shrink-0 place-items-center rounded-full bg-blue-100 text-sm font-bold text-blue-700">i</span>
          <div>
            <CardTitle>Async billing</CardTitle>
            <CardDescription className="slai-usage-async-copy mt-1 text-blue-900/70">
              Usage appears after provider logs are synced. Duplicate events are ignored and billed events create ledger debits.
            </CardDescription>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link className={secondaryButton} href="/dashboard/billing">View billing</Link>
          <Link className={secondaryButton} href="/dashboard/api-key">View API key</Link>
        </div>
      </div>
    </Card>
  );
}

function FilterBar({
  filters,
  setFilters,
  onApply,
  onClear,
  validationError,
  loading
}: {
  filters: FilterState;
  setFilters: (filters: FilterState) => void;
  onApply: () => void;
  onClear: () => void;
  validationError: string | null;
  loading: boolean;
}) {
  function submit(event: FormEvent) {
    event.preventDefault();
    onApply();
  }

  return (
    <Card className="rounded-2xl p-4">
      <form className="space-y-4" onSubmit={submit}>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-[1fr_0.8fr_1fr_1fr_auto] xl:items-end">
          <Input label="Model" placeholder="gpt-5.5" value={filters.model} onChange={(event) => setFilters({ ...filters, model: event.target.value })} />
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Status</span>
            <select
              className="mt-2 h-10 w-full rounded-md border border-slate-300 bg-white px-3 text-sm text-slate-950 outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4"
              value={filters.status}
              onChange={(event) => setFilters({ ...filters, status: event.target.value })}
            >
              <option value="">All</option>
              {statuses.map((status) => <option key={status} value={status}>{status}</option>)}
            </select>
          </label>
          <Input label="From" type="datetime-local" value={filters.from} onChange={(event) => setFilters({ ...filters, from: event.target.value })} />
          <Input label="To" type="datetime-local" value={filters.to} onChange={(event) => setFilters({ ...filters, to: event.target.value })} />
          <div className="flex gap-2">
            <Button className="rounded-lg" type="submit" disabled={loading}>Apply</Button>
            <Button className="rounded-lg" type="button" variant="secondary" onClick={onClear} disabled={loading}>Clear</Button>
          </div>
        </div>
        {validationError ? <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{validationError}</p> : null}
      </form>
    </Card>
  );
}

function EmptyUsageState({ filtered, onClear }: { filtered: boolean; onClear: () => void }) {
  return (
    <div className="rounded-2xl border border-dashed border-slate-200 bg-slate-50/80 px-5 py-8 text-center">
      <span className="mx-auto grid size-10 place-items-center rounded-full bg-white font-semibold text-blue-700 shadow-sm ring-1 ring-slate-200">U</span>
      <h3 className="mt-4 text-base font-semibold text-slate-950">{filtered ? "No usage matches these filters" : "No usage yet"}</h3>
      <p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-slate-500">
        {filtered
          ? "Try clearing filters or expanding the date range."
          : "Send requests with your SLAI-created API key. Usage will appear after sync."}
      </p>
      <div className="mt-5 flex flex-wrap justify-center gap-3">
        {filtered ? (
          <Button className="rounded-lg" type="button" onClick={onClear}>Clear filters</Button>
        ) : (
          <>
            <Link className={primaryButton} href="/dashboard/api-key">View API key</Link>
            <Link className={secondaryButton} href="/dashboard/billing">View billing</Link>
          </>
        )}
      </div>
    </div>
  );
}

function UsageTable({ usage, onSelect }: { usage: UsageEvent[]; onSelect: (event: UsageEvent) => void }) {
  return (
    <Card className="overflow-hidden rounded-2xl p-0">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-sm">
          <thead className="bg-slate-50">
            <tr>
              {["Time", "Model", "Tokens", "Cost", "Status", "Event ID", "Actions"].map((label) => (
                <th key={label} className="whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {usage.map((event) => (
              <tr key={event.id} className="hover:bg-slate-50">
                <td className="whitespace-nowrap px-4 py-3 text-slate-600">{formatDateTime(event.occurred_at)}</td>
                <td className="whitespace-nowrap px-4 py-3 font-medium text-slate-950">{event.combo_name ?? event.model ?? "-"}</td>
                <td className="whitespace-nowrap px-4 py-3 text-slate-700">
                  <div className="font-semibold text-slate-950">{formatUnits(event.total_tokens)}</div>
                  <div className="text-xs text-slate-500">{formatCompactUnits(event.input_tokens)} in / {formatCompactUnits(event.output_tokens)} out</div>
                </td>
                <td className="whitespace-nowrap px-4 py-3 font-semibold text-slate-950">{formatCredits(event.cost_units)}</td>
                <td className="whitespace-nowrap px-4 py-3"><Badge dot tone={statusTone(event.status)}>{event.status}</Badge></td>
                <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-slate-600">{truncateId(event.external_event_id, 10, 4)}</td>
                <td className="whitespace-nowrap px-4 py-3">
                  <button className="text-sm font-semibold text-blue-700 hover:text-blue-800" type="button" onClick={() => onSelect(event)}>View details</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

function DetailItem({ label, value, mono = false }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</dt>
      <dd className={cn("mt-1 break-words text-sm font-medium text-slate-950", mono && "font-mono")}>{value}</dd>
    </div>
  );
}

function UsageDetailsDrawer({ event, onClose }: { event: UsageEvent | null; onClose: () => void }) {
  if (!event) {
    return null;
  }
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-slate-950/30 backdrop-blur-sm" onMouseDown={onClose}>
      <aside className="h-full w-full max-w-xl overflow-y-auto border-l border-slate-200 bg-white shadow-2xl" onMouseDown={(event) => event.stopPropagation()}>
        <div className="sticky top-0 z-10 border-b border-slate-200 bg-white/95 px-5 py-4 backdrop-blur">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Usage event</p>
              <h2 className="mt-1 text-xl font-semibold text-slate-950">Event details</h2>
            </div>
            <button className={secondaryButton} type="button" onClick={onClose}>Close</button>
          </div>
        </div>
        <div className="space-y-5 p-5">
          <div className="grid gap-3 sm:grid-cols-2">
            <DetailItem label="Event ID" value={event.id} mono />
            <DetailItem label="External ID" value={event.external_event_id} mono />
            <DetailItem label="Status" value={<Badge dot tone={statusTone(event.status)}>{event.status}</Badge>} />
            <DetailItem label="Model" value={event.combo_name ?? event.model ?? "-"} />
            <DetailItem label="Input tokens" value={formatUnits(event.input_tokens)} />
            <DetailItem label="Output tokens" value={formatUnits(event.output_tokens)} />
            <DetailItem label="Total tokens" value={formatUnits(event.total_tokens)} />
            <DetailItem label="Credit cost" value={formatCredits(event.cost_units)} />
            <DetailItem label="Occurred" value={formatDateTime(event.occurred_at)} />
            <DetailItem label="Created" value={formatDateTime(event.created_at)} />
          </div>
        </div>
      </aside>
    </div>
  );
}

export default function UsagePage() {
  const [usage, setUsage] = useState<UsageEvent[]>([]);
  const [offset, setOffset] = useState(0);
  const [draftFilters, setDraftFilters] = useState<FilterState>(emptyFilters);
  const [appliedFilters, setAppliedFilters] = useState<FilterState>(emptyFilters);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [selectedEvent, setSelectedEvent] = useState<UsageEvent | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load(nextOffset = offset, filters = appliedFilters) {
    const validation = validateFilters(filters);
    if (validation) {
      setValidationError(validation);
      return;
    }
    setValidationError(null);
    setLoading(true);
    setError(null);
    api.usage
      .list(filtersToQuery(filters, nextOffset))
      .then((response) => {
        setUsage(response.usage);
        setOffset(nextOffset);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => load(0, emptyFilters), []);

  function applyFilters() {
    const validation = validateFilters(draftFilters);
    if (validation) {
      setValidationError(validation);
      return;
    }
    setAppliedFilters(draftFilters);
    load(0, draftFilters);
  }

  function clearFilters() {
    setDraftFilters(emptyFilters);
    setAppliedFilters(emptyFilters);
    setValidationError(null);
    load(0, emptyFilters);
  }

  const summary = useMemo(() => {
    const input = usage.reduce((sum, event) => sum + event.input_tokens, 0);
    const output = usage.reduce((sum, event) => sum + event.output_tokens, 0);
    const cost = usage.reduce((sum, event) => sum + event.cost_units, 0);
    const exceptions = usage.filter((event) => event.status === "failed" || event.status === "ignored").length;
    return { input, output, totalTokens: input + output, cost, exceptions };
  }, [usage]);
  const filtered = hasActiveFilters(appliedFilters);

  return (
    <DashboardShell>
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-blue-700">Usage</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">API usage</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-500">Usage is billed asynchronously from synced provider logs.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button className={secondaryButton} type="button" onClick={() => load(offset)} disabled={loading}>Refresh</button>
            <Link className={secondaryButton} href="/dashboard/api-key">View API key</Link>
            <Link className={secondaryButton} href="/dashboard/billing">View billing</Link>
          </div>
        </div>
      </section>

      <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard label="Usage events" value={formatUnits(usage.length)} hint="Current result page" />
        <SummaryCard label="Tokens" value={formatUnits(summary.totalTokens)} hint={formatCompactUnits(summary.input) + " input / " + formatCompactUnits(summary.output) + " output"} />
        <SummaryCard label="Credit cost" value={formatCredits(summary.cost)} hint="Credits on current page" />
        <SummaryCard label="Exceptions" value={formatUnits(summary.exceptions)} hint="Failed plus ignored events">
          <Badge dot tone={summary.exceptions > 0 ? "yellow" : "green"}>{summary.exceptions > 0 ? "Needs review" : "OK"}</Badge>
        </SummaryCard>
      </section>

      <div className="mt-6"><InfoBanner /></div>

      <div className="mt-6">
        <FilterBar
          filters={draftFilters}
          loading={loading}
          onApply={applyFilters}
          onClear={clearFilters}
          setFilters={setDraftFilters}
          validationError={validationError}
        />
      </div>

      <div className="mt-6">
        {loading ? <LoadingState label="Loading usage" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(offset)} /> : null}
        {!loading && !error && usage.length === 0 ? <EmptyUsageState filtered={filtered} onClear={clearFilters} /> : null}
        {!loading && !error && usage.length > 0 ? (
          <>
            <UsageTable usage={usage} onSelect={setSelectedEvent} />
            <div className="mt-4 flex flex-col gap-3 rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-500 shadow-sm sm:flex-row sm:items-center sm:justify-between">
              <span>Showing {offset + 1}-{offset + usage.length}. Total count is not exposed by the API yet.</span>
              <div className="flex gap-2">
                <Button type="button" variant="secondary" className="rounded-lg" disabled={offset === 0 || loading} onClick={() => load(Math.max(0, offset - LIMIT))}>
                  Previous
                </Button>
                <Button type="button" variant="secondary" className="rounded-lg" disabled={usage.length < LIMIT || loading} onClick={() => load(offset + LIMIT)}>
                  Next
                </Button>
              </div>
            </div>
          </>
        ) : null}
      </div>

      <UsageDetailsDrawer event={selectedEvent} onClose={() => setSelectedEvent(null)} />
    </DashboardShell>
  );
}
