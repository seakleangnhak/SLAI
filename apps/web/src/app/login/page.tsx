import Link from "next/link";

export default function LoginPage() {
  return (
    <main className="grid min-h-screen place-items-center bg-[#f7f8fb] px-4 py-12 text-slate-950">
      <section className="w-full max-w-md rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <Link href="/" className="flex items-center gap-3">
          <span className="grid size-8 place-items-center rounded-md bg-slate-950 text-sm font-semibold text-white">S</span>
          <span className="text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
        </Link>
        <h1 className="mt-8 text-2xl font-semibold tracking-normal text-slate-950">Sign in</h1>
        <form className="mt-6 space-y-4">
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Email</span>
            <input
              className="mt-2 w-full rounded-md border border-slate-300 px-3 py-2 text-sm outline-none ring-cyan-600/20 focus:border-cyan-700 focus:ring-4"
              type="email"
              placeholder="developer@example.com"
            />
          </label>
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Password</span>
            <input
              className="mt-2 w-full rounded-md border border-slate-300 px-3 py-2 text-sm outline-none ring-cyan-600/20 focus:border-cyan-700 focus:ring-4"
              type="password"
              placeholder="********"
            />
          </label>
          <button className="w-full rounded-md bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800" type="button">
            Continue
          </button>
        </form>
      </section>
    </main>
  );
}
