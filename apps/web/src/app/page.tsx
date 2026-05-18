"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { AppShell } from "@/components/AppShell";
import { api, ApiError, type User } from "@/lib/api";

const primaryLink =
  "slai-marketing-primary inline-flex min-h-11 items-center justify-center rounded-xl bg-slate-950 px-5 py-3 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-800";
const secondaryLink =
  "slai-marketing-secondary inline-flex min-h-11 items-center justify-center rounded-xl border border-slate-200 bg-white px-5 py-3 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50";

const trustBullets = [
  "Credits never expire",
  "Ledger-backed balance",
  "Usage synced automatically",
  "One API key for MVP",
];

const steps = [
  {
    title: "Top up credits",
    text: "Choose a prepaid package and pay with Bakong KHQR. Credits are added after confirmation.",
  },
  {
    title: "Create API key",
    text: "Create one active SLAI key for server-side AI requests.",
  },
  {
    title: "Send AI requests",
    text: "Send model requests with your SLAI-created key from trusted backend code.",
  },
  {
    title: "Track usage",
    text: "Synced usage events debit your ledger-backed credit balance.",
  },
];

const features = [
  [
    "Prepaid credits",
    "Give developers a simple balance they can use without monthly surprises.",
  ],
  [
    "API key management",
    "Create, rotate, and revoke the one active key supported in the MVP.",
  ],
  [
    "Usage tracking",
    "Inspect usage events, token counts, providers, models, and status.",
  ],
  [
    "Credit ledger",
    "Keep top-ups, debits, adjustments, and balances in an auditable ledger.",
  ],
  [
    "Payment checkout",
    "Let developers choose prepaid packages and pay through Bakong KHQR confirmation.",
  ],
  [
    "Provider gateway",
    "Connect model calls to SLAI billing through synced provider usage logs.",
  ],
];

function CodeBlock() {
  return (
    <div className="slai-marketing-code overflow-hidden rounded-2xl border border-slate-800 bg-slate-950 shadow-2xl shadow-slate-950/15">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
        <span className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">
          Curl
        </span>
        <span className="rounded-md bg-blue-500/10 px-2 py-1 text-xs font-semibold text-blue-300">
          Server-side
        </span>
      </div>
      <pre className="overflow-x-auto p-5 text-sm leading-7 text-slate-100">
        <code>{`curl https://api.slai.shop/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_SLAI_API_KEY" \\
  -H "Content-Type: application/json"`}</code>
      </pre>
    </div>
  );
}

function ProductPreview() {
  return (
    <div className="slai-marketing-preview relative rounded-[1.5rem] border border-slate-200 bg-white p-4 shadow-2xl shadow-blue-950/10">
      <div className="slai-marketing-preview-glow absolute -right-10 -top-10 size-28 rounded-full bg-blue-200/60 blur-3xl" />
      <div className="slai-marketing-preview-glow absolute -bottom-10 left-10 size-28 rounded-full bg-violet-200/50 blur-3xl" />
      <div className="slai-marketing-preview-screen relative overflow-hidden rounded-2xl border border-slate-200 bg-slate-50">
        <div className="slai-marketing-preview-header flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
          <div className="flex items-center gap-2">
            <span className="grid size-7 place-items-center rounded-lg bg-slate-950 text-xs font-bold text-white">
              S
            </span>
            <span className="text-sm font-semibold text-slate-950">
              SLAI console
            </span>
          </div>
          <span className="rounded-md bg-emerald-50 px-2 py-1 text-xs font-semibold text-emerald-700">
            Prepaid
          </span>
        </div>
        <div className="grid gap-3 p-4 sm:grid-cols-2">
          {["Balance", "API key", "Usage", "Ledger"].map((label, index) => (
            <div
              key={label}
              className="slai-marketing-preview-tile rounded-xl border border-slate-200 bg-white p-4 shadow-sm"
            >
              <p className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">
                {label}
              </p>
              <div className="mt-4 h-2 rounded-full bg-slate-100">
                <div
                  className="h-2 rounded-full bg-gradient-to-r from-blue-600 to-cyan-500"
                  style={{ width: `${72 - index * 12}%` }}
                />
              </div>
              <div className="mt-4 flex gap-2">
                <span className="h-2 w-12 rounded-full bg-slate-200" />
                <span className="h-2 w-8 rounded-full bg-slate-100" />
              </div>
            </div>
          ))}
        </div>
        <div className="slai-marketing-preview-footer border-t border-slate-200 bg-white p-4">
          <div className="rounded-xl bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-100">
            <span className="text-blue-300">Authorization:</span> Bearer
            YOUR_SLAI_API_KEY
          </div>
        </div>
      </div>
    </div>
  );
}

