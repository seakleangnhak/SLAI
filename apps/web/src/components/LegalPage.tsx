import { AppShell } from "@/components/AppShell";

type LegalPageProps = {
  children: React.ReactNode;
  description: string;
  eyebrow: string;
  title: string;
  updated: string;
};

type LegalSectionProps = {
  children: React.ReactNode;
  title: string;
};

export function LegalPage({
  children,
  description,
  eyebrow,
  title,
  updated,
}: LegalPageProps) {
  return (
    <AppShell section="public">
      <div className="mx-auto max-w-5xl">
        <section className="overflow-hidden rounded-[2rem] border border-slate-200 bg-white p-6 shadow-sm sm:p-8 lg:p-10">
          <div className="max-w-3xl">
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-blue-700">
              {eyebrow}
            </p>
            <h1 className="mt-4 text-4xl font-semibold tracking-normal text-slate-950 sm:text-5xl">
              {title}
            </h1>
            <p className="mt-4 text-base leading-7 text-slate-600">
              {description}
            </p>
            <p className="mt-5 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900">
              Operational draft. Review with qualified counsel before production
              launch. Last updated: {updated}.
            </p>
          </div>
        </section>
        <article className="mt-8 space-y-4">{children}</article>
      </div>
    </AppShell>
  );
}

export function LegalSection({ children, title }: LegalSectionProps) {
  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
      <h2 className="text-xl font-semibold text-slate-950">{title}</h2>
      <div className="mt-4 space-y-3 text-sm leading-7 text-slate-600">
        {children}
      </div>
    </section>
  );
}

export function LegalList({ items }: { items: string[] }) {
  return (
    <ul className="list-disc space-y-2 pl-5">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  );
}
