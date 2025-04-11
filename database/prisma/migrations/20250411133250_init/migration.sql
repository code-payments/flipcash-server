-- CreateTable
CREATE TABLE "flipcash_users" (
    "id" TEXT NOT NULL,
    "isStaff" BOOLEAN NOT NULL DEFAULT false,
    "isRegistered" BOOLEAN NOT NULL DEFAULT false,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_users_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "flipcash_publickeys" (
    "key" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_publickeys_pkey" PRIMARY KEY ("key")
);

-- CreateTable
CREATE TABLE "flipcash_iap" (
    "receiptId" TEXT NOT NULL,
    "platform" SMALLINT NOT NULL DEFAULT 0,
    "userId" TEXT NOT NULL,
    "product" SMALLINT NOT NULL DEFAULT 0,
    "state" SMALLINT NOT NULL DEFAULT 0,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "flipcash_iap_pkey" PRIMARY KEY ("receiptId")
);

-- CreateTable
CREATE TABLE "flipcash_pushtokens" (
    "userId" TEXT NOT NULL,
    "appInstallId" TEXT NOT NULL,
    "token" TEXT NOT NULL,
    "type" INTEGER NOT NULL DEFAULT 0,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_pushtokens_pkey" PRIMARY KEY ("userId","appInstallId")
);

-- AddForeignKey
ALTER TABLE "flipcash_publickeys" ADD CONSTRAINT "flipcash_publickeys_userId_fkey" FOREIGN KEY ("userId") REFERENCES "flipcash_users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
