import { afterAll, afterEach } from "vitest";
import { cleanupDatabase, closeTestDb } from "./helpers";

afterEach(async () => {
  await cleanupDatabase();
});

afterAll(async () => {
  await closeTestDb();
});
