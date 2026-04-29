"use client";

import { FormEvent, useEffect, useMemo, useRef, useState, type InputHTMLAttributes, type Ref, type TextareaHTMLAttributes } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { LoadingState } from "@/components/LoadingState";
import { api, readableError, type CreditPackage, type PackageInput } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatCompactUnits, formatDateTime, formatMoney, formatUnits } from "@/lib/format";

type PackageForm = {
  name: string;
  description: string;
  creditUnits: string;
  bonusCreditUnits: string;
  price: string;
  currency: string;
  active: boolean;
  sortOrder: string;
};

type PanelState =
  | { mode: "create" }
  | { mode: "edit"; package: CreditPackage }
  | null;

type StatusFilter = "all" | "active" | "inactive";

type FieldErrors = Partial<Record<keyof PackageForm, string>>;

type ValidationResult = {
  errors: FieldErrors;
  valid: boolean;
};

type ParsedMoney =
  | { ok: true; minor: number }
  | { ok: false; error: string };

const emptyForm: PackageForm = {
  name: "",
  description: "",
  creditUnits: "1000",
  bonusCreditUnits: "0",
  price: "10.00",
  currency: "USD",
  active: true,
  sortOrder: "0"
};

function formToInput(form: PackageForm): PackageInput {
  const parsedPrice = parseMoneyToMinor(form.price);
  if (!parsedPrice.ok) {
    throw new Error(parsedPrice.error);
  }
  return {
    name: form.name.trim(),
    description: form.description.trim() || null,
    creditUnits: Number(form.creditUnits),
    bonusCreditUnits: Number(form.bonusCreditUnits),
    priceMinor: parsedPrice.minor,
    currency: form.currency.trim().toUpperCase(),
    active: form.active,
    sortOrder: Number(form.sortOrder)
  };
}

function packageToForm(pkg: CreditPackage): PackageForm {
  return {
    name: pkg.name,
    description: pkg.description ?? "",
    creditUnits: String(pkg.creditUnits),
    bonusCreditUnits: String(pkg.bonusCreditUnits),
    price: minorToMoney(pkg.priceMinor),
    currency: pkg.currency,
    active: pkg.active,
    sortOrder: String(pkg.sortOrder)
  };
}

