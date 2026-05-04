import { Card } from "./Card";

export function LoadingState({ label = "Loading" }: { label?: string }) {
  return (
    <Card>
      <div className="flex items-center gap-3 text-sm font-medium text-slate-600 dark:text-slate-300">
        <span className="size-2 rounded-full bg-cyan-600" />
        {label}
      </div>
    </Card>
  );
}
