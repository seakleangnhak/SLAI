import { Button } from "./Button";
import { Card } from "./Card";

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <Card className="border-red-200 bg-red-50 dark:border-red-900/70 dark:bg-red-950/30">
      <h3 className="text-base font-semibold text-red-900">Unable to load data</h3>
      <p className="mt-2 text-sm leading-6 text-red-700">{message}</p>
      {onRetry ? (
        <Button className="mt-4" variant="secondary" onClick={onRetry}>
          Retry
        </Button>
      ) : null}
    </Card>
  );
}
