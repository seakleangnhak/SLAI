"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useState } from "react";

import { Button } from "@/components/Button";
import { api, readableError } from "@/lib/api";
import { isAdmin } from "@/lib/auth";

export default function AdminLoginPage() {
  const router = useRouter();
  const [nextPath, setNextPath] = useState("/admin");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [checkingSession, setCheckingSession] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const next = new URLSearchParams(window.location.search).get("next");
    if (next?.startsWith("/admin")) {
      setNextPath(next);
    }

    let cancelled = false;
    api.auth
      .me()
      .then((response) => {
        if (cancelled) {
          return;
        }
        if (isAdmin(response.user)) {
          router.replace(next?.startsWith("/admin") ? next : "/admin");
          return;
        }
        setError("This account does not have admin access.");
      })
      .catch((err) => {
        if (!cancelled && err?.status !== 401) {
          setError(readableError(err));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setCheckingSession(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [router]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError(null);

    try {
      await api.auth.login(email, password);
      const response = await api.auth.me();
      if (!isAdmin(response.user)) {
        await api.auth.logout();
        setError("This account does not have admin access.");
        return;
      }
      router.push(nextPath);
      router.refresh();
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  const signingIn = loading || checkingSession;

  return (
    <main
      className="relative grid min-h-screen place-items-center overflow-hidden bg-[#f5f7fb] px-4 py-10 text-slate-950 sm:px-6"
      style={{
        backgroundImage:
          "linear-gradient(rgba(15,23,42,0.035) 1px, transparent 1px), linear-gradient(90deg, rgba(15,23,42,0.035) 1px, transparent 1px), radial-gradient(circle at 50% 0%, rgba(8,145,178,0.10), transparent 34rem)",
        backgroundSize: "32px 32px, 32px 32px, 100% 100%"
      }}
    >
      <div className="pointer-events-none absolute top-24 h-60 w-[min(42rem,82vw)] rounded-[3rem] bg-cyan-300/20 blur-3xl" />

      <section className="relative w-full max-w-[440px] rounded-2xl border border-white/80 bg-white/95 p-7 shadow-[0_24px_70px_rgba(15,23,42,0.16)] backdrop-blur sm:p-9">
        <Link href="/" className="inline-flex items-center gap-3">
          <span className="grid size-10 place-items-center rounded-xl bg-slate-950 text-base font-semibold text-white shadow-sm">S</span>
          <span className="text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
        </Link>

        <div className="mt-9">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin Console</p>
          <h1 className="mt-3 text-2xl font-semibold tracking-normal text-slate-950 sm:text-3xl">Sign in to Admin Console</h1>
          <p className="mt-3 text-sm leading-6 text-slate-500">
            Secure access for managing credits, users, API keys, and usage.
          </p>
        </div>

        <form className="mt-7 space-y-5" onSubmit={submit}>
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Email</span>
            <input
              className="mt-2 h-12 w-full rounded-xl border border-slate-300 bg-white px-4 text-base text-slate-950 outline-none ring-cyan-600/20 placeholder:text-slate-400 transition focus:border-cyan-700 focus:ring-4 disabled:bg-slate-50 disabled:text-slate-500"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="email"
              disabled={checkingSession}
              required
            />
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-700">Password</span>
            <div className="relative mt-2">
              <input
                className="h-12 w-full rounded-xl border border-slate-300 bg-white px-4 pr-20 text-base text-slate-950 outline-none ring-cyan-600/20 placeholder:text-slate-400 transition focus:border-cyan-700 focus:ring-4 disabled:bg-slate-50 disabled:text-slate-500"
                type={showPassword ? "text" : "password"}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                disabled={checkingSession}
                required
              />
              <button
                className="absolute right-2 top-1/2 min-h-8 -translate-y-1/2 rounded-lg px-3 text-sm font-semibold text-slate-600 hover:bg-slate-100 hover:text-slate-950 disabled:text-slate-400"
                type="button"
                onClick={() => setShowPassword((visible) => !visible)}
                disabled={checkingSession}
              >
                {showPassword ? "Hide" : "Show"}
              </button>
            </div>
          </label>

          {error ? (
            <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-6 text-red-800" role="alert">
              {error}
            </div>
          ) : null}

          <Button className="h-12 w-full rounded-xl bg-slate-950 text-base shadow-sm hover:bg-slate-800" type="submit" disabled={signingIn}>
            {loading ? "Signing in..." : checkingSession ? "Checking session..." : "Sign in to admin"}
          </Button>
        </form>

        <p className="mt-5 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm leading-6 text-slate-600">
          Protected by HttpOnly sessions. Admin actions are audit logged.
        </p>

        <div className="mt-6 flex flex-wrap items-center justify-between gap-3 text-sm text-slate-500">
          <Link className="font-semibold text-cyan-700 hover:text-cyan-800" href="/login">User login</Link>
          <Link className="font-semibold text-slate-700 hover:text-slate-950" href="/">Back to home</Link>
        </div>
      </section>
    </main>
  );
}
