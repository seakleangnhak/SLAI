import type { User } from "./api";

export function isAdmin(user: User | null | undefined) {
  return user?.role === "ADMIN";
}

export function userInitial(user: User | null | undefined) {
  return user?.email?.slice(0, 1).toUpperCase() ?? "S";
}
