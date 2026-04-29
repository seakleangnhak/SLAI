export const CONFIGURED_API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
export const API_PROXY_BASE = "/api/slai";

export type Role = "USER" | "ADMIN";
export type UserStatus = "ACTIVE" | "SUSPENDED";
export type APIKeyStatus = "ACTIVE" | "SUSPENDED" | "REVOKED";

export type User = {
  id: string;
  email: string;
  role: Role;
  status: UserStatus;
  balancePolicy: string;
  createdAt: string;
  updatedAt: string;
};

export type CreditPackage = {
  id: string;
  name: string;
  description?: string | null;
  creditUnits: number;
  bonusCreditUnits: number;
  priceMinor: number;
  currency: string;
  active: boolean;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
};

export type PackageInput = {
  name: string;
  description?: string | null;
  creditUnits: number;
  bonusCreditUnits: number;
  priceMinor: number;
  currency: string;
  active?: boolean;
  sortOrder: number;
};

export type Balance = {
  userId: string;
  availableUnits: number;
  lifetimePurchasedUnits: number;
  lifetimeUsedUnits: number;
  version: number;
  updatedAt: string;
};

export type LedgerEntry = {
  id: string;
  userId: string;
  type: string;
  source: string;
  deltaUnits: number;
  balanceAfterUnits: number;
  paymentId?: string;
  usageEventId?: string;
  adminId?: string;
  idempotencyKey?: string;
  reason?: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
};

export type PublicAPIKey = {
  id: string;
  name: string;
  key_prefix: string;
  status: APIKeyStatus;
  created_at: string;
  last_used_at?: string | null;
  revoked_at?: string | null;
  omniroute_linked: boolean;
  local_dev_mode?: boolean;
};

export type AdminAPIKey = {
  id: string;
  key_prefix: string;
  status: APIKeyStatus;
  omniroute_key_id?: string | null;
  created_at: string;
  last_used_at?: string | null;
  revoked_at?: string | null;
};

export type CreateAPIKeyResponse = {
  api_key: PublicAPIKey;
  raw_api_key: string;
};

export type UsageEvent = {
  id: string;
  user_id: string;
  api_key_id: string;
  external_source: string;
  external_event_id: string;
  omniroute_key_id?: string | null;
  model?: string | null;
  provider?: string | null;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cost_units: number;
  status: string;
  occurred_at: string;
  raw_json?: Record<string, unknown>;
  created_at: string;
};

export type SyncResult = {
  fetched: number;
  billed: number;
  duplicate: number;
  ignored: number;
  failed: number;
  suspended_keys: number;
};

export type SyncStatus = {
  worker_enabled: boolean;
  last_started_at?: string | null;
  last_finished_at?: string | null;
  last_success_at?: string | null;
  last_error?: string | null;
  last_result?: SyncResult | null;
  next_run_at?: string | null;
  currently_running: boolean;
};

export type ManualTopUpInput = {
  userId: string;
  packageId?: string | null;
  amountMinor: number;
  currency: string;
  creditUnits: number;
  note?: string | null;
  idempotencyKey?: string | null;
};

export type AdjustmentInput = {
  userId: string;
  deltaUnits: number;
  reason: string;
  idempotencyKey?: string | null;
};

export type UsageFilter = {
  user_id?: string;
  api_key_id?: string;
  model?: string;
  provider?: string;
  status?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export class ApiError extends Error {
  status: number;
  code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

type RequestOptions = Omit<RequestInit, "body"> & {
  body?: unknown;
};

function buildUrl(path: string) {
  if (path.startsWith("http://") || path.startsWith("https://")) {
    return path;
  }
  return `${API_PROXY_BASE}${path.startsWith("/") ? path : `/${path}`}`;
}

async function parseResponse(response: Response) {
  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    return response.json();
  }
  const text = await response.text();
  return text ? { message: text } : null;
}

export async function apiFetch<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers);
  let body: BodyInit | undefined;

  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(options.body);
  }

  const response = await fetch(buildUrl(path), {
    ...options,
    body,
    credentials: "include",
    headers
  });

  const payload = await parseResponse(response);

  if (!response.ok) {
    const code = typeof payload?.error === "string" ? payload.error : undefined;
    const message =
      typeof payload?.message === "string"
        ? payload.message
        : code
          ? code.replaceAll("_", " ")
          : `Request failed with ${response.status}`;
    throw new ApiError(message, response.status, code);
  }

  return payload as T;
}

