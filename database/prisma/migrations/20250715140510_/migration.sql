-- AlterTable
ALTER TABLE "flipcash_users" ADD COLUMN     "displayName" TEXT;

-- CreateTable
CREATE TABLE "flipcash_x_profiles" (
    "id" TEXT NOT NULL,
    "username" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "profilePicUrl" TEXT NOT NULL,
    "followerCount" INTEGER NOT NULL DEFAULT 0,
    "verifiedType" SMALLINT NOT NULL DEFAULT 0,
    "accessToken" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipcash_x_profiles_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "flipcash_x_profiles_username_key" ON "flipcash_x_profiles"("username");

-- CreateIndex
CREATE UNIQUE INDEX "flipcash_x_profiles_userId_key" ON "flipcash_x_profiles"("userId");

-- AddForeignKey
ALTER TABLE "flipcash_x_profiles" ADD CONSTRAINT "flipcash_x_profiles_userId_fkey" FOREIGN KEY ("userId") REFERENCES "flipcash_users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
