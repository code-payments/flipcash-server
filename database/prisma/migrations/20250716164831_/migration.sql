-- CreateTable
CREATE TABLE "flipcash_rendezvous" (
    "key" TEXT NOT NULL,
    "address" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "expiresAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_rendezvous_pkey" PRIMARY KEY ("key")
);
