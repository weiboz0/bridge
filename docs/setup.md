# Development Setup

## PostgreSQL Configuration

Bridge requires PostgreSQL 15+ with two databases:

- `bridge` — development database
- `bridge_test` — test database (cleaned between test runs)

### Creating the databases

```bash
# As a PostgreSQL superuser
createdb bridge
createdb bridge_test
```

### Authentication

The development setup uses `trust` authentication for the `work` PostgreSQL user. Add these lines to the **top** of your `pg_hba.conf`:

```
local   all   work   trust
host    all   work   127.0.0.1/32   trust
host    all   work   ::1/128        trust
```

Then reload PostgreSQL:

```bash
sudo systemctl reload postgresql
```

> **Note:** Use `127.0.0.1` (not `localhost`) in connection strings to avoid IPv6 resolution issues where `localhost` may resolve to `::1`.

### Running Migrations

```bash
# Generate migration from schema changes
bun run db:generate

# Apply migrations to dev database
bun run db:migrate

# Apply migrations to test database
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run db:migrate
```

### Drizzle Studio

To browse the database visually:

```bash
bun run db:studio
```

## Google OAuth Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a new project (or select existing)
3. Navigate to **APIs & Services > Credentials**
4. Create an **OAuth 2.0 Client ID** (Web application)
5. Add authorized redirect URI: `http://localhost:3000/api/auth/callback/google`
6. Copy the Client ID and Client Secret to your `.env` file

## Auth.js Secret

Generate a secret for Auth.js:

```bash
openssl rand -base64 32
```

Add it to `.env` as `NEXTAUTH_SECRET`.

## Running Tests

Tests use a separate `bridge_test` database that is cleaned between each test.

```bash
# Run all tests
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test

# Run a specific test file
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test tests/api/classrooms.test.ts

# Watch mode
DATABASE_URL=postgresql://work@127.0.0.1:5432/bridge_test bun run test:watch
```

> **Note:** Tests run sequentially (`fileParallelism: false`) to prevent database cleanup conflicts between test files.
