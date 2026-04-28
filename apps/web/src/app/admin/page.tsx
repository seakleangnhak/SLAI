import { AppShell } from "@/components/AppShell";
import { MetricCard } from "@/components/MetricCard";

const rows = [
  { user: "developer@example.com", action: "Manual top-up", amount: "$50.00", status: "pending" },
  { user: "team@example.com", action: "Usage sync", amount: "12,800 credits", status: "billed" }
];

export default function AdminPage() {
  return (
    <AppShell section="admin">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin console</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Operations</h1>
        </div>
        <button className="rounded-md bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800">Create top-up</button>
      </div>

      <section className="mt-8 grid gap-4 md:grid-cols-3">
        <MetricCard label="Users" value="0" hint="Active accounts" />
        <MetricCard label="Manual top-ups" value="0" hint="MVP payment flow" />
        <MetricCard label="Usage events" value="0" hint="Idempotent ingestion" />
      </section>

      <section className="mt-8 overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-200 px-5 py-4">
          <h2 className="text-base font-semibold text-slate-950">Recent activity</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">
              <tr>
                <th className="px-5 py-3">User</th>
                <th className="px-5 py-3">Action</th>
                <th className="px-5 py-3">Amount</th>
                <th className="px-5 py-3">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 bg-white text-slate-700">
              {rows.map((row) => (
                <tr key={`${row.user}-${row.action}`}>
                  <td className="whitespace-nowrap px-5 py-4 font-medium text-slate-950">{row.user}</td>
                  <td className="whitespace-nowrap px-5 py-4">{row.action}</td>
                  <td className="whitespace-nowrap px-5 py-4">{row.amount}</td>
                  <td className="whitespace-nowrap px-5 py-4">{row.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </AppShell>
  );
}
