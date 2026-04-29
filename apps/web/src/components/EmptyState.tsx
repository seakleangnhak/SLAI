import { Card } from "./Card";

export function EmptyState({ title, message }: { title: string; message: string }) {
  return (
    <Card className="text-center">
      <h3 className="text-base font-semibold text-slate-950">{title}</h3>
      <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-500">{message}</p>
    </Card>
  );
}
