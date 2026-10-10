import "server-only";

import { db } from "@/lib/db";

export async function claimInviteFromBatch(batchId: string) {
  for (let index = 0; index < 3; index += 1) {
    const result = await db.$transaction(async (tx) => {
      const candidate = await tx.inviteCode.findFirst({
        where: {
          batchId,
          claimedAt: null,
          usedAt: null,
          batch: {
            isPublic: true,
          },
        },
        orderBy: {
          createdAt: "asc",
        },
        select: {
          batchId: true,
          code: true,
          id: true,
          note: true,
        },
      });

      if (!candidate) {
        return null;
      }

      const updated = await tx.inviteCode.updateMany({
        where: {
          batchId,
          claimedAt: null,
          id: candidate.id,
          usedAt: null,
        },
        data: {
          claimedAt: new Date(),
        },
      });

      if (updated.count === 0) {
        return "retry" as const;
      }

      return {
        batchId: candidate.batchId || batchId,
        code: candidate.code,
        note: candidate.note,
      };
    });

    if (result !== "retry") {
      return result;
    }
  }

  return null;
}
