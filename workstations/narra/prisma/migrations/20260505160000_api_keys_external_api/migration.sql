CREATE TYPE "GenerationClientSource" AS ENUM ('WEB', 'API');

CREATE TABLE "ApiKey" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "keyHash" TEXT NOT NULL,
    "keyPrefix" TEXT NOT NULL,
    "revokedAt" TIMESTAMP(3),
    "lastUsedAt" TIMESTAMP(3),
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "ApiKey_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "ApiConfig" (
    "id" TEXT NOT NULL,
    "scope" TEXT NOT NULL DEFAULT 'default',
    "isEnabled" BOOLEAN NOT NULL DEFAULT true,
    "requestsPerMinute" INTEGER NOT NULL DEFAULT 20,
    "requestsPerDay" INTEGER NOT NULL DEFAULT 500,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "ApiConfig_pkey" PRIMARY KEY ("id")
);

ALTER TABLE "GenerationJob"
ADD COLUMN "apiKeyId" TEXT,
ADD COLUMN "clientSource" "GenerationClientSource" NOT NULL DEFAULT 'WEB';

CREATE UNIQUE INDEX "ApiKey_keyHash_key" ON "ApiKey"("keyHash");
CREATE UNIQUE INDEX "ApiConfig_scope_key" ON "ApiConfig"("scope");
CREATE INDEX "ApiKey_userId_idx" ON "ApiKey"("userId");
CREATE INDEX "ApiKey_revokedAt_idx" ON "ApiKey"("revokedAt");
CREATE INDEX "GenerationJob_apiKeyId_idx" ON "GenerationJob"("apiKeyId");
CREATE INDEX "GenerationJob_apiKeyId_clientSource_createdAt_idx" ON "GenerationJob"("apiKeyId", "clientSource", "createdAt");
CREATE INDEX "GenerationJob_clientSource_idx" ON "GenerationJob"("clientSource");

ALTER TABLE "ApiKey"
ADD CONSTRAINT "ApiKey_userId_fkey"
FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE "GenerationJob"
ADD CONSTRAINT "GenerationJob_apiKeyId_fkey"
FOREIGN KEY ("apiKeyId") REFERENCES "ApiKey"("id") ON DELETE SET NULL ON UPDATE CASCADE;
