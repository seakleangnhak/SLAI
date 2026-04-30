export const CREDIT_UNIT_SCALE = 1_000_000;

export function formatUnits(value: number | null | undefined) {
  return new Intl.NumberFormat("en-US").format(value ?? 0);
}

export function formatCredits(value: number | null | undefined, options: Intl.NumberFormatOptions = {}) {
  const credits = (value ?? 0) / CREDIT_UNIT_SCALE;
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 6,
    ...options
  }).format(credits);
}

export function formatCompactCredits(value: number | null | undefined) {
  const credits = (value ?? 0) / CREDIT_UNIT_SCALE;
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 3,
    notation: "compact"
  }).format(credits);
}

export function formatCreditInput(value: number | null | undefined) {
  const credits = (value ?? 0) / CREDIT_UNIT_SCALE;
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 6,
    useGrouping: false
  }).format(credits);
}

export function formatCompactUnits(value: number | null | undefined) {
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 1,
    notation: "compact"
  }).format(value ?? 0);
}

export function formatMoney(minor: number, currency = "USD") {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency
  }).format(minor / 100);
}

export function formatDateTime(value?: string | null) {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(new Date(value));
}

export function formatDelta(value: number) {
  const formatted = formatCredits(Math.abs(value));
  return value < 0 ? `-${formatted}` : `+${formatted}`;
}

export function formatLedgerReason(entry: { type?: string; reason?: string | null; source?: string | null }) {
  const raw = entry.reason?.trim() || entry.source?.trim() || "";
  if (!raw) {
    return "-";
  }
  if (/omniroute/i.test(raw)) {
    if (entry.type === "usage_debit" || /usage/i.test(raw)) {
      return "API usage billing";
    }
    return raw.replace(/omniroute/gi, "SLAI");
  }
  return raw;
}

export function truncateId(value?: string | null, start = 8, end = 4) {
  if (!value) {
    return "-";
  }
  if (value.length <= start + end + 3) {
    return value;
  }
  return `${value.slice(0, start)}...${value.slice(-end)}`;
}
