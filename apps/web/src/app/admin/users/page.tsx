"use client";

import Link from "next/link";
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
import { api, readableError, type AdminUserFilter, type AdminUserListResponse } from "@/lib/api";
import { formatDateTime, formatUnits } from "@/lib/format";

const LIMIT = 50;

export default function AdminUsersPage() {
  const [data, setData] = useState<AdminUserListResponse | null>(null);
  const [filters, setFilters] = useState<AdminUserFilter>({ limit: LIMIT, offset: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load(nextFilters = filters) {
    setLoading(true);
    setError(null);
    api.admin.users
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
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin users</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Users</h1>
      </div>

      <Card className="mt-8">
        <CardHeader>
          <div>
            <CardTitle>Search users</CardTitle>
            <CardDescription>Filter by email, status, or role.</CardDescription>
          </div>
        </CardHeader>
        <form className="grid gap-4 md:grid-cols-4" onSubmit={submit}>
          <Input label="Email search" value={filters.q ?? ""} onChange={(event) => setFilters({ ...filters, q: event.target.value })} />
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Status</span>
            <select className="mt-2 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm" value={filters.status ?? ""} onChange={(event) => setFilters({ ...filters, status: event.target.value as AdminUserFilter["status"] })}>
              <option value="">Any</option>
              <option value="ACTIVE">ACTIVE</option>
              <option value="SUSPENDED">SUSPENDED</option>
            </select>
          </label>
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Role</span>
            <select className="mt-2 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm" value={filters.role ?? ""} onChange={(event) => setFilters({ ...filters, role: event.target.value as AdminUserFilter["role"] })}>
              <option value="">Any</option>
              <option value="USER">USER</option>
              <option value="ADMIN">ADMIN</option>
            </select>
          </label>
          <div className="self-end"><Button type="submit" disabled={loading}>Apply</Button></div>
        </form>
      </Card>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading users" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(filters)} /> : null}
        {!loading && !error && items.length === 0 ? <EmptyState title="No users" message="No users matched the selected filters." /> : null}
        {!loading && !error && items.length > 0 ? (
          <>
            <Table>
              <thead className="bg-slate-50"><tr><Th>Email</Th><Th>Role</Th><Th>Status</Th><Th>Balance</Th><Th>Lifetime used</Th><Th>API key</Th><Th>Created</Th><Th>Action</Th></tr></thead>
              <tbody className="divide-y divide-slate-100">
                {items.map((user) => (
                  <tr key={user.id}>
                    <Td className="font-medium text-slate-950">{user.email}</Td>
                    <Td>{user.role}</Td>
                    <Td><Badge tone={statusTone(user.status)}>{user.status}</Badge></Td>
                    <Td>{formatUnits(user.balance_units)}</Td>
                    <Td>{formatUnits(user.lifetime_used_units)}</Td>
                    <Td>{user.api_key_status ? <Badge tone={statusTone(user.api_key_status)}>{user.api_key_status}</Badge> : "-"}</Td>
                    <Td>{formatDateTime(user.created_at)}</Td>
                    <Td><Link className="font-semibold text-cyan-700" href={`/admin/users/${user.id}`}>Open</Link></Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <div className="mt-4 flex items-center justify-between">
              <Button type="button" variant="secondary" disabled={(data?.offset ?? 0) === 0 || loading} onClick={() => load({ ...filters, offset: Math.max(0, (data?.offset ?? 0) - LIMIT) })}>
                Previous
              </Button>
              <span className="text-sm text-slate-500">Showing {items.length} of {formatUnits(data?.total ?? 0)} users</span>
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
