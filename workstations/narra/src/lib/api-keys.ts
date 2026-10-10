import "server-only";

import { createHash, randomBytes } from "node:crypto";

import type { ApiKey } from "@prisma/client";

const API_KEY_PREFIX = "narra_sk_";
const API_KEY_PREVIEW_LENGTH = 18;

export function createApiKeySecret() {
  return `${API_KEY_PREFIX}${randomBytes(32).toString("base64url")}`;
}

export function hashApiKey(secret: string) {
  return createHash("sha256").update(secret).digest("hex");
}

export function getApiKeyPrefix(secret: string) {
  return secret.slice(0, API_KEY_PREVIEW_LENGTH);
}

export function serializeApiKey(
  apiKey: Pick<ApiKey, "id" | "name" | "keyPrefix" | "createdAt" | "lastUsedAt" | "revokedAt">,
) {
  return {
    createdAt: apiKey.createdAt.toISOString(),
    id: apiKey.id,
    keyPrefix: apiKey.keyPrefix,
    lastUsedAt: apiKey.lastUsedAt?.toISOString() ?? null,
    name: apiKey.name,
    revokedAt: apiKey.revokedAt?.toISOString() ?? null,
    status: apiKey.revokedAt ? "revoked" : "active",
  };
}
