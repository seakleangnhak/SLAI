import Link from "next/link";

const stats = [
  { label: "Balance model", value: "Integer credits" },
  { label: "Gateway", value: "OmniRoute" },
  { label: "MVP keys", value: "One active key" }
];

export default function Home() {
  return (
    <main className="min-h-screen bg-[#f7f8fb] text-slate-950">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <Link href="/" className="flex items-center gap-3">
            <span className="grid size-8 place-items-center rounded-md bg-slate-950 text-sm font-semibold text-white">S</span>
            <span className="text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
          </Link>
          <nav className="flex items-center gap-2">
            <Link href="/login" className="rounded-md px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100">
              Login
            </Link>
            <Link href="/dashboard" className="rounded-md bg-slate-950 px-3 py-2 text-sm font-medium text-white hover:bg-slate-800">
              Dashboard
            </Link>
          </nav>
        </div>
      </header>

      <section className="mx-auto grid max-w-7xl gap-10 px-4 py-16 sm:px-6 lg:grid-cols-[1.1fr_0.9fr] lg:px-8 lg:py-24">
        <div className="max-w-3xl">
          <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Prepaid AI API credits</p>
          <h1 className="mt-6 text-4xl font-semibold tracking-normal text-slate-950 sm:text-5xl">SLAI</h1>
          <p className="mt-5 max-w-2xl text-lg leading-8 text-slate-600">
            Buy credits once, route model calls through OmniRoute, and keep billing anchored in a ledger that never uses floating point math.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <Link href="/dashboard" className="rounded-md bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800">
              Open dashboard
            </Link>
            <Link href="/admin" className="rounded-md border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold text-slate-700 hover:bg-slate-50">
              Admin console
            </Link>
          </div>
        </div>

        <aside className="rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
          <div className="flex items-center justify-between border-b border-slate-200 pb-4">
            <div>
              <p className="text-sm font-medium text-slate-500">Available credits</p>
              <p className="mt-2 text-3xl font-semibold text-slate-950">128,000</p>
            </div>
            <span className="rounded-md bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700">Active</span>
          </div>
          <dl className="mt-5 grid gap-4 sm:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3">
            {stats.map((stat) => (
              <div key={stat.label}>
                <dt className="text-xs font-medium uppercase tracking-[0.12em] text-slate-400">{stat.label}</dt>
                <dd className="mt-1 text-sm font-semibold text-slate-800">{stat.value}</dd>
              </div>
            ))}
          </dl>
          <div className="mt-6 rounded-lg border border-slate-200 bg-slate-50 p-4">
            <div className="flex items-center justify-between text-sm">
              <span className="font-medium text-slate-600">OmniRoute usage sync</span>
              <span className="font-semibold text-slate-950">call_logs</span>
            </div>
            <div className="mt-4 h-2 rounded-full bg-slate-200">
              <div className="h-2 w-2/3 rounded-full bg-cyan-600" />
            </div>
          </div>
        </aside>
      </section>
    </main>
  );
}
