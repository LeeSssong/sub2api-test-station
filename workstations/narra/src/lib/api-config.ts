import "server-only";

import { GenerationClientSource } from "@prisma/client";

import { db } from "@/lib/db";
import { ApiRateLimitError } from "@/lib/api-errors";

const API_CONFIG_SCOPE = "default";

export async function getApiConfig() {
  return db.apiConfig.upsert({
    where: { scope: API_CONFIG_SCOPE },
    update: {},
    create: { scope: API_CONFIG_SCOPE },
  });
}

export function serializeApiConfig(config: Awaited<ReturnType<typeof getApiConfig>>) {
  return {
    isEnabled: config.isEnabled,
    requestsPerDay: config.requestsPerDay,
    requestsPerMinute: config.requestsPerMinute,
    updatedAt: config.updatedAt.toISOString(),
  };
}

export async function updateApiConfig(input: {
  isEnabled?: boolean;
  requestsPerDay?: number;
  requestsPerMinute?: number;
}) {
  await getApiConfig();
  return db.apiConfig.update({
    where: { scope: API_CONFIG_SCOPE },
    data: input,
  });
}

export async function assertApiRateLimit(apiKeyId: string) {
  const config = await getApiConfig();
  if (!config.isEnabled) {
    throw new ApiRateLimitError("站点 API 当前已关闭");
  }

  const now = new Date();
  const minuteStart = new Date(now.getTime() - 60 * 1000);
  const dayStart = new Date(now);
  dayStart.setHours(0, 0, 0, 0);

  const [minuteCount, dayCount] = await Promise.all([
    db.generationJob.count({
      where: {
        apiKeyId,
        clientSource: GenerationClientSource.API,
        createdAt: { gte: minuteStart },
      },
    }),
    db.generationJob.count({
      where: {
        apiKeyId,
        clientSource: GenerationClientSource.API,
        createdAt: { gte: dayStart },
      },
    }),
  ]);

  if (minuteCount >= config.requestsPerMinute) {
    throw new ApiRateLimitError(
      `请求过于频繁：每个 API Key 每分钟最多 ${config.requestsPerMinute} 次`,
    );
  }

  if (dayCount >= config.requestsPerDay) {
    throw new ApiRateLimitError(
      `今日 API 调用已达上限：每个 API Key 每天最多 ${config.requestsPerDay} 次`,
    );
  }
}