function StepCard({
  index,
  title,
  text,
}: {
  index: number;
  title: string;
  text: string;
}) {
  return (
    <div className="slai-marketing-card rounded-2xl border border-slate-200 bg-white p-5 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md">
      <span className="slai-marketing-step-index grid size-9 place-items-center rounded-xl bg-blue-50 text-sm font-bold text-blue-700 ring-1 ring-blue-100">
        {index}
      </span>
      <h3 className="mt-5 text-base font-semibold text-slate-950">{title}</h3>
      <p className="mt-2 text-sm leading-6 text-slate-500">{text}</p>
    </div>
  );
}

function FeatureCard({
  title,
  text,
  index,
}: {
  title: string;
  text: string;
  index: number;
}) {
  return (
    <div className="slai-marketing-card rounded-2xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-blue-200 hover:shadow-md">
      <span className="slai-marketing-feature-index grid size-10 place-items-center rounded-xl bg-slate-950 text-sm font-bold text-white">
        {String(index + 1).padStart(2, "0")}
      </span>
      <h3 className="mt-5 text-base font-semibold text-slate-950">{title}</h3>
      <p className="mt-2 text-sm leading-6 text-slate-500">{text}</p>
    </div>
  );
}

export default function Home() {
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const loggedIn = currentUser !== null;

  useEffect(() => {
    api.auth
      .me()
      .then((response) => setCurrentUser(response.user))
      .catch((err) => {
        if (!(err instanceof ApiError) || err.status !== 401) {
          setCurrentUser(null);
        }
      });
  }, []);

  return (
    <AppShell section="public" user={currentUser}>
      <div className="slai-marketing-page">
        <div className="slai-marketing-hero relative overflow-hidden rounded-[2rem] border border-slate-200 bg-white px-5 py-12 shadow-sm sm:px-8 lg:px-10 lg:py-16">
          <div className="slai-marketing-grid absolute inset-0 bg-[linear-gradient(to_right,#e2e8f0_1px,transparent_1px),linear-gradient(to_bottom,#e2e8f0_1px,transparent_1px)] bg-[size:44px_44px] opacity-35" />
          <div className="slai-marketing-hero-glow absolute left-1/2 top-0 size-96 -translate-x-1/2 rounded-full bg-blue-200/50 blur-3xl" />
          <section className="relative grid gap-10 lg:grid-cols-[1fr_0.9fr] lg:items-center">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.2em] text-blue-700">
                Prepaid AI API credits
              </p>
              <h1 className="mt-5 max-w-4xl text-5xl font-semibold tracking-normal text-slate-950 sm:text-6xl lg:text-7xl">
                Prepaid AI API credits for developers
              </h1>
              <p className="mt-6 max-w-2xl text-lg leading-8 text-slate-600">
                Top up credits, create one API key, send AI requests, and track
                usage as your balance is deducted.
              </p>
              <div className="mt-8 flex flex-wrap gap-3">
                <Link
                  href={loggedIn ? "/dashboard" : "/signup"}
                  className={primaryLink}
                >
                  {loggedIn ? "Dashboard" : "Create account"}
                </Link>
                <Link href="/packages" className={secondaryLink}>
                  View packages
                </Link>
              </div>
              <div className="mt-8 grid gap-3 sm:grid-cols-2">
                {trustBullets.map((item) => (
                  <div
                    key={item}
                    className="slai-marketing-trust-pill flex items-center gap-3 rounded-xl border border-slate-200 bg-white/75 px-3 py-2 text-sm font-medium text-slate-700 shadow-sm backdrop-blur"
                  >
                    <span className="size-2 rounded-full bg-blue-600" />
                    {item}
                  </div>
                ))}
              </div>
            </div>
            <ProductPreview />
          </section>
        </div>

        <section id="how-it-works" className="mt-16 scroll-mt-24">
          <div className="max-w-2xl">
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-blue-700">
              How it works
            </p>
            <h2 className="mt-3 text-3xl font-semibold tracking-normal text-slate-950">
              From top-up to tracked usage
            </h2>
            <p className="mt-3 text-sm leading-6 text-slate-500">
              SLAI keeps the developer flow small: fund an account, use one key,
              and let synced usage settle against a ledger-backed balance.
            </p>
          </div>
          <div className="mt-8 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            {steps.map((step, index) => (
              <StepCard
                index={index + 1}
                key={step.title}
                title={step.title}
                text={step.text}
              />
            ))}
          </div>
        </section>

        <section className="mt-16">
          <div className="max-w-2xl">
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-blue-700">
              Platform
            </p>
            <h2 className="mt-3 text-3xl font-semibold tracking-normal text-slate-950">
              Built for prepaid AI operations
            </h2>
          </div>
          <div className="mt-8 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {features.map(([title, text], index) => (
              <FeatureCard
                index={index}
                key={title}
                title={title}
                text={text}
              />
            ))}
          </div>
        </section>

        <section className="slai-marketing-band mt-16 grid gap-8 rounded-[2rem] border border-slate-200 bg-white p-5 shadow-sm lg:grid-cols-[0.85fr_1.15fr] lg:p-8">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-blue-700">
              Quickstart
            </p>
            <h2 className="mt-3 text-3xl font-semibold tracking-normal text-slate-950">
              Use your SLAI-created key from a trusted server
            </h2>
            <p className="mt-3 text-sm leading-6 text-slate-500">
              Create an account, add credits from billing, then send AI requests
              with the key shown once after creation or rotation.
            </p>
          </div>
          <CodeBlock />
        </section>

        <section className="slai-marketing-payment mt-6 rounded-2xl border border-amber-200 bg-gradient-to-r from-amber-50 to-white p-5 shadow-sm">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 className="text-base font-semibold text-slate-950">
                Bakong KHQR checkout
              </h2>
              <p className="mt-1 text-sm leading-6 text-amber-800/80">
                Choose a package, scan KHQR, and SLAI credits your balance after
                payment confirmation.
              </p>
            </div>
            <Link href="/packages" className={secondaryLink}>
              View packages
            </Link>
          </div>
        </section>

        <section className="slai-marketing-cta mt-16 rounded-[2rem] bg-slate-950 p-8 text-white shadow-2xl shadow-slate-950/15 lg:p-10">
          <div className="flex flex-col gap-6 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 className="text-3xl font-semibold tracking-normal">
                Start with prepaid AI credits
              </h2>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300">
                {loggedIn
                  ? "Open your dashboard to monitor credits, key status, usage, and billing activity."
                  : "Create an account, add prepaid credits, and monitor usage from your developer dashboard."}
              </p>
            </div>
            <div className="flex flex-wrap gap-3">
              <Link
                href={loggedIn ? "/dashboard" : "/signup"}
                className="inline-flex min-h-11 items-center justify-center rounded-xl bg-white px-5 py-3 text-sm font-semibold text-slate-950 transition hover:bg-blue-50"
              >
                {loggedIn ? "Open dashboard" : "Create account"}
              </Link>
              <Link
                href={loggedIn ? "/dashboard/billing" : "/login"}
                className="inline-flex min-h-11 items-center justify-center rounded-xl border border-white/15 px-5 py-3 text-sm font-semibold text-white transition hover:bg-white/10"
              >
                {loggedIn ? "Open billing" : "Sign in"}
              </Link>
            </div>
          </div>
        </section>
      </div>
    </AppShell>
  );
}
