import Link from "next/link";

type AppShellProps = {
  children: React.ReactNode;
  section: "dashboard" | "admin";
};

const navItems = {
  dashboard: [
    { href: "/dashboard", label: "Overview" },
    { href: "/dashboard", label: "API key" },
    { href: "/dashboard", label: "Usage" },
    { href: "/dashboard", label: "Credits" }
  ],
  admin: [
    { href: "/admin", label: "Overview" },
    { href: "/admin", label: "Users" },
    { href: "/admin", label: "Top-ups" },
    { href: "/admin", label: "Audit" }
  ]
};

export function AppShell({ children, section }: AppShellProps) {
  return (
    <div className="min-h-screen bg-[#f7f8fb] text-slate-950">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <Link href="/" className="flex items-center gap-3">
            <span className="grid size-8 place-items-center rounded-md bg-slate-950 text-sm font-semibold text-white">S</span>
            <span className="text-sm font-semibold tracking-[0.18em] text-slate-900">SLAI</span>
          </Link>
          <nav className="hidden items-center gap-1 md:flex">
            {navItems[section].map((item) => (
              <Link
                href={item.href}
                key={item.label}
                className="rounded-md px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-950"
              >
                {item.label}
              </Link>
            ))}
          </nav>
          <Link
            href="/login"
            className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50"
          >
            Account
          </Link>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">{children}</main>
    </div>
  );
}