function toQuery(params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") {
      search.set(key, String(value));
    }
  }
  const query = search.toString();
  return query ? `?${query}` : "";
}

export const api = {
  auth: {
    signup: (email: string, password: string) =>
      apiFetch<{ user: User }>("/v1/auth/signup", {
        method: "POST",
        body: { email, password }
      }),
    login: (email: string, password: string) =>
      apiFetch<{ user: User }>("/v1/auth/login", {
        method: "POST",
        body: { email, password }
      }),
    logout: () => apiFetch<{ status: string }>("/v1/auth/logout", { method: "POST" }),
    me: () => apiFetch<{ user: User }>("/v1/me")
  },
  packages: {
    listPublic: () => apiFetch<{ packages: CreditPackage[] }>("/v1/packages")
  },
  balance: {
    get: () => apiFetch<{ balance: Balance }>("/v1/balance")
  },
  ledger: {
    list: (limit = 50) => apiFetch<{ ledger: LedgerEntry[] }>(`/v1/ledger${toQuery({ limit })}`)
  },
  usage: {
    list: (limit = 50, offset = 0) =>
      apiFetch<{ usage: UsageEvent[] }>(`/v1/usage${toQuery({ limit, offset })}`)
  },
  apiKeys: {
    get: () => apiFetch<{ api_key: PublicAPIKey }>("/v1/api-key"),
    create: (name: string) =>
      apiFetch<CreateAPIKeyResponse>("/v1/api-key", {
        method: "POST",
        body: { name }
      }),
    rotate: () => apiFetch<CreateAPIKeyResponse>("/v1/api-key/rotate", { method: "POST" }),
    revoke: () => apiFetch<{ api_key: PublicAPIKey }>("/v1/api-key", { method: "DELETE" })
  },
  admin: {
    packages: {
      list: () => apiFetch<{ packages: CreditPackage[] }>("/v1/admin/packages"),
      create: (input: PackageInput) =>
        apiFetch<{ package: CreditPackage }>("/v1/admin/packages", {
          method: "POST",
          body: input
        }),
      update: (id: string, input: Partial<PackageInput>) =>
        apiFetch<{ package: CreditPackage }>(`/v1/admin/packages/${id}`, {
          method: "PATCH",
          body: input
        })
    },
    payments: {
      manualTopUp: (input: ManualTopUpInput) =>
        apiFetch("/v1/admin/payments/manual-topup", {
          method: "POST",
          body: input
        })
    },
    ledger: {
      adjustment: (input: AdjustmentInput) =>
        apiFetch("/v1/admin/ledger/adjustments", {
          method: "POST",
          body: input
        })
    },
    apiKeys: {
      getForUser: (userId: string) =>
        apiFetch<{ api_key: AdminAPIKey }>(`/v1/admin/users/${userId}/api-key`),
      suspend: (userId: string) =>
        apiFetch<{ api_key: AdminAPIKey }>(`/v1/admin/users/${userId}/api-key/suspend`, {
          method: "POST"
        }),
      resume: (userId: string) =>
        apiFetch<{ api_key: AdminAPIKey }>(`/v1/admin/users/${userId}/api-key/resume`, {
          method: "POST"
        }),
      revoke: (userId: string) =>
        apiFetch<{ api_key: AdminAPIKey }>(`/v1/admin/users/${userId}/api-key/revoke`, {
          method: "POST"
        })
    },
    usage: {
      list: (filter: UsageFilter = {}) =>
        apiFetch<{ usage: UsageEvent[] }>(`/v1/admin/usage${toQuery(filter)}`),
      sync: () => apiFetch<{ sync: SyncResult }>("/v1/admin/usage/sync", { method: "POST" }),
      syncStatus: () => apiFetch<{ sync_status: SyncStatus }>("/v1/admin/usage/sync-status")
    }
  }
};

export function isNotFound(error: unknown) {
  return error instanceof ApiError && error.status === 404;
}

export function readableError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return "Something went wrong.";
}
