import { AppShell } from "@/components/AppShell";
import { MetricCard } from "@/components/MetricCard";

export default function DashboardPage() {
  return (
    <AppShell section="dashboard">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">User dashboard</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Credits and API key</h1>
        </div>
        <button className="rounded-md bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800">Top up</button>
      </div>

      <section className="mt-8 grid gap-4 md:grid-cols-3">
        <MetricCard label="Available credits" value="0" hint="Ledger-backed balance" />
        <MetricCard label="Lifetime used" value="0" hint="Synced from OmniRoute logs" />
        <MetricCard label="Active keys" value="1" hint="MVP limit" />
      </section>

      <section className="mt-8 rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-base font-semibold text-slate-950">Default API key</h2>
            <p className="mt-1 text-sm text-slate-500">Raw keys are shown only once and stored as a hash plus prefix.</p>
          </div>
          <span className="w-fit rounded-md bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700">ACTIVE</span>
        </div>
        <div className="mt-5 rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 font-mono text-sm text-slate-700">slai_live_xxxxxxxxxxxx</div>
      </section>
    </AppShell>
  );
}
