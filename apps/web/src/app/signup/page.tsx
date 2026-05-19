"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

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

type SignupErrors = {
  email?: string;
  otp?: string;
  password?: string;
};

export default function SignupPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [otp, setOtp] = useState("");
  const [password, setPassword] = useState("");
  const [verificationSent, setVerificationSent] = useState(false);
  const [expiresAt, setExpiresAt] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<SignupErrors>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (verificationSent) {
      await verifySignup();
      return;
    }

    await requestSignupOTP();
  }

  async function requestSignupOTP() {
    const validation = validateSignup(email, password);
    setFieldErrors(validation);
    if (Object.keys(validation).length > 0) {
      return;
    }

    setLoading(true);
    setError(null);
    try {
      const response = await api.auth.signup(email.trim(), password);
      setEmail(response.verification.email);
      setExpiresAt(response.verification.expiresAt);
      setVerificationSent(true);
      setOtp("");
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoading(false);
    }
  }

  async function verifySignup() {
    const validation = validateOTP(otp);
    setFieldErrors(validation);
    if (Object.keys(validation).length > 0) {
      return;
    }

    setLoading(true);
    setError(null);
    try {
      await api.auth.verifySignup(email.trim(), otp.trim());
      router.push("/dashboard");
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
      router.push("/dashboard");
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
        title={verificationSent ? "Verify email" : "Create account"}
        subtitle={verificationSent ? `Enter the 6-digit code sent to ${email}.` : "Create your SLAI developer account and start with an empty ledger-backed balance."}
        footer={
          <>
            <p>
              Already have an account?{" "}
              <Link
                className="font-semibold text-blue-700 hover:text-blue-800"
                href="/login"
              >
                Sign in
              </Link>
            </p>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
              <Link
                className="font-semibold text-slate-700 hover:text-slate-950"
                href="/"
              >
                Back to home
              </Link>
            </div>
          </>
        }
      >
        <div className="mt-7 space-y-5">
          {error ? <AuthErrorAlert message={error} /> : null}
          {!verificationSent ? (
            <>
              <GoogleAuthButton
                label="Sign up with Google"
                onCredential={continueWithGoogle}
                onError={setError}
              />
              <AuthDivider />
            </>
          ) : null}
        </div>
        <form className="mt-5 space-y-5" onSubmit={submit} noValidate>
          {verificationSent ? (
            <>
              <AuthInput
                autoComplete="one-time-code"
                disabled={loading}
                error={fieldErrors.otp}
                inputMode="numeric"
                label="Verification code"
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
                <p className="text-xs leading-5 text-slate-500">Code expires at {new Date(expiresAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}.</p>
              ) : null}
            </>
          ) : (
            <>
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
                autoComplete="new-password"
                disabled={loading}
                error={fieldErrors.password}
                label="Password"
                minLength={8}
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
            </>
          )}
          <Button
            className="h-12 w-full rounded-xl bg-slate-950 text-base shadow-sm hover:bg-slate-800"
            type="submit"
            disabled={loading}
          >
            {loading ? (verificationSent ? "Verifying..." : "Sending code...") : verificationSent ? "Verify and continue" : "Create account"}
          </Button>
          {verificationSent ? (
            <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
              <button className="font-semibold text-blue-700 hover:text-blue-800 disabled:text-slate-400" disabled={loading} onClick={requestSignupOTP} type="button">
                Resend code
              </button>
              <button className="font-semibold text-slate-600 hover:text-slate-950 disabled:text-slate-400" disabled={loading} onClick={() => {
                setVerificationSent(false);
                setOtp("");
                setExpiresAt(null);
                setError(null);
                setFieldErrors({});
              }} type="button">
                Change email
              </button>
            </div>
          ) : null}
          <p className="text-xs leading-5 text-slate-500">
            By creating an account, you agree to the{" "}
            <Link
              className="font-semibold text-blue-700 hover:text-blue-800"
              href="/terms"
            >
              Terms
            </Link>{" "}
            and acknowledge the{" "}
            <Link
              className="font-semibold text-blue-700 hover:text-blue-800"
              href="/privacy"
            >
              Privacy Policy
            </Link>
            .
          </p>
        </form>
        <SecurityNote />
      </AuthCard>
    </AuthPageShell>
  );
}

function validateOTP(otp: string): SignupErrors {
  const errors: SignupErrors = {};
  if (!otp.trim()) {
    errors.otp = "Verification code is required.";
  } else if (!/^\d{6}$/.test(otp.trim())) {
    errors.otp = "Enter the 6-digit code.";
  }
  return errors;
}

function validateSignup(email: string, password: string): SignupErrors {
  const errors: SignupErrors = {};
  if (!email.trim()) {
    errors.email = "Email is required.";
  } else if (!looksLikeEmail(email)) {
    errors.email = "Enter a valid email address.";
  }
  if (!password) {
    errors.password = "Password is required.";
  } else if (password.length < 8) {
    errors.password = "Password must be at least 8 characters.";
  }
  return errors;
}

function looksLikeEmail(value: string) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}
