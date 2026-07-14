import Link from "next/link";

import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { CopyButton } from "@/components/CopyButton";
import { DashboardShell } from "@/components/DashboardShell";
import { cn } from "@/lib/cn";

const primaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg bg-slate-950 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-800";
const secondaryButton =
  "inline-flex min-h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 shadow-sm transition hover:border-slate-300 hover:bg-slate-50";

const curlExample = `curl https://api.slai.shop/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_SLAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-5.5",
    "messages": [
      { "role": "user", "content": "Write a one-sentence product summary." }
    ]
  }'`;

const javascriptExample = `const response = await fetch("https://api.slai.shop/v1/chat/completions", {
  method: "POST",
  headers: {
    Authorization: "Bearer YOUR_SLAI_API_KEY",
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    model: "gpt-5.5",
    messages: [{ role: "user", content: "Write a one-sentence product summary." }]
  })
});

const data = await response.json();`;

const codexSetupCommand = `npx seakleang-codex-setup`;

const codexDailyUse = `codex-slai
codex-slai-app
codex-slai-vscode /path/to/project`;

const codexProviderConfig = `model_provider = "slai"

[model_providers.slai]
name = "SLAI"
base_url = "https://api.slai.shop/v1"
wire_api = "responses"
env_key = "SLAI_API_KEY"`;

const claudeCodeSettings = `{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.slai.shop",
    "ANTHROPIC_AUTH_TOKEN": "YOUR_SLAI_API_KEY",
    "ANTHROPIC_MODEL": "YOUR_SLAI_MODEL"
  }
}`;

const claudeCodeShell = `export ANTHROPIC_BASE_URL=https://api.slai.shop
export ANTHROPIC_AUTH_TOKEN=YOUR_SLAI_API_KEY
export ANTHROPIC_MODEL=YOUR_SLAI_MODEL
claude`;

const quickSteps = [
  {
    title: "Add credits",
    body: "Choose a credit package or ask an admin to top up your account before sending production traffic.",
    href: "/dashboard/billing",
    label: "Open billing"
  },
  {
    title: "Create an API key",
    body: "SLAI shows the raw key once. Store it in your server environment and never ship it to browser code.",
    href: "/dashboard/api-key",
    label: "Manage key"
  },
  {
    title: "Call the SLAI API",
    body: "Use the SLAI-created key as a bearer token against the SLAI /v1 API.",
    href: "#quickstart",
    label: "View curl"
  }
];

const endpointRows = [
  ["API", "/v1/chat/completions", "Send chat completion requests through SLAI."],
  ["Dashboard", "/dashboard/usage", "Inspect synced usage events and billing status."],
  ["Dashboard", "/dashboard/billing", "Review credits, payments, and ledger history."],
  ["Dashboard", "/dashboard/api-key", "Create, rotate, or revoke your SLAI-managed key."]
];

const codexRows = [
  ["codex-slai", "Run Codex CLI with the isolated SLAI profile."],
  ["codex-slai-app", "Open Codex App with separate SLAI app data."],
  ["codex-slai-vscode /path/to/project", "Open VS Code with a SLAI Codex profile while reusing installed extensions."]
];

function CodeBlock({ title, code }: { title: string; code: string }) {
  return (
    <div className="overflow-hidden rounded-2xl border border-slate-800 bg-slate-950 shadow-sm">
      <div className="flex items-center justify-between gap-3 border-b border-white/10 px-4 py-2.5">
        <span className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-400">{title}</span>
        <CopyButton value={code} label="Copy" />
      </div>
      <pre className="overflow-x-auto p-4 text-xs leading-6 text-slate-100 sm:text-sm">
        <code>{code}</code>
      </pre>
    </div>
  );
}

