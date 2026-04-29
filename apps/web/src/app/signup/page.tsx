"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

import { Button } from "@/components/Button";
import { Card } from "@/components/Card";
import { Input } from "@/components/Input";
import { api, readableError } from "@/lib/api";

export default function SignupPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await api.auth.signup(email, password);
      router.push("/dashboard");
      router.refresh();
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-[#f7f8fb] px-4 py-12 text-slate-950">
      <Card className="w-full max-w-md">
        <Link href="/" className="flex items-center gap-3">
          <span className="grid size-8 place-items-center rounded-md bg-slate-950 text-sm font-semibold text-white">S</span>
          <span className="text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
        </Link>
        <h1 className="mt-8 text-2xl font-semibold tracking-normal text-slate-950">Create account</h1>
        <p className="mt-2 text-sm text-slate-500">Signup creates a user and an empty ledger-backed credit balance.</p>
        <form className="mt-6 space-y-4" onSubmit={submit}>
          <Input label="Email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} required />
          <Input
            label="Password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            minLength={8}
            required
          />
          {error ? <p className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}
          <Button className="w-full" type="submit" disabled={loading}>
            {loading ? "Creating account" : "Create account"}
          </Button>
        </form>
        <p className="mt-5 text-sm text-slate-500">
          Already have an account? <Link className="font-semibold text-cyan-700" href="/login">Sign in</Link>
        </p>
      </Card>
    </main>
  );
}
