"use client";

import { useEffect, useState } from "react";

import { Badge, statusTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { DashboardShell } from "@/components/DashboardShell";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { Table, Td, Th } from "@/components/Table";
import { api, readableError, type UsageEvent } from "@/lib/api";
import { formatDateTime, formatUnits } from "@/lib/format";

const LIMIT = 25;

export default function UsagePage() {
  const [usage, setUsage] = useState<UsageEvent[]>([]);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  function load(nextOffset = offset) {
    setLoading(true);
    setError(null);
    api.usage
      .list(LIMIT, nextOffset)
      .then((response) => {
        setUsage(response.usage);
        setOffset(nextOffset);
      })
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => load(0), []);

  return (
    <DashboardShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Usage</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">OmniRoute usage</h1>
        <p className="mt-2 text-sm text-slate-500">Usage is billed asynchronously from synced OmniRoute call logs.</p>
      </div>

      <div className="mt-8">
        {loading ? <LoadingState label="Loading usage" /> : null}
        {error ? <ErrorState message={error} onRetry={() => load(offset)} /> : null}
        {!loading && !error && usage.length === 0 ? <EmptyState title="No usage" message="Call OmniRoute with your SLAI-created key, then wait for sync." /> : null}
        {!loading && !error && usage.length > 0 ? (
          <>
            <Table>
              <thead className="bg-slate-50"><tr><Th>Model</Th><Th>Provider</Th><Th>Input</Th><Th>Output</Th><Th>Total</Th><Th>Cost</Th><Th>Status</Th><Th>Occurred</Th></tr></thead>
              <tbody className="divide-y divide-slate-100">
                {usage.map((event) => (
                  <tr key={event.id}>
                    <Td className="font-medium text-slate-950">{event.model ?? "-"}</Td>
                    <Td>{event.provider ?? "-"}</Td>
                    <Td>{formatUnits(event.input_tokens)}</Td>
                    <Td>{formatUnits(event.output_tokens)}</Td>
                    <Td>{formatUnits(event.total_tokens)}</Td>
                    <Td>{formatUnits(event.cost_units)}</Td>
                    <Td><Badge tone={statusTone(event.status)}>{event.status}</Badge></Td>
                    <Td>{formatDateTime(event.occurred_at)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <div className="mt-4 flex items-center justify-between">
              <Button type="button" variant="secondary" disabled={offset === 0 || loading} onClick={() => load(Math.max(0, offset - LIMIT))}>
                Previous
              </Button>
              <span className="text-sm text-slate-500">Offset {offset}</span>
              <Button type="button" variant="secondary" disabled={usage.length < LIMIT || loading} onClick={() => load(offset + LIMIT)}>
                Next
              </Button>
            </div>
          </>
        ) : null}
      </div>
    </DashboardShell>
  );
}
