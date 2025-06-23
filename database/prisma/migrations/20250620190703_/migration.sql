-- CreateTable
CREATE TABLE "flipcash_poolmembers" (
    "id" BIGSERIAL NOT NULL,
    "poolId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_poolmembers_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "flipcash_poolmembers_userId_id_idx" ON "flipcash_poolmembers"("userId", "id" ASC);

-- CreateIndex
CREATE UNIQUE INDEX "flipcash_poolmembers_userId_poolId_key" ON "flipcash_poolmembers"("userId", "poolId");
