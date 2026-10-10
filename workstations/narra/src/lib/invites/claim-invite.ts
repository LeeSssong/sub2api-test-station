type ClaimedInvite = {
  batchId: string;
  code: string;
  note: string | null;
};

type CreateInviteClaimServiceInput = {
  assignInvite: (batchId: string) => Promise<ClaimedInvite | null>;
};

export function createInviteClaimService(input: CreateInviteClaimServiceInput) {
  return {
    async claim(batchId: string) {
      const invite = await input.assignInvite(batchId);

      if (!invite) {
        throw new Error("当前批次邀请码已领完");
      }

      return {
        ...invite,
        registerUrl: `/register?inviteCode=${invite.code}`,
      };
    },
  };
}
