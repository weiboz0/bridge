#!/usr/bin/env node
// Validates the dedicated database URL input without ever echoing it.
import postgres from "postgres";

const INPUT_ENV = "CHECK_TEST_DATABASE_URL";
const TIMEOUT_MS = 5_000;

function parseTestDatabaseURL(value) {
  if (!value) {
    throw new Error("missing URL");
  }

  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error("invalid URL");
  }

  if (parsed.protocol !== "postgres:" && parsed.protocol !== "postgresql:") {
    throw new Error("non-Postgres URL");
  }

  let pathname;
  try {
    pathname = decodeURIComponent(parsed.pathname);
  } catch {
    throw new Error("invalid pathname encoding");
  }

  if (!pathname.startsWith("/") || pathname.length === 1) {
    throw new Error("missing database name");
  }

  const databaseName = pathname.slice(1);
  if (databaseName.includes("/") || !databaseName.endsWith("_test")) {
    throw new Error("non-test database name");
  }

  return databaseName;
}

function withinFiveSeconds(promise) {
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error("timed out")), TIMEOUT_MS);
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

async function validateLiveDatabase(value) {
  const sql = postgres(value, {
    max: 1,
    connect_timeout: 5,
  });

  try {
    const rows = await withinFiveSeconds(sql`SELECT current_database()`);
    const databaseName = rows[0]?.current_database;
    if (typeof databaseName !== "string" || !databaseName.endsWith("_test")) {
      throw new Error("connected database is not a test database");
    }
  } finally {
    await withinFiveSeconds(sql.end({ timeout: 5 })).catch(() => undefined);
  }
}

async function main() {
  const value = process.env[INPUT_ENV];
  parseTestDatabaseURL(value);

  if (process.argv[2] === "--parse-only") {
    if (process.argv.length !== 3) {
      throw new Error("unexpected arguments");
    }
    console.log("test database URL validation: accepted");
    return;
  }

  if (process.argv.length !== 2) {
    throw new Error("unexpected arguments");
  }

  await validateLiveDatabase(value);
  console.log("test database URL validation: accepted");
}

main().catch(() => {
  console.error("test database URL validation: rejected");
  process.exitCode = 1;
});
