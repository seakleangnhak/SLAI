"use client";

import Link from "next/link";
import { useState, type InputHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

type AuthPageShellProps = {
  children: React.ReactNode;
};

type AuthCardProps = {
  children: React.ReactNode;
  footer: React.ReactNode;
  subtitle: string;
  title: string;
};

type AuthInputProps = InputHTMLAttributes<HTMLInputElement> & {
  error?: string;
  label: string;
};

export function AuthPageShell({ children }: AuthPageShellProps) {
  return (
    <main
      className="relative grid min-h-screen place-items-center overflow-hidden bg-[#f5f7fb] px-4 py-10 text-slate-950 sm:px-6"
      style={{
        backgroundImage:
          "linear-gradient(rgba(15,23,42,0.035) 1px, transparent 1px), linear-gradient(90deg, rgba(15,23,42,0.035) 1px, transparent 1px), radial-gradient(circle at 50% 0%, rgba(14,165,233,0.11), transparent 34rem)",
        backgroundSize: "32px 32px, 32px 32px, 100% 100%"
      }}
    >
      <div className="pointer-events-none absolute top-24 h-64 w-[min(46rem,86vw)] rounded-[3rem] bg-blue-300/20 blur-3xl" />
      {children}
    </main>
  );
}

export function AuthCard({ children, footer, subtitle, title }: AuthCardProps) {
  return (
    <section className="relative w-full max-w-[500px] rounded-3xl border border-white/80 bg-white/95 p-7 shadow-[0_24px_70px_rgba(15,23,42,0.15)] backdrop-blur sm:p-9">
      <AuthLogo />
      <div className="mt-9">
        <h1 className="text-3xl font-semibold tracking-[-0.02em] text-slate-950">{title}</h1>
        <p className="mt-3 text-sm leading-6 text-slate-500">{subtitle}</p>
      </div>
      {children}
      <div className="mt-6 border-t border-slate-200 pt-5 text-sm text-slate-500">{footer}</div>
    </section>
  );
}

export function AuthLogo() {
  return (
    <Link href="/" className="inline-flex items-center gap-3">
      <span className="grid size-10 place-items-center rounded-xl bg-slate-950 text-base font-semibold text-white shadow-sm">S</span>
      <span>
        <span className="block text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
        <span className="mt-0.5 block text-xs text-slate-500">Prepaid AI credits</span>
      </span>
    </Link>
  );
}

export function AuthInput({ className, error, label, ...props }: AuthInputProps) {
  return (
    <label className="block">
      <span className="text-sm font-medium text-slate-700">{label}</span>
      <input
        aria-invalid={Boolean(error)}
        className={cn(
          "mt-2 h-12 w-full rounded-xl border bg-white px-4 text-base text-slate-950 outline-none transition placeholder:text-slate-400 focus:ring-4 disabled:bg-slate-50 disabled:text-slate-500",
          error ? "border-red-300 ring-red-600/15 focus:border-red-500" : "border-slate-300 ring-blue-600/20 focus:border-blue-700",
          className
        )}
        {...props}
      />
      {error ? <span className="mt-1.5 block text-xs font-medium text-red-600">{error}</span> : null}
    </label>
  );
}

export function AuthPasswordInput({ error, label, ...props }: AuthInputProps) {
  const [visible, setVisible] = useState(false);
  const disabled = Boolean(props.disabled);

  return (
    <label className="block">
      <span className="text-sm font-medium text-slate-700">{label}</span>
      <div className="relative mt-2">
        <input
          aria-invalid={Boolean(error)}
          className={cn(
            "h-12 w-full rounded-xl border bg-white px-4 pr-20 text-base text-slate-950 outline-none transition placeholder:text-slate-400 focus:ring-4 disabled:bg-slate-50 disabled:text-slate-500",
            error ? "border-red-300 ring-red-600/15 focus:border-red-500" : "border-slate-300 ring-blue-600/20 focus:border-blue-700"
          )}
          type={visible ? "text" : "password"}
          {...props}
        />
        <button
          className="absolute right-2 top-1/2 min-h-8 -translate-y-1/2 rounded-lg px-3 text-sm font-semibold text-slate-600 transition hover:bg-slate-100 hover:text-slate-950 disabled:text-slate-400"
          type="button"
          onClick={() => setVisible((current) => !current)}
          disabled={disabled}
        >
          {visible ? "Hide" : "Show"}
        </button>
      </div>
      {error ? <span className="mt-1.5 block text-xs font-medium text-red-600">{error}</span> : null}
    </label>
  );
}

export function AuthErrorAlert({ message }: { message: string }) {
  return (
    <div className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-6 text-red-800" role="alert">
      {message}
    </div>
  );
}

export function SecurityNote() {
  return (
    <p className="mt-5 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm leading-6 text-slate-600">
      Sessions are stored in HttpOnly cookies and never in localStorage.
    </p>
  );
}
