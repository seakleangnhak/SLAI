export const CONFIGURED_API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
export const API_PROXY_BASE = "/api/slai";
export const CREDIT_UNIT_SCALE = 1_000_000;

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

export type Payment = {
  id: string;
  userId: string;
  packageId?: string | null;
  packageName?: string | null;
  provider: string;
  providerRef?: string | null;
  externalPaymentId?: string | null;
  checkoutReference?: string | null;
  qrPayload?: string | null;
  qrImageDataUri?: string | null;
  qrMd5?: string | null;
  expiresAt?: string | null;
  callbackReceivedAt?: string | null;
  providerStatus?: string | null;
  providerTransactionId?: string | null;
  providerApv?: string | null;
  amountMinor: number;
  currency: string;
  creditUnits: number;
  status: string;
  adminId?: string | null;
  note?: string | null;
  proofUploaded?: boolean;
  rejectionReason?: string | null;
  adminPaymentReference?: string | null;
  reviewedByAdminId?: string | null;
  reviewedAt?: string | null;
  createdAt: string;
  updatedAt?: string;
  paidAt?: string | null;
};

export type CheckoutInfo = {
  provider: string;
  display_name: string;
  account_name?: string | null;
  account_id?: string | null;
  khqr_image_url?: string | null;
  qr_payload?: string | null;
  qr_image_data_uri?: string | null;
  reference?: string | null;
  expires_at?: string | null;
  instructions?: string | null;
};

export type CheckoutResponse = {
  payment: Payment;
  checkout: CheckoutInfo;
};

export type PaymentSettings = {
  provider: string;
  enabled: boolean;
  display_name: string;
  account_name?: string | null;
  account_id?: string | null;
  khqr_image_url?: string | null;
  instructions?: string | null;
  updated_at: string;
};

export type PaymentSettingsInput = {
  enabled: boolean;
  display_name: string;
  account_name?: string | null;
  account_id?: string | null;
  instructions?: string | null;
};

export type PaymentProviderStatus = {
  provider: string;
  mode: string;
  enabled: boolean;
  base_url_configured: boolean;
  api_key_configured: boolean;
  callback_base_url_configured: boolean;
  callback_secret_configured: boolean;
  merchant_prefix?: string;
  default_expiry_seconds: number;
};

export type AdminPaymentItem = {
  id: string;
  user_id: string;
  user_email: string;
  package_id?: string | null;
  package_name?: string | null;
  provider: string;
  provider_ref?: string | null;
  external_payment_id?: string | null;
  checkout_reference?: string | null;
  expires_at?: string | null;
  provider_status?: string | null;
  provider_transaction_id?: string | null;
  provider_apv?: string | null;
  amount_minor: number;
  currency: string;
  credit_units: number;
  status: string;
  admin_id?: string | null;
  note?: string | null;
  created_at: string;
  updated_at: string;
  paid_at?: string | null;
  proof_uploaded: boolean;
  proof_file_sha256?: string | null;
  duplicate_proof_count: number;
  reviewed_by_admin_id?: string | null;
  reviewed_by_admin_email?: string | null;
  reviewed_at?: string | null;
  admin_payment_reference?: string | null;
  rejection_reason?: string | null;
  proof?: PaymentProof | null;
};

export type PaymentProof = {
  id: string;
  payment_id: string;
  user_id: string;
  file_name: string;
  file_mime: string;
  file_size: number;
  file_sha256: string;
  user_transaction_ref?: string | null;
  user_note?: string | null;
  uploaded_at: string;
};

export type AdminPaymentListResponse = {
  items: AdminPaymentItem[];
  limit: number;
  offset: number;
  total: number;
};

