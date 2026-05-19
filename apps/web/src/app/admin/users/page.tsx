"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type AdminUserFilter, type AdminUserListItem, type AdminUserListResponse, type UserStatus } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactCredits, formatDateTime, formatUnits, truncateId } from "@/lib/format";

const LIMIT = 50;

function authProviderLabel(provider: AdminUserListItem["auth_provider"]) {
  return provider === "google" ? "Google" : "Password";
}

export default function AdminUsersPage() {
  const [data, setData] = useState<AdminUserListResponse | null>(null);
  const [filters, setFilters] = useState<AdminUserFilter>({ limit: LIMIT, offset: 0 });
  const [draftFilters, setDraftFilters] = useState<AdminUserFilter>({ limit: LIMIT, offset: 0 });
  const [loading, setLoading] = useState(true);
  const [actionUserId, setActionUserId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  function load(nextFilters = filters) {
    setLoading(true);
    setError(null);
    api.admin.users
      .list(nextFilters)
      .then((response) => {
        setData(response);
        setFilters(nextFilters);
        setDraftFilters(nextFilters);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => load({ limit: LIMIT, offset: 0 }), []);

  function submit(event: FormEvent) {
    event.preventDefault();
    setNotice(null);
    load({ ...draftFilters, limit: LIMIT, offset: 0 });
  }

  function resetFilters() {
    const reset = { limit: LIMIT, offset: 0 };
    setDraftFilters(reset);
    load(reset);
  }

  async function updateStatus(user: AdminUserListItem, status: UserStatus) {
    const message = status === "SUSPENDED"
      ? `Suspend ${user.email}? Their active API key will also be suspended.`
      : `Activate ${user.email}? Their API key will not be resumed automatically.`;
    if (!window.confirm(message)) {
      return;
    }

    setActionUserId(user.id);
    setError(null);
    setNotice(null);
    try {
      await api.admin.users.updateStatus(user.id, status);
      setNotice(`${user.email} is now ${status}.`);
      await api.admin.users.list(filters).then(setData);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionUserId(null);
    }
  }

  const items = data?.items ?? [];
  const offset = data?.offset ?? 0;
  const total = data?.total ?? 0;
  const rangeStart = total === 0 ? 0 : offset + 1;
  const rangeEnd = Math.min(offset + items.length, total);
  const shownCounts = useMemo(() => ({
    active: items.filter((user) => user.status === "ACTIVE").length,
    suspended: items.filter((user) => user.status === "SUSPENDED").length,
    admin: items.filter((user) => user.role === "ADMIN").length
  }), [items]);

  return (
    <AdminShell>
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-[-0.02em] text-slate-950">Users</h1>
          <p className="mt-1 text-sm text-slate-500">Manage developer accounts, balances, API keys, and access.</p>
        </div>
        <Button type="button" variant="secondary" onClick={() => load(filters)} disabled={loading}>
          Refresh
        </Button>
      </div>

      <section className="mt-6 grid gap-3 md:grid-cols-4">
        <SummaryCard label="Total users" value={formatUnits(total)} hint="Matching filters" />
        <SummaryCard label="Active shown" value={formatUnits(shownCounts.active)} hint="Current page" tone="green" />
        <SummaryCard label="Suspended shown" value={formatUnits(shownCounts.suspended)} hint="Current page" tone="yellow" />
        <SummaryCard label="Admins shown" value={formatUnits(shownCounts.admin)} hint="Current page" tone="blue" />
      </section>

      <Card className="mt-6 p-4">
        <CardHeader className="mb-4">
          <div>
            <CardTitle>Directory</CardTitle>
            <CardDescription>Search and filter admin-safe user records.</CardDescription>
          </div>
        </CardHeader>
        <form className="grid gap-3 lg:grid-cols-[minmax(260px,1fr)_180px_160px_auto_auto]" onSubmit={submit}>
          <label className="block">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">Search</span>
            <input
              className="mt-2 h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-950 outline-none ring-blue-600/20 placeholder:text-slate-400 focus:border-blue-600 focus:ring-4"
              onChange={(event) => setDraftFilters({ ...draftFilters, q: event.target.value })}
              placeholder="Search by email..."
              value={draftFilters.q ?? ""}
            />
          </label>
          <SelectFilter
            label="Status"
            onChange={(value) => setDraftFilters({ ...draftFilters, status: value as AdminUserFilter["status"] })}
            options={[{ label: "All", value: "" }, { label: "Active", value: "ACTIVE" }, { label: "Suspended", value: "SUSPENDED" }]}
            value={draftFilters.status ?? ""}
          />
          <SelectFilter
            label="Role"
            onChange={(value) => setDraftFilters({ ...draftFilters, role: value as AdminUserFilter["role"] })}
            options={[{ label: "All", value: "" }, { label: "User", value: "USER" }, { label: "Admin", value: "ADMIN" }]}
            value={draftFilters.role ?? ""}
          />
          <div className="flex items-end"><Button className="w-full" type="submit" disabled={loading}>Apply</Button></div>
          <div className="flex items-end"><Button className="w-full" type="button" variant="secondary" onClick={resetFilters} disabled={loading}>Clear</Button></div>
        </form>
      </Card>

      <div className="mt-6 space-y-4">
        {loading ? <UsersTableSkeleton /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(filters)} /> : null}
        {notice ? <p className="rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-sm text-blue-800">{notice}</p> : null}
        {!loading && !error && items.length === 0 ? <EmptyState title="No users found" message="No users matched the selected filters." /> : null}
        {!loading && !error && items.length > 0 ? (
          <>
            <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-slate-200 text-sm">
                  <thead className="sticky top-0 bg-slate-50/95 backdrop-blur">
                    <tr>
                      <DenseTh>User</DenseTh>
                      <DenseTh>Role</DenseTh>
                      <DenseTh>Status</DenseTh>
                      <DenseTh>Sign-in</DenseTh>
                      <DenseTh>Balance</DenseTh>
                      <DenseTh>Lifetime Used</DenseTh>
                      <DenseTh>API Key</DenseTh>
                      <DenseTh>Created</DenseTh>
                      <DenseTh>Actions</DenseTh>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {items.map((user) => (
                      <tr className="transition-colors hover:bg-slate-50" key={user.id}>
                        <DenseTd>
                          <div className="min-w-0">
                            <Link className="font-semibold text-slate-950 hover:text-blue-700" href={`/admin/users/${user.id}`}>{user.email}</Link>
                            <p className="mt-1 font-mono text-xs text-slate-400">{truncateId(user.id, 8, 6)}</p>
                          </div>
                        </DenseTd>
                        <DenseTd><Badge dot tone={statusTone(user.role)}>{user.role}</Badge></DenseTd>
                        <DenseTd><Badge dot tone={statusTone(user.status)}>{user.status}</Badge></DenseTd>
                        <DenseTd><Badge tone="neutral">{authProviderLabel(user.auth_provider)}</Badge></DenseTd>
                        <DenseTd><span className="font-mono text-slate-900">{formatCompactCredits(user.balance_units)}</span></DenseTd>
                        <DenseTd><span className="font-mono text-slate-700">{formatCompactCredits(user.lifetime_used_units)}</span></DenseTd>
                        <DenseTd>
                          {user.api_key_status ? (
                            <div className="space-y-1">
                              <Badge dot tone={statusTone(user.api_key_status)}>{user.api_key_status}</Badge>
                              {user.api_key_prefix ? <p className="font-mono text-xs text-slate-400">{user.api_key_prefix}</p> : null}
                            </div>
                          ) : <span className="text-slate-400">-</span>}
                        </DenseTd>
                        <DenseTd>{formatDateTime(user.created_at)}</DenseTd>
                        <DenseTd>
                          <div className="flex flex-wrap gap-2">
                            <Link className="inline-flex min-h-8 items-center rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:border-blue-200 hover:text-blue-700" href={`/admin/users/${user.id}`}>
                              View
                            </Link>
                            {user.status === "ACTIVE" ? (
                              <button className="inline-flex min-h-8 items-center rounded-md border border-amber-200 bg-amber-50 px-3 text-xs font-semibold text-amber-800 hover:bg-amber-100 disabled:opacity-50" disabled={actionUserId === user.id} onClick={() => updateStatus(user, "SUSPENDED")} type="button">
                                Suspend
                              </button>
                            ) : (
                              <button className="inline-flex min-h-8 items-center rounded-md border border-emerald-200 bg-emerald-50 px-3 text-xs font-semibold text-emerald-800 hover:bg-emerald-100 disabled:opacity-50" disabled={actionUserId === user.id} onClick={() => updateStatus(user, "ACTIVE")} type="button">
                                Activate
                              </button>
                            )}
                          </div>
                        </DenseTd>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
            <div className="flex flex-col gap-3 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm sm:flex-row sm:items-center sm:justify-between">
              <span className="text-slate-500">Showing <span className="font-medium text-slate-950">{formatUnits(rangeStart)}-{formatUnits(rangeEnd)}</span> of <span className="font-medium text-slate-950">{formatUnits(total)}</span></span>
              <div className="flex gap-2">
                <Button type="button" variant="secondary" disabled={offset === 0 || loading} onClick={() => load({ ...filters, offset: Math.max(0, offset - LIMIT) })}>Previous</Button>
                <Button type="button" variant="secondary" disabled={rangeEnd >= total || loading} onClick={() => load({ ...filters, offset: offset + LIMIT })}>Next</Button>
              </div>
            </div>
          </>
        ) : null}
      </div>
    </AdminShell>
  );
}

function SummaryCard({ label, value, hint, tone = "neutral" }: { label: string; value: string; hint: string; tone?: "neutral" | "green" | "yellow" | "blue" }) {
  const marker = {
    neutral: "bg-slate-300",
    green: "bg-emerald-500",
    yellow: "bg-amber-500",
    blue: "bg-blue-500"
  }[tone];

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between">
        <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</p>
        <span className={cn("size-2 rounded-full", marker)} />
      </div>
      <p className="mt-3 text-2xl font-semibold tracking-[-0.02em] text-slate-950">{value}</p>
      <p className="mt-1 text-xs text-slate-500">{hint}</p>
    </Card>
  );
}

function SelectFilter({ label, value, options, onChange }: { label: string; value: string; options: { label: string; value: string }[]; onChange: (value: string) => void }) {
  return (
    <label className="block">
      <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">{label}</span>
      <select className="mt-2 h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-950 outline-none ring-blue-600/20 focus:border-blue-600 focus:ring-4" value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
      </select>
    </label>
  );
}

function DenseTh({ children }: { children: React.ReactNode }) {
  return <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">{children}</th>;
}

function DenseTd({ children }: { children: React.ReactNode }) {
  return <td className="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{children}</td>;
}

function UsersTableSkeleton() {
  return (
    <div className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
      <LoadingState label="Loading users" />
      <div className="space-y-2 p-4">
        {Array.from({ length: 6 }).map((_, index) => <div className="h-10 animate-pulse rounded-md bg-slate-100" key={index} />)}
      </div>
    </div>
  );
}