function StepCard({ step, title, body, href, label }: { step: number; title: string; body: string; href: string; label: string }) {
  return (
    <Card className="rounded-2xl p-5 shadow-sm transition hover:border-blue-200 hover:shadow-md">
      <div className="flex h-full flex-col gap-4">
        <span className="grid size-9 place-items-center rounded-full bg-blue-50 text-sm font-semibold text-blue-700 ring-1 ring-blue-100">{step}</span>
        <div>
          <h2 className="text-base font-semibold text-slate-950">{title}</h2>
          <p className="mt-2 text-sm leading-6 text-slate-500">{body}</p>
        </div>
        <Link className={cn(secondaryButton, "mt-auto w-fit")} href={href}>{label}</Link>
      </div>
    </Card>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-3 py-2.5">
      <dt className="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</dt>
      <dd className="mt-1 break-words font-mono text-sm font-semibold text-slate-950">{value}</dd>
    </div>
  );
}

export default function DashboardDocsPage() {
  return (
    <DashboardShell>
      <div className="space-y-8">
        <section className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_18rem] lg:items-stretch">
          <div className="rounded-2xl border border-blue-200 bg-gradient-to-br from-white via-blue-50/70 to-cyan-50 p-6 shadow-sm">
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-blue-700">Developer docs</p>
            <h1 className="mt-3 max-w-3xl text-3xl font-semibold tracking-normal text-slate-950 sm:text-4xl">Start sending AI requests with prepaid SLAI credits.</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-slate-600 sm:text-base">
              SLAI manages credits, key lifecycle, and billing history. Your application sends OpenAI-compatible requests to the SLAI API with the raw key created in SLAI.
            </p>
            <div className="mt-5 flex flex-wrap gap-3">
              <Link className={primaryButton} href="/dashboard/api-key">Create API key</Link>
              <Link className={secondaryButton} href="/dashboard/billing">Add credits</Link>
            </div>
          </div>
          <Card className="rounded-2xl p-5">
            <CardHeader className="mb-4 block">
              <CardTitle>Runtime values</CardTitle>
              <CardDescription>Replace placeholders before deploying your integration.</CardDescription>
            </CardHeader>
            <dl className="space-y-3">
              <DetailRow label="API base URL" value="https://api.slai.shop/v1" />
              <DetailRow label="Header" value="Authorization: Bearer YOUR_SLAI_API_KEY" />
              <DetailRow label="Content type" value="application/json" />
            </dl>
          </Card>
        </section>

        <section className="grid gap-4 md:grid-cols-3">
          {quickSteps.map((item, index) => <StepCard key={item.title} step={index + 1} {...item} />)}
        </section>

        <section className="grid gap-5 xl:grid-cols-2" id="quickstart">
          <CodeBlock title="curl" code={curlExample} />
          <CodeBlock title="JavaScript" code={javascriptExample} />
        </section>

        <section className="grid gap-5 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]" id="codex-setup">
          <Card className="rounded-2xl p-5">
            <CardHeader className="mb-4 block">
              <CardTitle>SLAI Codex setup</CardTitle>
              <CardDescription>Use the setup package when you want Codex CLI, Codex App, or VS Code Codex traffic to run through SLAI without changing your default Codex profile.</CardDescription>
            </CardHeader>
            <div className="space-y-4 text-sm leading-6 text-slate-600">
              <p>The installer asks for your SLAI API key, creates an isolated profile at <code className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs text-slate-800">~/.codex-slai</code>, writes the SLAI provider config, and creates launcher commands.</p>
              <p>You can optionally import compatible Codex conversations and project state from <code className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs text-slate-800">~/.codex</code>. Your original Codex profile is not changed.</p>
            </div>
            <div className="mt-5 grid gap-3 sm:grid-cols-2">
              <DetailRow label="Profile" value="~/.codex-slai" />
              <DetailRow label="API key env" value="SLAI_API_KEY" />
            </div>
          </Card>

          <div className="space-y-5">
            <CodeBlock title="Install" code={codexSetupCommand} />
            <CodeBlock title="Daily use" code={codexDailyUse} />
          </div>
        </section>

        <section className="grid gap-5 lg:grid-cols-[minmax(0,1.1fr)_minmax(20rem,0.9fr)]">
          <Card className="overflow-hidden rounded-2xl p-0">
            <div className="border-b border-slate-200 px-5 py-4">
              <CardTitle>Codex launchers</CardTitle>
              <CardDescription>The setup creates these commands for the isolated SLAI profile.</CardDescription>
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50">
                  <tr>
                    {["Command", "Purpose"].map((label) => (
                      <th key={label} className="whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 bg-white">
                  {codexRows.map(([command, purpose]) => (
                    <tr key={command} className="hover:bg-slate-50">
                      <td className="whitespace-nowrap px-4 py-3 font-mono text-xs font-semibold text-slate-800">{command}</td>
                      <td className="px-4 py-3 text-slate-600">{purpose}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>

          <CodeBlock title="Provider config" code={codexProviderConfig} />
        </section>

        <section className="grid gap-5 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]" id="claude-code">
          <Card className="rounded-2xl p-5">
            <CardHeader className="mb-4 block">
              <CardTitle>Claude Code config</CardTitle>
              <CardDescription>Point Claude Code at SLAI by setting its API base URL and bearer token.</CardDescription>
            </CardHeader>
            <div className="space-y-4 text-sm leading-6 text-slate-600">
              <p>For a global setup, add the environment block to <code className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs text-slate-800">~/.claude/settings.json</code>.</p>
              <p>For one project only, use <code className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs text-slate-800">.claude/settings.local.json</code> so your raw key stays out of source control.</p>
            </div>
            <div className="mt-5 grid gap-3 sm:grid-cols-2">
              <DetailRow label="Base URL" value="https://api.slai.shop" />
              <DetailRow label="Auth token" value="YOUR_SLAI_API_KEY" />
            </div>
          </Card>

          <div className="space-y-5">
            <CodeBlock title="settings.json" code={claudeCodeSettings} />
            <CodeBlock title="Shell" code={claudeCodeShell} />
          </div>
        </section>

        <section className="grid gap-5 lg:grid-cols-[minmax(0,1.2fr)_minmax(20rem,0.8fr)]">
          <Card className="overflow-hidden rounded-2xl p-0">
            <div className="border-b border-slate-200 px-5 py-4">
              <CardTitle>Common surfaces</CardTitle>
              <CardDescription>Use these paths while building and debugging your integration.</CardDescription>
            </div>
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50">
                  <tr>
                    {["Surface", "Path", "Purpose"].map((label) => (
                      <th key={label} className="whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 bg-white">
                  {endpointRows.map(([method, path, purpose]) => (
                    <tr key={path} className="hover:bg-slate-50">
                      <td className="whitespace-nowrap px-4 py-3"><span className="rounded-md bg-slate-100 px-2 py-1 font-mono text-xs font-semibold text-slate-700">{method}</span></td>
                      <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-slate-700">{path}</td>
                      <td className="px-4 py-3 text-slate-600">{purpose}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>

          <Card className="rounded-2xl p-5">
            <CardHeader className="mb-4 block">
              <CardTitle>Billing behavior</CardTitle>
              <CardDescription>Usage and balance updates are applied after API calls.</CardDescription>
            </CardHeader>
            <div className="space-y-4 text-sm leading-6 text-slate-600">
              <p>Each successful model request is metered against your prepaid balance. SLAI records idempotent usage events and deducts the final credit cost from your credits.</p>
              <p>If balance reaches zero or below, SLAI pauses API access until more credits are added.</p>
              <p>Low balance and password changed security alerts are sent by email when notification delivery is configured.</p>
            </div>
            <div className="mt-5 grid gap-3 sm:grid-cols-2">
              <Link className={secondaryButton} href="/dashboard/usage">View usage</Link>
              <Link className={secondaryButton} href="/dashboard/settings">Security settings</Link>
            </div>
          </Card>
        </section>
      </div>
    </DashboardShell>
  );
}
