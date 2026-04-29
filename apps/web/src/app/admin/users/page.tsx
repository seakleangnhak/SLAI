"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

import { AdminShell } from "@/components/AdminShell";
import { Button } from "@/components/Button";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/Card";
import { Input } from "@/components/Input";

export default function AdminUsersPage() {
  const router = useRouter();
  const [userId, setUserId] = useState("");

  function submit(event: FormEvent) {
    event.preventDefault();
    if (userId.trim()) {
      router.push(`/admin/users/${encodeURIComponent(userId.trim())}`);
    }
  }

  return (
    <AdminShell>
      <div>
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-700">Admin users</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-normal text-slate-950">Users</h1>
      </div>

      <Card className="mt-8 border-amber-200 bg-amber-50">
        <CardTitle>User list endpoint missing</CardTitle>
        <CardDescription>
          The backend does not currently expose admin user list or admin user profile endpoints. This page does not fake a user list.
        </CardDescription>
      </Card>

      <Card className="mt-6 max-w-2xl">
        <CardHeader>
          <div>
            <CardTitle>Open user by ID</CardTitle>
            <CardDescription>Use a known user ID to manage API key status, manual top-up, or credit adjustment.</CardDescription>
          </div>
        </CardHeader>
        <form className="flex flex-col gap-3 sm:flex-row" onSubmit={submit}>
          <div className="flex-1"><Input label="User ID" value={userId} onChange={(event) => setUserId(event.target.value)} required /></div>
          <Button className="self-end" type="submit">Open user</Button>
        </form>
      </Card>
    </AdminShell>
  );
}
