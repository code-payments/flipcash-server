/*
  Warnings:

  - Added the required column `paymentAmount` to the `flipcash_iap` table without a default value. This is not possible if the table is not empty.
  - Added the required column `paymentCurrency` to the `flipcash_iap` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "flipcash_iap" ADD COLUMN     "paymentAmount" DOUBLE PRECISION NOT NULL,
ADD COLUMN     "paymentCurrency" TEXT NOT NULL;
