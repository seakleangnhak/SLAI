"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useState } from "react";

import { Button } from "@/components/Button";
import { Card } from "@/components/Card";
import { Input } from "@/components/Input";
import { api, readableError } from "@/lib/api";
import { isAdmin } from "@/lib/auth";

export default function AdminLoginPage() {
  const router = useRouter();
  const [nextPath, setNextPath] = useState("/admin");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
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
    <main className="grid min-h-screen place-items-center bg-[#f7f8fb] px-4 py-12 text-slate-950">
      <Card className="w-full max-w-md">
        <Link href="/" className="flex items-center gap-3">
          <span className="grid size-8 place-items-center rounded-md bg-slate-950 text-sm font-semibold text-white">S</span>
          <span className="text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
        </Link>
        <p className="mt-8 text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin Console</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-normal text-slate-950">Sign in to admin</h1>
        <p className="mt-2 text-sm leading-6 text-slate-500">Admin access only. Sessions are stored in HttpOnly cookies.</p>

        <form className="mt-6 space-y-4" onSubmit={submit}>
          <Input label="Email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} required />
          <Input label="Password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} required />
          {error ? <p className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}
          <Button className="w-full" type="submit" disabled={signingIn}>
            {loading ? "Signing in" : checkingSession ? "Checking session" : "Continue"}
          </Button>
        </form>

        <div className="mt-5 flex flex-wrap items-center justify-between gap-3 text-sm text-slate-500">
          <Link className="font-semibold text-cyan-700" href="/login">User login</Link>
          <Link className="font-semibold text-slate-700" href="/">Back to home</Link>
        </div>
      </Card>
    </main>
  );
}
