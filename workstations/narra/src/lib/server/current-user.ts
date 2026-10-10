import "server-only";

import { cookies } from "next/headers";
import { siteProfile } from "@/lib/workstation/site";
import { decryptProviderSecret } from "@/lib/providers/provider-secret";
import { cache } from "react";
import { Role } from "@prisma/client";

import { db } from "@/lib/db";
import { readSession } from "@/lib/auth/session";

export const getCurrentUserRecord = cache(async () => {
  const session = await readSession();
  if (!session) {
    return null;
  }

  const identity = (await cookies()).get("site_identity")?.value;
  if (!identity) return null;
  let profile;
  try { profile = await siteProfile(await decryptProviderSecret(identity, process.env.AUTH_SECRET!)); } catch { return null; }
  const user = await db.user.findUnique({
    where: { id: session.userId },
    select: {
      createdAt: true,
      avatarUrl: true,
      bannedAt: true,
      credits: true,
      email: true,
      id: true,
      nickname: true,
      oauthProvider: true,
      role: true,
    },
  });

  // 封禁用户按未登录处理：等效登出，无需等待 token 过期
  if (!user || user.bannedAt || Date.now()-user.createdAt.getTime()>=21600000 || !user.id.startsWith(`site_${profile.id}_`)) {
    return null;
  }

  return {...user,role:profile.role === "admin" ? Role.ADMIN : Role.USER};
});

export const getCurrentSession = cache(async () => {
  return readSession();
});

export async function requireCurrentSession() {
  const session = await getCurrentSession();
  if (!session) {
    throw new Error("请先登录");
  }
  return session;
}

export async function requireCurrentUserRecord() {
  const user = await getCurrentUserRecord();
  if (!user) {
    throw new Error("请先登录");
  }

  return user;
}

export async function requireAdminRecord() {
  const user = await requireCurrentUserRecord();
  if (user.role !== Role.ADMIN) {
    throw new Error("没有管理员权限");
  }

  return user;
}
