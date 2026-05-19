"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useState } from "react";

import {
  AuthCard,
  AuthDivider,
  AuthErrorAlert,
  AuthInput,
  AuthPageShell,
  AuthPasswordInput,
  GoogleAuthButton,
  SecurityNote,
} from "@/components/AuthPage";
import { Button } from "@/components/Button";
import { api, readableError } from "@/lib/api";

type LoginErrors = {
  email?: string;
  password?: string;
};

export default function LoginPage() {
  const router = useRouter();
  const [nextPath, setNextPath] = useState("/dashboard");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [fieldErrors, setFieldErrors] = useState<LoginErrors>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const next = new URLSearchParams(window.location.search).get("next");
    if (next?.startsWith("/dashboard")) {
      setNextPath(next);
    }
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const validation = validateLogin(email, password);
    setFieldErrors(validation);
    if (Object.keys(validation).length > 0) {
      return;
    }

    setLoading(true);
    setError(null);
    try {
      await api.auth.login(email.trim(), password);
      router.push(nextPath);
      router.refresh();
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  async function continueWithGoogle(credential: string) {
    setLoading(true);
    setError(null);
    try {
      await api.auth.google(credential);
      router.push(nextPath);
      router.refresh();
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthPageShell>
      <AuthCard
        title="Sign in"
        subtitle="Use your SLAI account. Sessions are stored in HttpOnly cookies."
        footer={
          <>
            <p>
              No account yet?{" "}
              <Link
                className="font-semibold text-blue-700 hover:text-blue-800"
                href="/signup"
              >
                Create one
              </Link>
            </p>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
              <Link
                className="font-semibold text-slate-700 hover:text-slate-950"
                href="/"
              >
                Back to home
              </Link>
              <div className="flex flex-wrap gap-3">
                <Link
                  className="font-semibold text-slate-500 hover:text-slate-950"
                  href="/terms"
                >
                  Terms
                </Link>
                <Link
                  className="font-semibold text-slate-500 hover:text-slate-950"
                  href="/privacy"
                >
                  Privacy
                </Link>
                <Link
                  className="font-semibold text-slate-500 hover:text-slate-950"
                  href="/admin/login"
                >
                  Admin console
                </Link>
              </div>
            </div>
          </>
        }
      >
        <div className="mt-7 space-y-5">
          {error ? <AuthErrorAlert message={error} /> : null}
          <GoogleAuthButton
            label="Continue with Google"
            onCredential={continueWithGoogle}
            onError={setError}
          />
          <AuthDivider />
        </div>
        <form className="mt-5 space-y-5" onSubmit={submit} noValidate>
          <AuthInput
            autoComplete="email"
            disabled={loading}
            error={fieldErrors.email}
            label="Email"
            onChange={(event) => {
              setEmail(event.target.value);
              setFieldErrors((current) => ({ ...current, email: undefined }));
            }}
            placeholder="name@company.com"
            type="email"
            value={email}
          />
          <AuthPasswordInput
            autoComplete="current-password"
            disabled={loading}
            error={fieldErrors.password}
            label="Password"
            onChange={(event) => {
              setPassword(event.target.value);
              setFieldErrors((current) => ({
                ...current,
                password: undefined,
              }));
            }}
            placeholder="Enter your password"
            value={password}
          />
          <Button
            className="h-12 w-full rounded-xl bg-slate-950 text-base shadow-sm hover:bg-slate-800"
            type="submit"
            disabled={loading}
          >
            {loading ? "Signing in..." : "Continue"}
          </Button>
        </form>
        <SecurityNote />
      </AuthCard>
    </AuthPageShell>
  );
}

function validateLogin(email: string, password: string): LoginErrors {
  const errors: LoginErrors = {};
  if (!email.trim()) {
    errors.email = "Email is required.";
  } else if (!looksLikeEmail(email)) {
    errors.email = "Enter a valid email address.";
  }
  if (!password) {
    errors.password = "Password is required.";
  }
  return errors;
}

function looksLikeEmail(value: string) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}