export default function AdminPackagesPage() {
  const [packages, setPackages] = useState<CreditPackage[]>([]);
  const [form, setForm] = useState<PackageForm>(emptyForm);
  const [panel, setPanel] = useState<PanelState>(null);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [activeActionId, setActiveActionId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [notice, setNotice] = useState<string | null>(null);

  function load() {
    setLoading(true);
    setError(null);
    api.admin.packages
      .list()
      .then((response) => setPackages(response.packages))
      .catch((err) => setError(readableError(err)))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setForm(emptyForm);
    setFormError(null);
    setFieldErrors(validateForm(emptyForm).errors);
    setPanel({ mode: "create" });
  }

  function openEdit(pkg: CreditPackage) {
    const nextForm = packageToForm(pkg);
    setForm(nextForm);
    setFormError(null);
    setFieldErrors(validateForm(nextForm).errors);
    setPanel({ mode: "edit", package: pkg });
  }

  async function submitPackage(event: FormEvent) {
    event.preventDefault();
    const validation = validateForm(form);
    setFieldErrors(validation.errors);
    if (!validation.valid) {
      setFormError("Fix the highlighted fields before saving.");
      return;
    }
    setActionLoading(true);
    setFormError(null);
    setError(null);
    setNotice(null);
    try {
      const input = formToInput(form);
      if (panel?.mode === "edit") {
        await api.admin.packages.update(panel.package.id, input);
        setNotice("Package updated.");
      } else {
        await api.admin.packages.create(input);
        setNotice("Package created.");
      }
      setPanel(null);
      await api.admin.packages.list().then((response) => setPackages(response.packages));
    } catch (err) {
      setFormError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  async function toggleActive(pkg: CreditPackage) {
    const nextActive = !pkg.active;
    const message = nextActive
      ? `Activate ${pkg.name}? It will be visible to developers.`
      : `Deactivate ${pkg.name}? It will be hidden from public packages.`;
    if (!window.confirm(message)) {
      return;
    }
    setActiveActionId(pkg.id);
    setError(null);
    setNotice(null);
    try {
      await api.admin.packages.update(pkg.id, { active: nextActive });
      setNotice(nextActive ? "Package activated." : "Package deactivated.");
      await api.admin.packages.list().then((response) => setPackages(response.packages));
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActiveActionId(null);
    }
  }

  const summary = useMemo(() => {
    const active = packages.filter((pkg) => pkg.active).length;
    const inactive = packages.length - active;
    const totalCredits = packages.reduce((sum, pkg) => sum + pkg.creditUnits + pkg.bonusCreditUnits, 0);
    const currencies = new Set(packages.map((pkg) => pkg.currency));
    const hasSingleCurrency = currencies.size <= 1;
    const averagePrice = packages.length === 0 ? 0 : hasSingleCurrency ? Math.round(packages.reduce((sum, pkg) => sum + pkg.priceMinor, 0) / packages.length) : null;
    const currency = hasSingleCurrency ? packages[0]?.currency ?? "USD" : null;
    return { active, inactive, totalCredits, averagePrice, currency, hasSingleCurrency };
  }, [packages]);

  const visiblePackages = useMemo(() => {
    const query = search.trim().toLowerCase();
    return packages.filter((pkg) => {
      if (statusFilter === "active" && !pkg.active) {
        return false;
      }
      if (statusFilter === "inactive" && pkg.active) {
        return false;
      }
      if (!query) {
        return true;
      }
      return pkg.name.toLowerCase().includes(query) || (pkg.description ?? "").toLowerCase().includes(query);
    });
  }, [packages, search, statusFilter]);

  return (
    <AdminShell>
      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">Admin packages</p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-0.02em] text-slate-950">Credit packages</h1>
          <p className="mt-1 text-sm text-slate-500">Manage prepaid credit bundles shown to developers.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={openCreate}>Create package</Button>
          <Button type="button" variant="secondary" onClick={load} disabled={loading}>Refresh</Button>
        </div>
      </div>

      <section className="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <SummaryCard label="Total packages" value={formatUnits(packages.length)} hint="Admin catalog" />
        <SummaryCard label="Active" value={formatUnits(summary.active)} hint="Visible to developers" tone="green" />
        <SummaryCard label="Inactive" value={formatUnits(summary.inactive)} hint="Hidden from public list" tone="yellow" />
        <SummaryCard label="Credits offered" value={formatCompactUnits(summary.totalCredits)} hint="Included plus bonus" tone="blue" />
        <SummaryCard
          label="Average price"
          value={summary.averagePrice === null || summary.currency === null ? "Mixed" : formatMoney(summary.averagePrice, summary.currency)}
          hint={summary.hasSingleCurrency ? "Across loaded packages" : "Multiple currencies"}
        />
      </section>

      {error ? <div className="mt-6"><ErrorState message={error} onRetry={load} /></div> : null}
      {notice ? <p className="mt-6 rounded-lg border border-blue-100 bg-blue-50 px-3 py-2 text-sm text-blue-800">{notice}</p> : null}

      <Card className="mt-6 p-4">
        <CardHeader className="mb-4">
          <div>
            <CardTitle>Package catalog</CardTitle>
            <CardDescription>Active packages are returned by the public packages endpoint.</CardDescription>
          </div>
        </CardHeader>
        <div className="mb-4 grid gap-3 lg:grid-cols-[minmax(260px,1fr)_auto] lg:items-end">
          <label className="block">
            <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">Search</span>
            <input
              className="mt-2 h-10 w-full rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-950 outline-none ring-blue-600/20 placeholder:text-slate-400 focus:border-blue-600 focus:ring-4"
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search packages..."
              value={search}
            />
          </label>
          <div className="flex rounded-lg border border-slate-200 bg-slate-100 p-1">
            {(["all", "active", "inactive"] as const).map((filter) => (
              <button
                className={cn("rounded-md px-3 py-1.5 text-xs font-semibold capitalize transition", statusFilter === filter ? "bg-white text-slate-950 shadow-sm" : "text-slate-500 hover:text-slate-950")}
                key={filter}
                onClick={() => setStatusFilter(filter)}
                type="button"
              >
                {filter}
              </button>
            ))}
          </div>
        </div>

        {loading ? <LoadingState label="Loading packages" /> : null}
        {!loading && visiblePackages.length === 0 ? <PackageEmptyState onCreate={openCreate} hasPackages={packages.length > 0} /> : null}
        {!loading && visiblePackages.length > 0 ? (
          <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="sticky top-0 bg-slate-50/95 backdrop-blur">
                  <tr>
                    <DenseTh>Package</DenseTh>
                    <DenseTh>Credits</DenseTh>
                    <DenseTh>Price</DenseTh>
                    <DenseTh>Status</DenseTh>
                    <DenseTh>Sort</DenseTh>
                    <DenseTh>Updated</DenseTh>
                    <DenseTh>Actions</DenseTh>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {visiblePackages.map((pkg) => (
                    <tr className="transition-colors hover:bg-slate-50" key={pkg.id}>
                      <DenseTd>
                        <div className="max-w-sm">
                          <p className="font-semibold text-slate-950">{pkg.name}</p>
                          <p className="mt-1 line-clamp-2 text-xs leading-5 text-slate-500">{pkg.description || "No description"}</p>
                        </div>
                      </DenseTd>
                      <DenseTd>
                        <div className="font-mono text-slate-900">{formatCompactUnits(pkg.creditUnits)} + {formatCompactUnits(pkg.bonusCreditUnits)} bonus</div>
                        <p className="mt-1 text-xs text-slate-500">{formatCompactUnits(pkg.creditUnits + pkg.bonusCreditUnits)} total</p>
                      </DenseTd>
                      <DenseTd><span className="font-mono text-slate-900">{formatMoney(pkg.priceMinor, pkg.currency)}</span></DenseTd>
                      <DenseTd><Badge dot tone={pkg.active ? "green" : "yellow"}>{pkg.active ? "Active" : "Inactive"}</Badge></DenseTd>
                      <DenseTd><span className="font-mono text-slate-700">{pkg.sortOrder}</span></DenseTd>
                      <DenseTd>
                        <p>{formatDateTime(pkg.updatedAt)}</p>
                        <p className="mt-1 text-xs text-slate-400">Created {formatDateTime(pkg.createdAt)}</p>
                      </DenseTd>
                      <DenseTd>
                        <div className="flex flex-wrap gap-2">
                          <button className="inline-flex min-h-8 items-center rounded-md border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:border-blue-200 hover:text-blue-700" onClick={() => openEdit(pkg)} type="button">
                            Edit
                          </button>
                          <button
                            className={cn(
                              "inline-flex min-h-8 items-center rounded-md border px-3 text-xs font-semibold disabled:opacity-50",
                              pkg.active
                                ? "border-amber-200 bg-amber-50 text-amber-800 hover:bg-amber-100"
                                : "border-emerald-200 bg-emerald-50 text-emerald-800 hover:bg-emerald-100"
                            )}
                            disabled={activeActionId === pkg.id}
                            onClick={() => toggleActive(pkg)}
                            type="button"
                          >
                            {pkg.active ? "Deactivate" : "Activate"}
                          </button>
                        </div>
                      </DenseTd>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}
      </Card>

      {panel ? (
        <PackagePanel
          fieldErrors={fieldErrors}
          error={formError}
          form={form}
          loading={actionLoading}
          mode={panel.mode}
          onClose={() => setPanel(null)}
          onSubmit={submitPackage}
          setForm={(nextForm) => {
            setForm(nextForm);
            setFieldErrors(validateForm(nextForm).errors);
          }}
        />
      ) : null}
    </AdminShell>
  );
}

function SummaryCard({ label, value, hint, tone = "neutral" }: { label: string; value: string; hint: string; tone?: "neutral" | "green" | "yellow" | "blue" }) {
  const marker = {
    neutral: "bg-slate-300",
    green: "bg-emerald-500",
    yellow: "bg-amber-500",
    blue: "bg-blue-500"
  }[tone];

  return (
    <Card className="p-4">
      <div className="flex items-center justify-between">
        <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-500">{label}</p>
        <span className={cn("size-2 rounded-full", marker)} />
      </div>
      <p className="mt-3 text-2xl font-semibold tracking-[-0.02em] text-slate-950">{value}</p>
      <p className="mt-1 text-xs text-slate-500">{hint}</p>
    </Card>
  );
}

function PackageEmptyState({ hasPackages, onCreate }: { hasPackages: boolean; onCreate: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-slate-50 px-6 py-10 text-center">
      <h3 className="text-base font-semibold text-slate-950">{hasPackages ? "No packages match filters" : "No packages yet"}</h3>
      <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-500">
        {hasPackages ? "Adjust search or status filters to see more packages." : "Create your first prepaid credit bundle for developers."}
      </p>
      {!hasPackages ? <Button className="mt-4" type="button" onClick={onCreate}>Create package</Button> : null}
    </div>
  );
}

function PackagePanel({
  error,
  fieldErrors,
  form,
  loading,
  mode,
  onClose,
  onSubmit,
  setForm
}: {
  error: string | null;
  fieldErrors: FieldErrors;
  form: PackageForm;
  loading: boolean;
  mode: "create" | "edit";
  onClose: () => void;
  onSubmit: (event: FormEvent) => void;
  setForm: (form: PackageForm) => void;
}) {
  const nameInputRef = useRef<HTMLInputElement>(null);
  const validation = validateForm(form);
  const parsedPrice = parseMoneyToMinor(form.price);
  const priceMinor = parsedPrice.ok ? parsedPrice.minor : null;
  const includedCredits = Number(form.creditUnits);
  const bonusCredits = Number(form.bonusCreditUnits);
  const totalCredits = safePositiveNumber(includedCredits) + safePositiveNumber(bonusCredits);
  const currency = form.currency.trim().toUpperCase() || "USD";

  useEffect(() => {
    nameInputRef.current?.focus();
  }, []);

  return (
    <div className="fixed inset-0 z-50 bg-slate-950/40 backdrop-blur-sm" onClick={onClose}>
      <div className="ml-auto flex h-full w-full max-w-[560px] animate-[drawerIn_180ms_ease-out] flex-col bg-white shadow-2xl ring-1 ring-slate-200" onClick={(event) => event.stopPropagation()}>
        <div className="sticky top-0 z-10 border-b border-slate-200 bg-white/95 px-5 py-4 backdrop-blur">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold text-slate-950">{mode === "create" ? "Create package" : "Edit package"}</h2>
              <p className="mt-1 text-sm text-slate-500">Configure prepaid credits, pricing, and visibility.</p>
            </div>
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          </div>
        </div>

        <form className="flex min-h-0 flex-1 flex-col" onSubmit={onSubmit}>
          <div className="flex-1 space-y-5 overflow-y-auto px-5 py-5">
            {error ? <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</p> : null}

          <DrawerSection title="Package details" description="Shown to developers on pricing and billing screens.">
            <FormInput
              error={fieldErrors.name}
              label="Name"
              onChange={(event) => setForm({ ...form, name: event.target.value })}
              ref={nameInputRef}
              required
              value={form.name}
            />
            <FormTextarea
              hint="Shown to developers on pricing and billing screens."
              label="Description"
              onChange={(event) => setForm({ ...form, description: event.target.value })}
              value={form.description}
            />
          </DrawerSection>

          <DrawerSection title="Pricing" description="Price is entered as normal money and stored as integer minor units.">
            <div className="grid gap-3 sm:grid-cols-[1fr_120px]">
              <FormInput
                error={fieldErrors.price}
                hint="Stored as minor units internally."
                inputMode="decimal"
                label="Price"
                onChange={(event) => setForm({ ...form, price: event.target.value })}
                placeholder="10.00"
                required
                value={form.price}
              />
              <FormInput
                error={fieldErrors.currency}
                label="Currency"
                onChange={(event) => setForm({ ...form, currency: event.target.value.toUpperCase() })}
                required
                value={form.currency}
              />
            </div>
            <DerivedLine tone={priceMinor === null ? "red" : "blue"}>
              {priceMinor === null ? "Enter a valid price using up to two decimals." : `Will be stored as ${formatUnits(priceMinor)} minor units.`}
            </DerivedLine>
          </DrawerSection>

          <DrawerSection title="Credits" description="Define the included balance and any bonus credit units.">
            <div className="grid gap-3 sm:grid-cols-2">
              <FormInput
                error={fieldErrors.creditUnits}
                label="Included credits"
                min="1"
                onChange={(event) => setForm({ ...form, creditUnits: event.target.value })}
                required
                type="number"
                value={form.creditUnits}
              />
              <FormInput
                error={fieldErrors.bonusCreditUnits}
                label="Bonus credits"
                min="0"
                onChange={(event) => setForm({ ...form, bonusCreditUnits: event.target.value })}
                required
                type="number"
                value={form.bonusCreditUnits}
              />
            </div>
            <DerivedLine tone="green">
              {safePositiveNumber(bonusCredits) > 0
                ? `Total developer credits: ${formatUnits(totalCredits)} including ${formatUnits(safePositiveNumber(bonusCredits))} bonus.`
                : `Total developer credits: ${formatUnits(totalCredits)}.`}
            </DerivedLine>
          </DrawerSection>

          <DrawerSection title="Visibility" description="Control whether developers can see this package.">
            <button
              className={cn(
                "flex w-full items-start gap-3 rounded-lg border px-3 py-3 text-left transition",
                form.active ? "border-emerald-200 bg-emerald-50" : "border-slate-200 bg-slate-50 hover:bg-slate-100"
              )}
              onClick={() => setForm({ ...form, active: !form.active })}
              type="button"
            >
              <span className={cn("mt-0.5 flex size-5 items-center justify-center rounded-full border", form.active ? "border-emerald-600 bg-emerald-600" : "border-slate-300 bg-white")}>
                {form.active ? <span className="size-2 rounded-full bg-white" /> : null}
              </span>
              <span>
                <span className="block font-medium text-slate-900">Active package</span>
                <span className="mt-1 block text-xs leading-5 text-slate-500">Active packages are visible to developers. Inactive packages are hidden from public pricing.</span>
              </span>
            </button>
          </DrawerSection>

          <DrawerSection title="Ordering" description="Lower numbers appear first.">
            <FormInput
              error={fieldErrors.sortOrder}
              hint="Lower numbers appear first."
              label="Sort order"
              onChange={(event) => setForm({ ...form, sortOrder: event.target.value })}
              required
              type="number"
              value={form.sortOrder}
            />
          </DrawerSection>

          <DeveloperPreview
            active={form.active}
            bonusCredits={safePositiveNumber(bonusCredits)}
            currency={currency}
            includedCredits={safePositiveNumber(includedCredits)}
            name={form.name.trim() || "Untitled package"}
            priceMinor={priceMinor ?? 0}
          />
          </div>

          <div className="sticky bottom-0 z-10 flex gap-3 border-t border-slate-200 bg-white/95 px-5 py-4 backdrop-blur">
            <Button className="flex-1" type="submit" disabled={loading || !validation.valid}>{loading ? "Saving" : mode === "create" ? "Create package" : "Save changes"}</Button>
            <Button className="flex-1" type="button" variant="secondary" onClick={onClose} disabled={loading}>Cancel</Button>
          </div>
        </form>
      </div>
    </div>
  );
}

function DenseTh({ children }: { children: React.ReactNode }) {
  return <th className="whitespace-nowrap px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-[0.1em] text-slate-500">{children}</th>;
}

function DenseTd({ children }: { children: React.ReactNode }) {
  return <td className="whitespace-nowrap px-4 py-3 align-middle text-slate-700">{children}</td>;
}

function DrawerSection({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return (
    <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="mb-4">
        <h3 className="text-sm font-semibold text-slate-950">{title}</h3>
        <p className="mt-1 text-xs leading-5 text-slate-500">{description}</p>
      </div>
      <div className="space-y-3">{children}</div>
    </section>
  );
}

type FormInputProps = InputHTMLAttributes<HTMLInputElement> & {
  error?: string;
  hint?: string;
  label: string;
  ref?: Ref<HTMLInputElement>;
};

function FormInput({ error, hint, label, className, ref, ...props }: FormInputProps) {
  return (
    <label className="block">
      <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">{label}</span>
      <input
        className={cn(
          "mt-2 h-10 w-full rounded-md border bg-white px-3 text-sm text-slate-950 outline-none transition placeholder:text-slate-400 focus:ring-4",
          error ? "border-red-300 ring-red-600/15 focus:border-red-500" : "border-slate-200 ring-blue-600/20 focus:border-blue-600",
          className
        )}
        ref={ref}
        {...props}
      />
      {error ? <span className="mt-1 block text-xs text-red-600">{error}</span> : hint ? <span className="mt-1 block text-xs text-slate-500">{hint}</span> : null}
    </label>
  );
}

type FormTextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & {
  error?: string;
  hint?: string;
  label: string;
};

function FormTextarea({ error, hint, label, className, ...props }: FormTextareaProps) {
  return (
    <label className="block">
      <span className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">{label}</span>
      <textarea
        className={cn(
          "mt-2 min-h-24 w-full rounded-md border bg-white px-3 py-2 text-sm text-slate-950 outline-none transition placeholder:text-slate-400 focus:ring-4",
          error ? "border-red-300 ring-red-600/15 focus:border-red-500" : "border-slate-200 ring-blue-600/20 focus:border-blue-600",
          className
        )}
        {...props}
      />
      {error ? <span className="mt-1 block text-xs text-red-600">{error}</span> : hint ? <span className="mt-1 block text-xs text-slate-500">{hint}</span> : null}
    </label>
  );
}

function DerivedLine({ children, tone }: { children: React.ReactNode; tone: "blue" | "green" | "red" }) {
  const toneClass = {
    blue: "border-blue-100 bg-blue-50 text-blue-700",
    green: "border-emerald-100 bg-emerald-50 text-emerald-700",
    red: "border-red-100 bg-red-50 text-red-700"
  }[tone];

  return <p className={cn("rounded-md border px-3 py-2 text-xs font-medium", toneClass)}>{children}</p>;
}

function DeveloperPreview({
  active,
  bonusCredits,
  currency,
  includedCredits,
  name,
  priceMinor
}: {
  active: boolean;
  bonusCredits: number;
  currency: string;
  includedCredits: number;
  name: string;
  priceMinor: number;
}) {
  return (
    <section className="rounded-xl border border-blue-100 bg-gradient-to-br from-white to-blue-50/60 p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-blue-700">Developer preview</p>
          <h3 className="mt-2 text-lg font-semibold text-slate-950">{name}</h3>
        </div>
        <Badge dot tone={active ? "green" : "yellow"}>{active ? "Active" : "Inactive"}</Badge>
      </div>
      <div className="mt-4 flex items-end justify-between gap-4 border-t border-blue-100 pt-4">
        <div>
          <p className="text-xs text-slate-500">Package price</p>
          <p className="mt-1 text-2xl font-semibold tracking-[-0.02em] text-slate-950">{formatMoney(priceMinor, currency)}</p>
        </div>
        <div className="text-right">
          <p className="text-xs text-slate-500">Credits</p>
          <p className="mt-1 font-mono text-sm font-semibold text-slate-900">{formatUnits(includedCredits)}</p>
          {bonusCredits > 0 ? <p className="mt-1 text-xs text-emerald-700">+ {formatUnits(bonusCredits)} bonus</p> : null}
        </div>
      </div>
      <p className="mt-4 text-xs leading-5 text-slate-500">This is how the package summary will appear to users.</p>
    </section>
  );
}

function validateForm(form: PackageForm): ValidationResult {
  const errors: FieldErrors = {};
  if (!form.name.trim()) {
    errors.name = "Name is required.";
  }
  if (!form.currency.trim()) {
    errors.currency = "Currency is required.";
  }
  if (!isIntegerString(form.creditUnits) || Number(form.creditUnits) <= 0) {
    errors.creditUnits = "Included credits must be greater than 0.";
  }
  if (!isIntegerString(form.bonusCreditUnits) || Number(form.bonusCreditUnits) < 0) {
    errors.bonusCreditUnits = "Bonus credits must be 0 or greater.";
  }
  const parsedPrice = parseMoneyToMinor(form.price);
  if (!parsedPrice.ok) {
    errors.price = parsedPrice.error;
  }
  if (!/^-?\d+$/.test(form.sortOrder.trim())) {
    errors.sortOrder = "Sort order must be a number.";
  }
  return { errors, valid: Object.keys(errors).length === 0 };
}

function parseMoneyToMinor(value: string): ParsedMoney {
  const normalized = value.trim();
  if (!normalized) {
    return { ok: false, error: "Price is required." };
  }
  if (!/^\d+(\.\d{1,2})?$/.test(normalized)) {
    return { ok: false, error: "Use a valid money amount with up to two decimals." };
  }
  const [wholePart, fractionPart = ""] = normalized.split(".");
  const whole = Number(wholePart);
  const fraction = Number(fractionPart.padEnd(2, "0") || "0");
  const minor = whole * 100 + fraction;
  if (minor < 0 || !Number.isSafeInteger(minor)) {
    return { ok: false, error: "Price must be 0 or greater." };
  }
  return { ok: true, minor };
}

function minorToMoney(minor: number) {
  return (minor / 100).toFixed(2);
}

function safePositiveNumber(value: number) {
  if (!Number.isFinite(value) || value < 0) {
    return 0;
  }
  return value;
}

function isIntegerString(value: string) {
  return /^\d+$/.test(value.trim());
}
