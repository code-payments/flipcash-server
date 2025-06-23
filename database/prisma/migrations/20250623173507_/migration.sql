/*
  Warnings:

  - The `resolution` column on the `flipcash_pools` table would be dropped and recreated. This will lead to data loss if there is data in the column.

*/
-- AlterTable
ALTER TABLE "flipcash_pools" DROP COLUMN "resolution",
ADD COLUMN     "resolution" SMALLINT NOT NULL DEFAULT 0;