export type AdminPaymentFilter = {
  status?: string;
  user_id?: string;
  provider?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
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
  omniroute_enabled?: boolean;
  sync_mode?: string;
  worker_interval_seconds?: number;
  batch_limit?: number;
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

export type UserUsageFilter = Omit<UsageFilter, "user_id" | "api_key_id">;

export type AdminUserFilter = {
  q?: string;
  status?: UserStatus | "";
  role?: Role | "";
  limit?: number;
  offset?: number;
};

export type AdminUserListItem = {
  id: string;
  email: string;
  role: Role;
  status: UserStatus;
  balance_units: number;
  lifetime_purchased_units: number;
  lifetime_used_units: number;
  api_key_status?: APIKeyStatus | null;
  api_key_prefix?: string | null;
  created_at: string;
  updated_at: string;
};

export type AdminUserListResponse = {
  items: AdminUserListItem[];
  limit: number;
  offset: number;
  total: number;
};

export type AdminBalance = {
  available_units: number;
  lifetime_purchased_units: number;
  lifetime_used_units: number;
  version: number;
  updated_at: string;
};

export type AdminUsageEvent = {
  id: string;
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
  created_at: string;
};

export type AdminLedgerEntry = {
  id: string;
  type: string;
  source: string;
  delta_units: number;
  balance_after_units: number;
  payment_id?: string | null;
  usage_event_id?: string | null;
  admin_id?: string | null;
  idempotency_key?: string | null;
  reason?: string | null;
  created_at: string;
};

export type AdminPayment = {
  id: string;
  package_id?: string | null;
  provider: string;
  provider_ref?: string | null;
  amount_minor: number;
  currency: string;
  credit_units: number;
  status: string;
  admin_id?: string | null;
  note?: string | null;
  created_at: string;
  paid_at?: string | null;
};

export type AdminUserDetail = {
  id: string;
  email: string;
  role: Role;
  status: UserStatus;
  balance: AdminBalance;
  api_key?: AdminAPIKey | null;
  recent_usage: AdminUsageEvent[];
  recent_ledger: AdminLedgerEntry[];
  recent_payments: AdminPayment[];
  created_at: string;
  updated_at: string;
};

export type AuditLogFilter = {
  admin_id?: string;
  action?: string;
  target_type?: string;
  target_id?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export type AuditLogItem = {
  id: string;
  admin_id: string;
  admin_email: string;
  action: string;
  target_type?: string | null;
  target_id?: string | null;
  metadata: Record<string, unknown>;
  created_at: string;
};

export type AuditLogListResponse = {
  items: AuditLogItem[];
  limit: number;
  offset: number;
  total: number;
};

export type AdminDashboard = {
  users: {
    total: number;
    active: number;
    suspended: number;
  };
  credits: {
    total_available_units: number;
    total_purchased_units: number;
    total_used_units: number;
  };
  revenue: {
    total_paid_minor: number;
    currency: string;
  };
  api_keys: {
    active: number;
    suspended: number;
    revoked: number;
  };
  usage: {
    total_events: number;
    billed_events: number;
    failed_events: number;
    ignored_events: number;
    total_input_tokens: number;
    total_output_tokens: number;
    total_tokens: number;
    total_cost_units: number;
  };
  recent_payments: AdminDashboardPayment[];
  recent_usage: AdminDashboardUsage[];
  recent_audit_logs: AdminDashboardAuditLog[];
  sync_status: {
    worker_enabled: boolean;
    currently_running: boolean;
    last_success_at?: string | null;
    last_error?: string | null;
  };
};

export type AdminDashboardPayment = {
  id: string;
  user_id: string;
  user_email: string;
  amount_minor: number;
  currency: string;
  credit_units: number;
  status: string;
  created_at: string;
};

export type AdminDashboardUsage = {
  id: string;
  user_id: string;
  user_email: string;
  model?: string | null;
  provider?: string | null;
  total_tokens: number;
  cost_units: number;
  status: string;
  occurred_at: string;
  created_at: string;
};

export type AdminDashboardAuditLog = {
  id: string;
  admin_id: string;
  admin_email: string;
  action: string;
  target_type?: string | null;
  target_id?: string | null;
  created_at: string;
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

export function apiAssetUrl(path: string) {
  return buildUrl(path);
}

async function parseResponse(response: Response) {
  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    return response.json();
  }
  const text = await response.text();
  return text ? { message: text } : null;
}

export async function apiUpload<T>(path: string, formData: FormData, options: Omit<RequestInit, "body"> = {}): Promise<T> {
  const response = await fetch(buildUrl(path), {
    ...options,
    method: options.method ?? "POST",
    body: formData,
    credentials: "include"
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

function toStoredCredits(value: number) {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.round(value * CREDIT_UNIT_SCALE);
}

function scalePackageInput(input: PackageInput): PackageInput {
  return {
    ...input,
    creditUnits: toStoredCredits(input.creditUnits),
    bonusCreditUnits: toStoredCredits(input.bonusCreditUnits)
  };
}

function scalePartialPackageInput(input: Partial<PackageInput>): Partial<PackageInput> {
  return {
    ...input,
    creditUnits: input.creditUnits === undefined ? undefined : toStoredCredits(input.creditUnits),
    bonusCreditUnits: input.bonusCreditUnits === undefined ? undefined : toStoredCredits(input.bonusCreditUnits)
  };
}

function scaleManualTopUpInput(input: ManualTopUpInput): ManualTopUpInput {
  return {
    ...input,
    creditUnits: toStoredCredits(input.creditUnits)
  };
}

function scaleAdjustmentInput(input: AdjustmentInput): AdjustmentInput {
  return {
    ...input,
    deltaUnits: toStoredCredits(input.deltaUnits)
  };
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
  payments: {
    list: (limit = 50, offset = 0) => apiFetch<{ payments: Payment[] }>(`/v1/payments${toQuery({ limit, offset })}`),
    get: (id: string) => apiFetch<{ payment: Payment }>(`/v1/payments/${id}`),
    refresh: (id: string) => apiFetch<{ payment: Payment }>(`/v1/payments/${id}/refresh`, { method: "POST" }),
    proofUrl: (id: string) => apiAssetUrl(`/v1/payments/${id}/proof`),
    uploadProof: (id: string, formData: FormData) =>
      apiUpload<{ payment: Payment }>(`/v1/payments/${id}/proof`, formData)
  },
  checkout: {
    package: (packageId: string) =>
      apiFetch<CheckoutResponse>(`/v1/checkout/package/${packageId}`, { method: "POST" }),
    bakongSettings: () => apiFetch<{ settings: PaymentSettings }>("/v1/payment-settings/bakong-khqr")
  },
  usage: {
    list: (filterOrLimit: UserUsageFilter | number = 50, offset = 0) => {
      const filter = typeof filterOrLimit === "number" ? { limit: filterOrLimit, offset } : filterOrLimit;
      return apiFetch<{ usage: UsageEvent[] }>(`/v1/usage${toQuery(filter)}`);
    }
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
    dashboard: {
      get: () => apiFetch<AdminDashboard>("/v1/admin/dashboard")
    },
    users: {
      list: (filter: AdminUserFilter = {}) =>
        apiFetch<AdminUserListResponse>(`/v1/admin/users${toQuery(filter)}`),
      get: (userId: string) => apiFetch<AdminUserDetail>(`/v1/admin/users/${userId}`),
      updateStatus: (userId: string, status: UserStatus) =>
        apiFetch<AdminUserDetail>(`/v1/admin/users/${userId}/status`, {
          method: "PATCH",
          body: { status }
        })
    },
    auditLogs: {
      list: (filter: AuditLogFilter = {}) =>
        apiFetch<AuditLogListResponse>(`/v1/admin/audit-logs${toQuery(filter)}`)
    },
    packages: {
      list: () => apiFetch<{ packages: CreditPackage[] }>("/v1/admin/packages"),
      create: (input: PackageInput) =>
        apiFetch<{ package: CreditPackage }>("/v1/admin/packages", {
          method: "POST",
          body: scalePackageInput(input)
        }),
      update: (id: string, input: Partial<PackageInput>) =>
        apiFetch<{ package: CreditPackage }>(`/v1/admin/packages/${id}`, {
          method: "PATCH",
          body: scalePartialPackageInput(input)
        })
    },
    payments: {
      list: (filter: AdminPaymentFilter = {}) =>
        apiFetch<AdminPaymentListResponse>(`/v1/admin/payments${toQuery(filter)}`),
      get: (id: string) => apiFetch<{ payment: AdminPaymentItem }>(`/v1/admin/payments/${id}`),
      proofUrl: (id: string) => apiAssetUrl(`/v1/admin/payments/${id}/proof`),
      approve: (id: string, paymentReference: string, note?: string | null) =>
        apiFetch(`/v1/admin/payments/${id}/approve`, {
          method: "POST",
          body: { payment_reference: paymentReference, note }
        }),
      reject: (id: string, reason: string) =>
        apiFetch<{ payment: Payment }>(`/v1/admin/payments/${id}/reject`, {
          method: "POST",
          body: { reason }
        }),
      manualTopUp: (input: ManualTopUpInput) =>
        apiFetch("/v1/admin/payments/manual-topup", {
          method: "POST",
          body: scaleManualTopUpInput(input)
        })
    },
    paymentSettings: {
      bakong: {
        get: () => apiFetch<{ settings: PaymentSettings }>("/v1/admin/payment-settings/bakong-khqr"),
        update: (input: PaymentSettingsInput) =>
          apiFetch<{ settings: PaymentSettings }>("/v1/admin/payment-settings/bakong-khqr", {
            method: "PATCH",
            body: input
          }),
        uploadImage: (formData: FormData) =>
          apiUpload<{ settings: PaymentSettings }>("/v1/admin/payment-settings/bakong-khqr/khqr-image", formData),
        providerStatus: () =>
          apiFetch<{ provider_status: PaymentProviderStatus }>("/v1/admin/payment-settings/bakong-khqr/provider-status"),
        imageUrl: () => apiAssetUrl("/v1/payment-settings/bakong-khqr/khqr-image")
      }
    },
    ledger: {
      adjustment: (input: AdjustmentInput) =>
        apiFetch("/v1/admin/ledger/adjustments", {
          method: "POST",
          body: scaleAdjustmentInput(input)
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
