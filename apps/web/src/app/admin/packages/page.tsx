"use client";

import { FormEvent, useEffect, useState } from "react";

import { AdminShell } from "@/components/AdminShell";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { Input, Textarea } from "@/components/Input";
import { LoadingState } from "@/components/LoadingState";
import { Table, Td, Th } from "@/components/Table";
import { api, readableError, type CreditPackage, type PackageInput } from "@/lib/api";
import { formatMoney, formatUnits } from "@/lib/format";

type PackageForm = {
  name: string;
  description: string;
  creditUnits: string;
  bonusCreditUnits: string;
  priceMinor: string;
  currency: string;
  active: boolean;
  sortOrder: string;
};

const emptyForm: PackageForm = {
  name: "",
  description: "",
  creditUnits: "1000",
  bonusCreditUnits: "0",
  priceMinor: "1000",
  currency: "USD",
  active: true,
  sortOrder: "0"
};

function formToInput(form: PackageForm): PackageInput {
  return {
    name: form.name,
    description: form.description || null,
    creditUnits: Number(form.creditUnits),
    bonusCreditUnits: Number(form.bonusCreditUnits),
    priceMinor: Number(form.priceMinor),
    currency: form.currency,
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
    priceMinor: String(pkg.priceMinor),
    currency: pkg.currency,
    active: pkg.active,
    sortOrder: String(pkg.sortOrder)
  };
}

export default function AdminPackagesPage() {
  const [packages, setPackages] = useState<CreditPackage[]>([]);
  const [createForm, setCreateForm] = useState<PackageForm>(emptyForm);
  const [editId, setEditId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<PackageForm>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
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

  async function create(event: FormEvent) {
    event.preventDefault();
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      await api.admin.packages.create(formToInput(createForm));
      setCreateForm(emptyForm);
      setNotice("Package created.");
      load();
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  function startEdit(pkg: CreditPackage) {
    setEditId(pkg.id);
    setEditForm(packageToForm(pkg));
  }

  async function saveEdit(event: FormEvent) {
    event.preventDefault();
    if (!editId) {
      return;
    }
    setActionLoading(true);
    setError(null);
    setNotice(null);
    try {
      await api.admin.packages.update(editId, formToInput(editForm));
      setEditId(null);
      setNotice("Package updated.");
      load();
    } catch (err) {
      setError(readableError(err));
    } finally {
      setActionLoading(false);
    }
  }

  return (
    <AdminShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin packages</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Credit packages</h1>
      </div>

      {error ? <div className="mt-8"><ErrorState message={error} onRetry={load} /></div> : null}
      {notice ? <p className="mt-8 rounded-md bg-cyan-50 px-3 py-2 text-sm text-cyan-800">{notice}</p> : null}

      <section className="mt-8 grid gap-6 xl:grid-cols-[0.9fr_1.1fr]">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>Create package</CardTitle>
              <CardDescription>Public packages are visible to users, but payment remains manual.</CardDescription>
            </div>
          </CardHeader>
          <PackageEditor form={createForm} setForm={setCreateForm} onSubmit={create} submitLabel="Create package" loading={actionLoading} />
        </Card>

        <div>
          {loading ? <LoadingState label="Loading packages" /> : null}
          {!loading && packages.length === 0 ? <EmptyState title="No packages" message="Create the first prepaid credit package." /> : null}
          {!loading && packages.length > 0 ? (
            <Table>
              <thead className="bg-slate-50"><tr><Th>Name</Th><Th>Credits</Th><Th>Price</Th><Th>Status</Th><Th>Sort</Th><Th>Action</Th></tr></thead>
              <tbody className="divide-y divide-slate-100">
                {packages.map((pkg) => (
                  <tr key={pkg.id}>
                    <Td className="font-medium text-slate-950">{pkg.name}</Td>
                    <Td>{formatUnits(pkg.creditUnits + pkg.bonusCreditUnits)}</Td>
                    <Td>{formatMoney(pkg.priceMinor, pkg.currency)}</Td>
                    <Td><Badge tone={pkg.active ? "green" : "neutral"}>{pkg.active ? "Active" : "Inactive"}</Badge></Td>
                    <Td>{pkg.sortOrder}</Td>
                    <Td><Button type="button" variant="secondary" onClick={() => startEdit(pkg)}>Edit</Button></Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          ) : null}
        </div>
      </section>

      {editId ? (
        <Card className="mt-8">
          <CardHeader>
            <div>
              <CardTitle>Edit package</CardTitle>
              <CardDescription>Changes apply immediately to package listings.</CardDescription>
            </div>
            <Button type="button" variant="ghost" onClick={() => setEditId(null)}>Cancel</Button>
          </CardHeader>
          <PackageEditor form={editForm} setForm={setEditForm} onSubmit={saveEdit} submitLabel="Save package" loading={actionLoading} />
        </Card>
      ) : null}
    </AdminShell>
  );
}

function PackageEditor({
  form,
  setForm,
  onSubmit,
  submitLabel,
  loading
}: {
  form: PackageForm;
  setForm: (form: PackageForm) => void;
  onSubmit: (event: FormEvent) => void;
  submitLabel: string;
  loading: boolean;
}) {
  return (
    <form className="grid gap-4 md:grid-cols-2" onSubmit={onSubmit}>
      <Input label="Name" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
      <Input label="Currency" value={form.currency} onChange={(event) => setForm({ ...form, currency: event.target.value.toUpperCase() })} required />
      <div className="md:col-span-2">
        <Textarea label="Description" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
      </div>
      <Input label="Credit units" type="number" min="0" value={form.creditUnits} onChange={(event) => setForm({ ...form, creditUnits: event.target.value })} required />
      <Input label="Bonus units" type="number" min="0" value={form.bonusCreditUnits} onChange={(event) => setForm({ ...form, bonusCreditUnits: event.target.value })} required />
      <Input label="Price minor" type="number" min="0" value={form.priceMinor} onChange={(event) => setForm({ ...form, priceMinor: event.target.value })} required />
      <Input label="Sort order" type="number" value={form.sortOrder} onChange={(event) => setForm({ ...form, sortOrder: event.target.value })} required />
      <label className="flex items-center gap-2 text-sm font-medium text-slate-700">
        <input className="size-4 rounded border-slate-300" type="checkbox" checked={form.active} onChange={(event) => setForm({ ...form, active: event.target.checked })} />
        Active
      </label>
      <div className="md:col-span-2"><Button type="submit" disabled={loading}>{loading ? "Saving" : submitLabel}</Button></div>
    </form>
  );
}
