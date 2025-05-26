/*
  Warnings:

  - A unique constraint covering the columns `[userId]` on the table `flipcash_publickeys` will be added. If there are existing duplicate values, this will fail.

*/
-- CreateIndex
CREATE UNIQUE INDEX "flipcash_publickeys_userId_key" ON "flipcash_publickeys"("userId");
