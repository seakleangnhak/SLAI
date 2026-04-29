import Link from "next/link";

import { AppShell } from "@/components/AppShell";
import { Card } from "@/components/Card";

const capabilities = [
  "Ledger-backed prepaid credits",
  "One active OmniRoute key in MVP",
  "Idempotent usage billing",
  "Manual admin top-ups"
];

export default function Home() {
  return (
    <AppShell section="public">
      <section className="grid gap-10 py-10 lg:grid-cols-[1.05fr_0.95fr] lg:py-16">
        <div className="max-w-3xl">
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Prepaid AI API credits</p>
          <h1 className="mt-6 text-4xl font-semibold tracking-normal text-slate-950 sm:text-5xl">SLAI</h1>
          <p className="mt-5 max-w-2xl text-lg leading-8 text-slate-600">
            Buy credits once, route model calls through OmniRoute, and keep billing anchored in a ledger that never uses floating point math.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <Link href="/signup" className="rounded-md bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800">
              Create account
            </Link>
            <Link href="/packages" className="rounded-md border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold text-slate-700 hover:bg-slate-50">
              View packages
            </Link>
          </div>
        </div>

        <Card className="self-start">
          <div className="flex items-center justify-between border-b border-slate-200 pb-4">
            <div>
              <p className="text-sm font-medium text-slate-500">Billing model</p>
              <p className="mt-2 text-3xl font-semibold text-slate-950">Prepaid</p>
            </div>
            <span className="rounded-md bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700">MVP ready</span>
          </div>
          <ul className="mt-5 space-y-3 text-sm text-slate-700">
            {capabilities.map((item) => (
              <li className="flex items-center gap-3" key={item}>
                <span className="size-2 rounded-full bg-cyan-600" />
                {item}
              </li>
            ))}
          </ul>
        </Card>
      </section>
    </AppShell>
  );
}
