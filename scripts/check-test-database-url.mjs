#!/usr/bin/env node
// Validates the dedicated database URL input without ever echoing it.
import net from "node:net";
import postgres from "postgres";

const INPUT_ENV = "CHECK_TEST_DATABASE_URL";
const TIMEOUT_MS = 5_000;
const LIBPQ_ROUTING_OPTIONS = new Set([
  "host", "hostaddr", "port", "dbname", "database", "user", "password",
  "service", "servicefile", "target_session_attrs", "load_balance_hosts",
]);

// This dedicated CLI performs exactly one direct probe, never Postgres.js's
// target-session routing checks inherited from its ambient environment.
delete process.env.PGTARGETSESSIONATTRS;

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

  if (value.includes("#")) {
    throw new Error("URL fragments are not allowed");
  }

  let hostname;
  try {
    hostname = decodeURIComponent(parsed.hostname);
  } catch {
    throw new Error("invalid hostname encoding");
  }

  if (hostname.includes(",")) {
    throw new Error("multiple database hosts");
  }

  if ([...parsed.searchParams.keys()].some((key) => LIBPQ_ROUTING_OPTIONS.has(key.toLowerCase()))) {
    throw new Error("libpq routing options are not allowed");
  }

  const rawPathname = parsed.pathname;
  const encodedPathname = rawPathname.match(/^\/([^/%]*)%5[Ff]test$/);
  if (rawPathname.includes("%") && !encodedPathname) {
    throw new Error("unsupported pathname encoding");
  }
  const pathname = encodedPathname ? `/${encodedPathname[1]}_test` : rawPathname;

  if (!pathname.startsWith("/") || pathname.length === 1) {
    throw new Error("missing database name");
  }

  const databaseName = pathname.slice(1);
  if (databaseName.includes("/") || !databaseName.endsWith("_test")) {
    throw new Error("non-test database name");
  }

  parsed.pathname = `/${databaseName}`;
  return parsed;
}

function createOneShotSocketController() {
  let attempted = false;
  let rawSocket;

  return {
    createSocket(options) {
      if (attempted) {
        throw new Error("test database probe permits one connection attempt");
      }
      attempted = true;

      rawSocket = net.createConnection({
        host: options.host[0],
        port: options.port[0],
      });
      return new Promise((resolve, reject) => {
        let connected = false;
        rawSocket.once("connect", () => {
          connected = true;
          rawSocket.host = options.host[0];
          rawSocket.port = options.port[0];
          resolve(rawSocket);
        });
        rawSocket.once("error", reject);
        rawSocket.once("close", () => {
          if (!connected) {
            reject(new Error("socket closed before connect"));
          }
        });
      });
    },
    destroy() {
      rawSocket?.destroy();
    },
  };
}

async function validateLiveDatabase(value) {
  const socketController = createOneShotSocketController();
  const sql = postgres(value, {
    max: 1,
    connect_timeout: 5,
    fetch_types: false,
    prepare: false,
    socket: socketController.createSocket,
  });

  let deadline;
  try {
    const query = sql`SELECT current_database()`;
    const timeout = new Promise((_, reject) => {
      deadline = setTimeout(() => {
        socketController.destroy();
        void sql.end({ timeout: 0 }).catch(() => undefined);
        reject(new Error("timed out"));
      }, TIMEOUT_MS);
    });
    const rows = await Promise.race([query, timeout]);
    const databaseName = rows[0]?.current_database;
    if (typeof databaseName !== "string" || !databaseName.endsWith("_test")) {
      throw new Error("connected database is not a test database");
    }
  } finally {
    clearTimeout(deadline);
    socketController.destroy();
    await sql.end({ timeout: 0 }).catch(() => undefined);
  }
}

async function main() {
  const value = process.env[INPUT_ENV];
  const parsed = parseTestDatabaseURL(value);

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

  await validateLiveDatabase(parsed.toString());
  console.log("test database URL validation: accepted");
}

main().catch(() => {
  console.error("test database URL validation: rejected");
  process.exitCode = 1;
});
