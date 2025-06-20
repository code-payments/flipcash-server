-- CreateTable
CREATE TABLE "flipcash_pools" (
    "id" TEXT NOT NULL,
    "creatorId" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "buyInCurrency" TEXT NOT NULL,
    "buyInAmount" DOUBLE PRECISION NOT NULL,
    "fundingDestination" TEXT NOT NULL,
    "isOpen" BOOLEAN NOT NULL,
    "resolution" BOOLEAN,
    "signature" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_pools_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "flipcash_bets" (
    "id" TEXT NOT NULL,
    "poolId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "selectedOutcome" BOOLEAN NOT NULL,
    "payoutDestination" TEXT NOT NULL,
    "signature" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_bets_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "flipcash_pools_fundingDestination_key" ON "flipcash_pools"("fundingDestination");

-- CreateIndex
CREATE UNIQUE INDEX "flipcash_bets_poolId_userId_key" ON "flipcash_bets"("poolId", "userId");
