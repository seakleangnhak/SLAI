"use client";

import Link from "next/link";
import { FormEvent, useState } from "react";

import {
  AuthCard,
  AuthErrorAlert,
  AuthInput,
  AuthPageShell,
  AuthPasswordInput,
  SecurityNote,
} from "@/components/AuthPage";
import { Button } from "@/components/Button";
import { api, readableError } from "@/lib/api";

type ResetStep = "request" | "confirm" | "success";

type ResetErrors = {
  email?: string;
  otp?: string;
  password?: string;
};

export default function PasswordResetPage() {
  const [step, setStep] = useState<ResetStep>("request");
  const [email, setEmail] = useState("");
  const [otp, setOtp] = useState("");
  const [password, setPassword] = useState("");
  const [expiresAt, setExpiresAt] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<ResetErrors>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (step === "confirm") {
      await confirmReset();
      return;
    }
    if (step === "request") {
      await requestReset();
    }
  }

  async function requestReset() {
    const validation = validateEmail(email);
    setFieldErrors(validation);
    if (Object.keys(validation).length > 0) {
      return;
    }

    setLoading(true);
    setError(null);
    try {
      const response = await api.auth.requestPasswordReset(email.trim());
      setEmail(response.verification.email);
      setExpiresAt(response.verification.expiresAt);
      setOtp("");
      setPassword("");
      setStep("confirm");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  async function confirmReset() {
    const validation = validateReset(otp, password);
    setFieldErrors(validation);
    if (Object.keys(validation).length > 0) {
      return;
    }

    setLoading(true);
    setError(null);
    try {
      await api.auth.confirmPasswordReset(email.trim(), otp.trim(), password);
      setStep("success");
      setOtp("");
      setPassword("");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthPageShell>
      <AuthCard
        title={step === "success" ? "Password updated" : step === "confirm" ? "Reset password" : "Forgot password"}
        subtitle={
          step === "success"
            ? "Your password has been changed. Sign in with your new password."
            : step === "confirm"
              ? `Enter the 6-digit code sent to ${email}.`
              : "Enter your account email and we will send a reset code if the account can use password sign-in."
        }
        footer={
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Link className="font-semibold text-blue-700 hover:text-blue-800" href="/login">
              Back to sign in
            </Link>
            <Link className="font-semibold text-slate-600 hover:text-slate-950" href="/signup">
              Create account
            </Link>
          </div>
        }
      >
        <div className="mt-7 space-y-5">
          {error ? <AuthErrorAlert message={error} /> : null}
        </div>
        {step === "success" ? (
          <div className="mt-5">
            <Link
              className="inline-flex h-12 w-full items-center justify-center rounded-xl bg-slate-950 px-4 text-base font-semibold text-white shadow-sm transition hover:bg-slate-800"
              href="/login"
            >
              Continue to sign in
            </Link>
          </div>
        ) : (
          <form className="mt-5 space-y-5" onSubmit={submit} noValidate>
            {step === "request" ? (
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
            ) : (
              <>
                <AuthInput
                  autoComplete="one-time-code"
                  disabled={loading}
                  error={fieldErrors.otp}
                  inputMode="numeric"
                  label="Reset code"
                  maxLength={6}
                  onChange={(event) => {
                    setOtp(event.target.value.replace(/\D/g, "").slice(0, 6));
                    setFieldErrors((current) => ({ ...current, otp: undefined }));
                  }}
                  placeholder="000000"
                  type="text"
                  value={otp}
                />
                {expiresAt ? (
                  <p className="text-xs leading-5 text-slate-500">
                    Code expires at {new Date(expiresAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}.
                  </p>
                ) : null}
                <AuthPasswordInput
                  autoComplete="new-password"
                  disabled={loading}
                  error={fieldErrors.password}
                  label="New password"
                  minLength={8}
                  onChange={(event) => {
                    setPassword(event.target.value);
                    setFieldErrors((current) => ({ ...current, password: undefined }));
                  }}
                  placeholder="Enter a new password"
                  value={password}
                />
              </>
            )}
            <Button
              className="h-12 w-full rounded-xl bg-slate-950 text-base shadow-sm hover:bg-slate-800"
              type="submit"
              disabled={loading}
            >
              {loading ? (step === "confirm" ? "Updating..." : "Sending code...") : step === "confirm" ? "Update password" : "Send reset code"}
            </Button>
            {step === "confirm" ? (
              <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
                <button className="font-semibold text-blue-700 hover:text-blue-800 disabled:text-slate-400" disabled={loading} onClick={requestReset} type="button">
                  Resend code
                </button>
                <button className="font-semibold text-slate-600 hover:text-slate-950 disabled:text-slate-400" disabled={loading} onClick={() => {
                  setStep("request");
                  setOtp("");
                  setPassword("");
                  setExpiresAt(null);
                  setError(null);
                  setFieldErrors({});
                }} type="button">
                  Change email
                </button>
              </div>
            ) : null}
          </form>
        )}
        <SecurityNote />
      </AuthCard>
    </AuthPageShell>
  );
}

function validateEmail(email: string): ResetErrors {
  const errors: ResetErrors = {};
  if (!email.trim()) {
    errors.email = "Email is required.";
  } else if (!looksLikeEmail(email)) {
    errors.email = "Enter a valid email address.";
  }
  return errors;
}

function validateReset(otp: string, password: string): ResetErrors {
  const errors = validateOTP(otp);
  if (!password) {
    errors.password = "New password is required.";
  } else if (password.length < 8) {
    errors.password = "Password must be at least 8 characters.";
  }
  return errors;
}

function validateOTP(otp: string): ResetErrors {
  const errors: ResetErrors = {};
  if (!otp.trim()) {
    errors.otp = "Reset code is required.";
  } else if (!/^\d{6}$/.test(otp.trim())) {
    errors.otp = "Enter the 6-digit code.";
  }
  return errors;
}

function looksLikeEmail(value: string) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}
