import { createHmac, timingSafeEqual } from "node:crypto";

import type { UserRole } from "@/lib/types";

const FOURTEEN_DAYS_IN_SECONDS = 60 * 60 * 6;

type SessionPayload = {
  userId: string;
  role: UserRole;
};

type StoredSessionPayload = SessionPayload & {
  exp: number;
  iat: number;
};

function toBase64Url(value: string) {
  return Buffer.from(value).toString("base64url");
}

function fromBase64Url(value: string) {
  return Buffer.from(value, "base64url").toString("utf8");
}

function signSegment(value: string, secret: string) {
  return createHmac("sha256", secret).update(value).digest("base64url");
}

export async function createSessionToken(
  payload: SessionPayload,
  secret: string,
) {
  const header = toBase64Url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const issuedAt = Math.floor(Date.now() / 1000);
  const body = toBase64Url(
    JSON.stringify({
      ...payload,
      exp: issuedAt + FOURTEEN_DAYS_IN_SECONDS,
      iat: issuedAt,
    } satisfies StoredSessionPayload),
  );
  const signature = signSegment(`${header}.${body}`, secret);

  return `${header}.${body}.${signature}`;
}

export async function verifySessionToken(token: string, secret: string) {
  const [header, body, signature] = token.split(".");

  if (!header || !body || !signature) {
    throw new Error("会话令牌格式错误");
  }

  const expectedSignature = signSegment(`${header}.${body}`, secret);

  if (
    !timingSafeEqual(
      Buffer.from(signature, "utf8"),
      Buffer.from(expectedSignature, "utf8"),
    )
  ) {
    throw new Error("会话令牌签名无效");
  }

  const payload = JSON.parse(fromBase64Url(body)) as StoredSessionPayload;
  const now = Math.floor(Date.now() / 1000);

  if (payload.exp <= now) {
    throw new Error("会话令牌已过期");
  }

  return payload;
}
