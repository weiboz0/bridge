# Foundation Implementation Plan (Plan 1 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Set up the Bridge project from scratch with auth, database, classroom management, and a basic UI shell — enough to create accounts, log in, create classrooms, and join them via codes.

**Architecture:** Next.js monolith on Bun runtime. Drizzle ORM with PostgreSQL for data. Auth.js v5 for authentication (Google OAuth + email/password). API routes handle all server logic. shadcn/ui for component library.

**Tech Stack:** Bun, Next.js 15 (App Router), TypeScript, Drizzle ORM, PostgreSQL, Redis, Auth.js v5, Vitest, shadcn/ui, Tailwind CSS, Zod

**Spec:** `docs/superpowers/specs/001-bridge-platform-design.md`

**Subsequent plans:**
- Plan 2: Live Editor (CodeMirror 6 + Pyodide)
- Plan 3: Real-time Sessions (Yjs + Hocuspocus + teacher dashboard)
- Plan 4: AI & Interaction (AI tutor + annotations + help queue + broadcast)

---

## File Structure

```
bridge/
├── src/
│   ├── app/
│   │   ├── layout.tsx                              # Root layout with providers
│   │   ├── page.tsx                                # Landing page → redirect
│   │   ├── (auth)/
│   │   │   ├── login/page.tsx                      # Login form (OAuth + credentials)
│   │   │   └── register/page.tsx                   # Registration form
│   │   ├── dashboard/
│   │   │   ├── layout.tsx                          # Authenticated layout with nav
│   │   │   ├── page.tsx                            # Dashboard home
│   │   │   └── classrooms/
│   │   │       ├── page.tsx                        # List my classrooms
│   │   │       ├── new/page.tsx                    # Create classroom form
│   │   │       ├── join/page.tsx                   # Join via code form
│   │   │       └── [id]/page.tsx                   # Classroom detail
│   │   └── api/
│   │       ├── auth/[...nextauth]/route.ts         # Auth.js handler
│   │       ├── auth/register/route.ts              # Registration endpoint
│   │       ├── classrooms/route.ts                 # GET (list) / POST (create)
│   │       ├── classrooms/[id]/route.ts            # GET / PUT / DELETE
│   │       ├── classrooms/[id]/members/route.ts    # GET members
│   │       └── classrooms/join/route.ts            # POST join by code
│   ├── lib/
│   │   ├── db/
│   │   │   ├── index.ts                            # Drizzle client instance
│   │   │   └── schema.ts                           # All table definitions
│   │   ├── auth.ts                                 # Auth.js config + helpers
│   │   ├── classrooms.ts                           # Classroom business logic
│   │   └── utils.ts                                # joinCode generator, etc.
│   ├── components/
│   │   ├── session-provider.tsx                    # NextAuth SessionProvider wrapper
│   │   ├── sign-out-button.tsx                     # Sign out button component
│   │   └── ui/                                     # shadcn/ui (auto-generated)
│   ├── types/
│   │   └── next-auth.d.ts                          # Auth.js type augmentation
│   └── middleware.ts                               # Auth route protection
├── tests/
│   ├── setup.ts                                    # Global test setup
│   ├── helpers.ts                                  # DB helpers, factories
│   ├── unit/
│   │   ├── schema.test.ts                          # Schema validation tests
│   │   └── utils.test.ts                           # Utility function tests
│   └── api/
│       ├── db-connection.test.ts                   # DB connection integration tests
│       ├── classrooms.test.ts                      # Classroom CRUD tests
│       └── classrooms-join.test.ts                 # Join code tests
├── drizzle.config.ts                               # Drizzle Kit config
├── .env.example                                    # Environment template
├── .gitignore
├── next.config.ts                                  # Next.js config
├── package.json
├── tsconfig.json
├── tailwind.config.ts
├── postcss.config.mjs
└── vitest.config.ts                                # Vitest config
```

---

## Task 1: Project Scaffolding

**Files:**
- Create: `package.json`, `tsconfig.json`, `next.config.ts`, `tailwind.config.ts`, `postcss.config.mjs`, `.env.example`, `.gitignore`, `vitest.config.ts`, `tests/setup.ts`, `src/app/layout.tsx`, `src/app/page.tsx`

- [ ] **Step 1: Create Next.js project with Bun**

```bash
cd /home/chris/workshop/Bridge
bunx create-next-app@latest . --typescript --tailwind --eslint --app --src-dir --import-alias "@/*" --no-turbopack
```

Accept defaults. This creates the Next.js scaffold with App Router, TypeScript, Tailwind, and `src/` directory.

- [ ] **Step 2: Install core dependencies**

```bash
bun add drizzle-orm postgres zod nanoid
bun add -d drizzle-kit vitest @vitejs/plugin-react @testing-library/react @testing-library/jest-dom jsdom
```

- [ ] **Step 3: Create `.env.example`**

Create `.env.example`:

```env
# Database
DATABASE_URL=postgresql://bridge:bridge@localhost:5432/bridge

# Auth
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=generate-a-secret-with-openssl-rand-base64-32

# Google OAuth
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=

# Redis
REDIS_URL=redis://localhost:6379
```

- [ ] **Step 4: Update `.gitignore`**

Append to the generated `.gitignore`:

```
# env
.env
.env.local
.env.test

# editor
.vscode/
.idea/
```

- [ ] **Step 5: Configure Vitest**

Create `vitest.config.ts`:

```typescript
import { defineConfig } from "vitest/config";
import path from "path";

export default defineConfig({
  test: {
    globals: true,
    environment: "node",
    setupFiles: ["./tests/setup.ts"],
    include: ["tests/**/*.test.ts"],
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
```

Create `tests/setup.ts`:

```typescript
import { afterAll } from "vitest";

afterAll(async () => {
  // Cleanup hooks for database connections etc.
  // Will be populated in Task 2
});
```

- [ ] **Step 6: Add test script to package.json**

Add to the `"scripts"` section of `package.json`:

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [ ] **Step 7: Write smoke test**

Create `tests/unit/smoke.test.ts`:

```typescript
import { describe, it, expect } from "vitest";

describe("smoke test", () => {
  it("should run tests", () => {
    expect(1 + 1).toBe(2);
  });
});
```

- [ ] **Step 8: Run smoke test**

```bash
bun run test
```

Expected: 1 test passes.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "chore: scaffold Next.js project with Bun, Vitest, Tailwind"
```

---

## Task 2: Database Schema & Connection

**Files:**
- Create: `src/lib/db/schema.ts`, `src/lib/db/index.ts`, `drizzle.config.ts`, `tests/helpers.ts`
- Modify: `tests/setup.ts`
- Test: `tests/unit/schema.test.ts`

- [ ] **Step 1: Write failing test for schema exports**

Create `tests/unit/schema.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import {
  users,
  authProviders,
  schools,
  classrooms,
  classroomMembers,
} from "@/lib/db/schema";

describe("database schema", () => {
  it("exports users table with expected columns", () => {
    const columns = Object.keys(users);
    expect(columns).toContain("id");
    expect(columns).toContain("name");
    expect(columns).toContain("email");
    expect(columns).toContain("role");
    expect(columns).toContain("schoolId");
    expect(columns).toContain("passwordHash");
    expect(columns).toContain("createdAt");
  });

  it("exports authProviders table with expected columns", () => {
    const columns = Object.keys(authProviders);
    expect(columns).toContain("id");
    expect(columns).toContain("userId");
    expect(columns).toContain("provider");
    expect(columns).toContain("providerUserId");
  });

  it("exports schools table with expected columns", () => {
    const columns = Object.keys(schools);
    expect(columns).toContain("id");
    expect(columns).toContain("name");
    expect(columns).toContain("settings");
  });

  it("exports classrooms table with expected columns", () => {
    const columns = Object.keys(classrooms);
    expect(columns).toContain("id");
    expect(columns).toContain("schoolId");
    expect(columns).toContain("teacherId");
    expect(columns).toContain("name");
    expect(columns).toContain("gradeLevel");
    expect(columns).toContain("editorMode");
    expect(columns).toContain("joinCode");
  });

  it("exports classroomMembers table with expected columns", () => {
    const columns = Object.keys(classroomMembers);
    expect(columns).toContain("classroomId");
    expect(columns).toContain("userId");
    expect(columns).toContain("joinedAt");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
bun run test tests/unit/schema.test.ts
```

Expected: FAIL — cannot resolve `@/lib/db/schema`.

- [ ] **Step 3: Define Drizzle schema**

Create `src/lib/db/schema.ts`:

```typescript
import {
  pgTable,
  uuid,
  varchar,
  text,
  timestamp,
  jsonb,
  pgEnum,
  uniqueIndex,
  index,
} from "drizzle-orm/pg-core";

export const userRoleEnum = pgEnum("user_role", [
  "admin",
  "teacher",
  "student",
]);

export const authProviderEnum = pgEnum("auth_provider", [
  "google",
  "microsoft",
  "email",
]);

export const gradeLevelEnum = pgEnum("grade_level", ["K-5", "6-8", "9-12"]);

export const editorModeEnum = pgEnum("editor_mode", [
  "blockly",
  "python",
  "javascript",
]);

// --- Tables ---

export const schools = pgTable("schools", {
  id: uuid("id").primaryKey().defaultRandom(),
  name: varchar("name", { length: 255 }).notNull(),
  settings: jsonb("settings").default({}),
  createdAt: timestamp("created_at").defaultNow().notNull(),
  updatedAt: timestamp("updated_at").defaultNow().notNull(),
});

export const users = pgTable(
  "users",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    name: varchar("name", { length: 255 }).notNull(),
    email: varchar("email", { length: 255 }).notNull(),
    role: userRoleEnum("role").notNull().default("student"),
    avatarUrl: text("avatar_url"),
    passwordHash: text("password_hash"),
    schoolId: uuid("school_id").references(() => schools.id),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [uniqueIndex("users_email_idx").on(table.email)]
);

export const authProviders = pgTable(
  "auth_providers",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    userId: uuid("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    provider: authProviderEnum("provider").notNull(),
    providerUserId: varchar("provider_user_id", { length: 255 }).notNull(),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("auth_provider_unique_idx").on(
      table.provider,
      table.providerUserId
    ),
  ]
);

export const classrooms = pgTable(
  "classrooms",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    schoolId: uuid("school_id").references(() => schools.id),
    teacherId: uuid("teacher_id")
      .notNull()
      .references(() => users.id),
    name: varchar("name", { length: 255 }).notNull(),
    description: text("description").default(""),
    gradeLevel: gradeLevelEnum("grade_level").notNull(),
    editorMode: editorModeEnum("editor_mode").notNull().default("python"),
    joinCode: varchar("join_code", { length: 10 }).notNull(),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("classrooms_join_code_idx").on(table.joinCode),
    index("classrooms_teacher_idx").on(table.teacherId),
  ]
);

export const classroomMembers = pgTable(
  "classroom_members",
  {
    classroomId: uuid("classroom_id")
      .notNull()
      .references(() => classrooms.id, { onDelete: "cascade" }),
    userId: uuid("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    joinedAt: timestamp("joined_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("classroom_member_unique_idx").on(
      table.classroomId,
      table.userId
    ),
  ]
);
```

- [ ] **Step 4: Run schema test**

```bash
bun run test tests/unit/schema.test.ts
```

Expected: All 5 tests pass.

- [ ] **Step 5: Set up Drizzle client**

Create `src/lib/db/index.ts`:

```typescript
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import * as schema from "./schema";

const connectionString = process.env.DATABASE_URL!;

const client = postgres(connectionString);

export const db = drizzle(client, { schema });

export type Database = typeof db;
```

- [ ] **Step 6: Create Drizzle config**

Create `drizzle.config.ts`:

```typescript
import { defineConfig } from "drizzle-kit";

export default defineConfig({
  schema: "./src/lib/db/schema.ts",
  out: "./drizzle",
  dialect: "postgresql",
  dbCredentials: {
    url: process.env.DATABASE_URL!,
  },
});
```

- [ ] **Step 7: Add migration scripts to package.json**

Add to the `"scripts"` section of `package.json`:

```json
"db:generate": "drizzle-kit generate",
"db:migrate": "drizzle-kit migrate",
"db:studio": "drizzle-kit studio"
```

- [ ] **Step 8: Create test database and run migration**

```bash
# Create the database (adjust credentials as needed)
createdb bridge

# Copy .env.example to .env and fill in DATABASE_URL
cp .env.example .env
# Edit .env with actual DATABASE_URL

# Generate and run migration
bun run db:generate
bun run db:migrate
```

Expected: Migration runs successfully, tables are created.

- [ ] **Step 9: Create test helpers**

Create `tests/helpers.ts`:

```typescript
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import * as schema from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { nanoid } from "nanoid";

const testClient = postgres(
  process.env.DATABASE_URL || "postgresql://bridge:bridge@localhost:5432/bridge_test"
);
export const testDb = drizzle(testClient, { schema });

export async function cleanupDatabase() {
  await testDb.delete(schema.classroomMembers);
  await testDb.delete(schema.classrooms);
  await testDb.delete(schema.authProviders);
  await testDb.delete(schema.users);
  await testDb.delete(schema.schools);
}

export async function createTestSchool(overrides: Partial<typeof schema.schools.$inferInsert> = {}) {
  const [school] = await testDb
    .insert(schema.schools)
    .values({
      name: "Test School",
      ...overrides,
    })
    .returning();
  return school;
}

export async function createTestUser(
  overrides: Partial<typeof schema.users.$inferInsert> = {}
) {
  const [user] = await testDb
    .insert(schema.users)
    .values({
      name: "Test User",
      email: `test-${nanoid(6)}@example.com`,
      role: "teacher",
      ...overrides,
    })
    .returning();
  return user;
}

export async function createTestClassroom(
  teacherId: string,
  overrides: Partial<typeof schema.classrooms.$inferInsert> = {}
) {
  const [classroom] = await testDb
    .insert(schema.classrooms)
    .values({
      teacherId,
      name: "Test Classroom",
      gradeLevel: "6-8",
      editorMode: "python",
      joinCode: nanoid(8),
      ...overrides,
    })
    .returning();
  return classroom;
}

export async function closeTestDb() {
  await testClient.end();
}
```

Update `tests/setup.ts`:

```typescript
import { afterAll, afterEach } from "vitest";
import { cleanupDatabase, closeTestDb } from "./helpers";

afterEach(async () => {
  await cleanupDatabase();
});

afterAll(async () => {
  await closeTestDb();
});
```

- [ ] **Step 10: Write integration test for DB connection**

Create `tests/api/db-connection.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { testDb, createTestSchool, createTestUser } from "../helpers";
import { users, schools } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

describe("database connection", () => {
  it("can insert and query a school", async () => {
    const school = await createTestSchool({ name: "Bridge Academy" });

    expect(school.id).toBeDefined();
    expect(school.name).toBe("Bridge Academy");

    const [found] = await testDb
      .select()
      .from(schools)
      .where(eq(schools.id, school.id));

    expect(found.name).toBe("Bridge Academy");
  });

  it("can insert and query a user", async () => {
    const user = await createTestUser({
      name: "Alice",
      email: "alice@school.edu",
      role: "student",
    });

    expect(user.id).toBeDefined();
    expect(user.role).toBe("student");

    const [found] = await testDb
      .select()
      .from(users)
      .where(eq(users.id, user.id));

    expect(found.name).toBe("Alice");
    expect(found.email).toBe("alice@school.edu");
  });

  it("enforces unique email constraint", async () => {
    await createTestUser({ email: "duplicate@school.edu" });

    await expect(
      createTestUser({ email: "duplicate@school.edu" })
    ).rejects.toThrow();
  });
});
```

- [ ] **Step 11: Create test database and run integration tests**

```bash
# Create test database
createdb bridge_test

# Run migration against test DB
DATABASE_URL=postgresql://bridge:bridge@localhost:5432/bridge_test bun run db:migrate

# Run tests
DATABASE_URL=postgresql://bridge:bridge@localhost:5432/bridge_test bun run test
```

Expected: All tests pass (schema unit tests + DB integration tests).

- [ ] **Step 12: Commit**

```bash
git add src/lib/db/ drizzle.config.ts drizzle/ tests/
git commit -m "feat: add database schema with Drizzle ORM

Define tables for users, auth_providers, schools, classrooms,
and classroom_members. Include test helpers and integration tests."
```

---

## Task 3: Utility Functions

**Files:**
- Create: `src/lib/utils.ts`
- Test: `tests/unit/utils.test.ts`

- [ ] **Step 1: Write failing tests for join code generator**

Create `tests/unit/utils.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import { generateJoinCode } from "@/lib/utils";

describe("generateJoinCode", () => {
  it("returns a string of 8 characters", () => {
    const code = generateJoinCode();
    expect(code).toHaveLength(8);
  });

  it("contains only uppercase alphanumeric characters", () => {
    const code = generateJoinCode();
    expect(code).toMatch(/^[A-Z0-9]{8}$/);
  });

  it("generates unique codes", () => {
    const codes = new Set(Array.from({ length: 100 }, () => generateJoinCode()));
    expect(codes.size).toBe(100);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
bun run test tests/unit/utils.test.ts
```

Expected: FAIL — `generateJoinCode` is not exported.

- [ ] **Step 3: Implement generateJoinCode**

Create `src/lib/utils.ts`:

```typescript
import { customAlphabet } from "nanoid";

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"; // no 0/O/1/I to avoid confusion
const generate = customAlphabet(alphabet, 8);

export function generateJoinCode(): string {
  return generate();
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
bun run test tests/unit/utils.test.ts
```

Expected: All 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/utils.ts tests/unit/utils.test.ts
git commit -m "feat: add join code generator utility"
```

---

## Task 4: Authentication

**Files:**
- Create: `src/lib/auth.ts`, `src/app/api/auth/[...nextauth]/route.ts`, `src/middleware.ts`
- Modify: `src/app/layout.tsx`

- [ ] **Step 1: Install auth dependencies**

```bash
bun add next-auth@beta @auth/drizzle-adapter
```

- [ ] **Step 2: Configure Auth.js**

Create `src/lib/auth.ts`:

```typescript
import NextAuth from "next-auth";
import Google from "next-auth/providers/google";
import Credentials from "next-auth/providers/credentials";
import { db } from "@/lib/db";
import { users, authProviders } from "@/lib/db/schema";
import { eq, and } from "drizzle-orm";
import { z } from "zod";

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
});

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [
    Google({
      clientId: process.env.GOOGLE_CLIENT_ID!,
      clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
    }),
    Credentials({
      name: "Email",
      credentials: {
        email: { label: "Email", type: "email" },
        password: { label: "Password", type: "password" },
      },
      async authorize(credentials) {
        const parsed = loginSchema.safeParse(credentials);
        if (!parsed.success) return null;

        const [user] = await db
          .select()
          .from(users)
          .where(eq(users.email, parsed.data.email));

        if (!user || !user.passwordHash) return null;

        const valid = await Bun.password.verify(
          parsed.data.password,
          user.passwordHash
        );
        if (!valid) return null;

        return {
          id: user.id,
          name: user.name,
          email: user.email,
          role: user.role,
        };
      },
    }),
  ],
  callbacks: {
    async signIn({ user, account }) {
      if (account?.provider === "google" && user.email) {
        // Upsert user on Google sign-in
        const [existing] = await db
          .select()
          .from(users)
          .where(eq(users.email, user.email));

        let userId: string;

        if (existing) {
          userId = existing.id;
        } else {
          const [newUser] = await db
            .insert(users)
            .values({
              name: user.name || "Unknown",
              email: user.email,
              role: "teacher", // default role for OAuth sign-up
              avatarUrl: user.image,
            })
            .returning();
          userId = newUser.id;
        }

        // Upsert auth provider link
        const [existingProvider] = await db
          .select()
          .from(authProviders)
          .where(
            and(
              eq(authProviders.provider, "google"),
              eq(authProviders.providerUserId, account.providerAccountId)
            )
          );

        if (!existingProvider) {
          await db.insert(authProviders).values({
            userId,
            provider: "google",
            providerUserId: account.providerAccountId,
          });
        }
      }
      return true;
    },
    async jwt({ token, user }) {
      if (user) {
        // On sign-in, look up the DB user to get role
        const [dbUser] = await db
          .select()
          .from(users)
          .where(eq(users.email, token.email!));
        if (dbUser) {
          token.id = dbUser.id;
          token.role = dbUser.role;
        }
      }
      return token;
    },
    async session({ session, token }) {
      if (token) {
        session.user.id = token.id as string;
        session.user.role = token.role as string;
      }
      return session;
    },
  },
  pages: {
    signIn: "/login",
  },
  session: {
    strategy: "jwt",
  },
});
```

- [ ] **Step 3: Add type augmentation for session**

Create `src/types/next-auth.d.ts`:

```typescript
import { DefaultSession } from "next-auth";

declare module "next-auth" {
  interface Session {
    user: {
      id: string;
      role: string;
    } & DefaultSession["user"];
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    id?: string;
    role?: string;
  }
}
```

- [ ] **Step 4: Create auth API route**

Create `src/app/api/auth/[...nextauth]/route.ts`:

```typescript
import { handlers } from "@/lib/auth";

export const { GET, POST } = handlers;
```

- [ ] **Step 5: Create registration API route**

Create `src/app/api/auth/register/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { db } from "@/lib/db";
import { users, authProviders } from "@/lib/db/schema";
import { eq } from "drizzle-orm";

const registerSchema = z.object({
  name: z.string().min(1).max(255),
  email: z.string().email(),
  password: z.string().min(8).max(128),
  role: z.enum(["teacher", "student"]),
});

export async function POST(request: NextRequest) {
  const body = await request.json();
  const parsed = registerSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const { name, email, password, role } = parsed.data;

  // Check if email already exists
  const [existing] = await db
    .select()
    .from(users)
    .where(eq(users.email, email));

  if (existing) {
    return NextResponse.json(
      { error: "Email already registered" },
      { status: 409 }
    );
  }

  const passwordHash = await Bun.password.hash(password);

  const [user] = await db
    .insert(users)
    .values({
      name,
      email,
      role,
      passwordHash,
    })
    .returning({
      id: users.id,
      name: users.name,
      email: users.email,
      role: users.role,
    });

  // Create email auth provider link
  await db.insert(authProviders).values({
    userId: user.id,
    provider: "email",
    providerUserId: user.id,
  });

  return NextResponse.json(user, { status: 201 });
}
```

- [ ] **Step 6: Create auth middleware**

Create `src/middleware.ts`:

```typescript
export { auth as middleware } from "@/lib/auth";

export const config = {
  matcher: ["/dashboard/:path*", "/api/classrooms/:path*"],
};
```

- [ ] **Step 7: Commit**

```bash
git add src/lib/auth.ts src/types/next-auth.d.ts src/app/api/auth/ src/middleware.ts
git commit -m "feat: add authentication with Auth.js

Google OAuth and email/password credentials providers.
JWT session with role in token. Registration endpoint.
Middleware protects dashboard and API routes."
```

---

## Task 5: Classroom CRUD API

**Files:**
- Create: `src/app/api/classrooms/route.ts`, `src/app/api/classrooms/[id]/route.ts`, `src/app/api/classrooms/[id]/members/route.ts`, `src/app/api/classrooms/join/route.ts`
- Test: `tests/api/classrooms.test.ts`, `tests/api/classrooms-join.test.ts`

- [ ] **Step 1: Write failing tests for classroom creation and listing**

Create `tests/api/classrooms.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import {
  testDb,
  createTestUser,
  createTestClassroom,
} from "../helpers";
import { classrooms, classroomMembers } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import { generateJoinCode } from "@/lib/utils";
import {
  createClassroom,
  listClassrooms,
  getClassroom,
  getClassroomMembers,
} from "@/lib/classrooms";

describe("classroom operations", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@school.edu" });
    student = await createTestUser({ role: "student", email: "student@school.edu" });
  });

  describe("createClassroom", () => {
    it("creates a classroom with a generated join code", async () => {
      const classroom = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Intro to Python",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      expect(classroom.id).toBeDefined();
      expect(classroom.name).toBe("Intro to Python");
      expect(classroom.teacherId).toBe(teacher.id);
      expect(classroom.joinCode).toHaveLength(8);
    });

    it("creates unique join codes", async () => {
      const c1 = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Class 1",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });
      const c2 = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Class 2",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      expect(c1.joinCode).not.toBe(c2.joinCode);
    });
  });

  describe("listClassrooms", () => {
    it("returns classrooms where user is teacher", async () => {
      await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "My Class",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      const results = await listClassrooms(testDb, teacher.id);
      expect(results).toHaveLength(1);
      expect(results[0].name).toBe("My Class");
    });

    it("returns classrooms where user is a member", async () => {
      const classroom = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Shared Class",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      await testDb.insert(classroomMembers).values({
        classroomId: classroom.id,
        userId: student.id,
      });

      const results = await listClassrooms(testDb, student.id);
      expect(results).toHaveLength(1);
      expect(results[0].name).toBe("Shared Class");
    });

    it("returns empty array for user with no classrooms", async () => {
      const results = await listClassrooms(testDb, student.id);
      expect(results).toHaveLength(0);
    });
  });

  describe("getClassroom", () => {
    it("returns classroom by ID", async () => {
      const created = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Find Me",
        gradeLevel: "9-12" as const,
        editorMode: "javascript" as const,
      });

      const found = await getClassroom(testDb, created.id);
      expect(found).not.toBeNull();
      expect(found!.name).toBe("Find Me");
      expect(found!.editorMode).toBe("javascript");
    });

    it("returns null for non-existent ID", async () => {
      const found = await getClassroom(
        testDb,
        "00000000-0000-0000-0000-000000000000"
      );
      expect(found).toBeNull();
    });
  });

  describe("getClassroomMembers", () => {
    it("returns all members of a classroom", async () => {
      const classroom = await createClassroom(testDb, {
        teacherId: teacher.id,
        name: "Full Class",
        gradeLevel: "6-8" as const,
        editorMode: "python" as const,
      });

      await testDb.insert(classroomMembers).values({
        classroomId: classroom.id,
        userId: student.id,
      });

      const members = await getClassroomMembers(testDb, classroom.id);
      expect(members).toHaveLength(1);
      expect(members[0].userId).toBe(student.id);
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
bun run test tests/api/classrooms.test.ts
```

Expected: FAIL — cannot resolve `@/lib/classrooms`.

- [ ] **Step 3: Implement classroom operations**

Create `src/lib/classrooms.ts`:

```typescript
import { eq, or } from "drizzle-orm";
import { classrooms, classroomMembers, users } from "@/lib/db/schema";
import { generateJoinCode } from "@/lib/utils";
import type { Database } from "@/lib/db";

interface CreateClassroomInput {
  teacherId: string;
  name: string;
  description?: string;
  gradeLevel: "K-5" | "6-8" | "9-12";
  editorMode: "blockly" | "python" | "javascript";
  schoolId?: string;
}

export async function createClassroom(db: Database, input: CreateClassroomInput) {
  const [classroom] = await db
    .insert(classrooms)
    .values({
      ...input,
      joinCode: generateJoinCode(),
    })
    .returning();
  return classroom;
}

export async function listClassrooms(db: Database, userId: string) {
  // Get classrooms where user is teacher OR a member
  const teacherClassrooms = await db
    .select()
    .from(classrooms)
    .where(eq(classrooms.teacherId, userId));

  const memberClassroomIds = await db
    .select({ classroomId: classroomMembers.classroomId })
    .from(classroomMembers)
    .where(eq(classroomMembers.userId, userId));

  if (memberClassroomIds.length === 0) return teacherClassrooms;

  const memberClassrooms = await db
    .select()
    .from(classrooms)
    .where(
      or(
        ...memberClassroomIds.map((m) => eq(classrooms.id, m.classroomId))
      )
    );

  // Merge and deduplicate
  const seen = new Set(teacherClassrooms.map((c) => c.id));
  const result = [...teacherClassrooms];
  for (const c of memberClassrooms) {
    if (!seen.has(c.id)) {
      result.push(c);
      seen.add(c.id);
    }
  }
  return result;
}

export async function getClassroom(db: Database, classroomId: string) {
  const [classroom] = await db
    .select()
    .from(classrooms)
    .where(eq(classrooms.id, classroomId));
  return classroom || null;
}

export async function getClassroomByJoinCode(db: Database, joinCode: string) {
  const [classroom] = await db
    .select()
    .from(classrooms)
    .where(eq(classrooms.joinCode, joinCode));
  return classroom || null;
}

export async function joinClassroom(
  db: Database,
  classroomId: string,
  userId: string
) {
  const [member] = await db
    .insert(classroomMembers)
    .values({ classroomId, userId })
    .onConflictDoNothing()
    .returning();
  return member;
}

export async function getClassroomMembers(db: Database, classroomId: string) {
  return db
    .select({
      userId: classroomMembers.userId,
      joinedAt: classroomMembers.joinedAt,
      name: users.name,
      email: users.email,
      role: users.role,
    })
    .from(classroomMembers)
    .innerJoin(users, eq(classroomMembers.userId, users.id))
    .where(eq(classroomMembers.classroomId, classroomId));
}
```

- [ ] **Step 4: Run tests**

```bash
bun run test tests/api/classrooms.test.ts
```

Expected: All tests pass.

- [ ] **Step 5: Write failing tests for join-by-code**

Create `tests/api/classrooms-join.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestUser } from "../helpers";
import { classrooms, classroomMembers } from "@/lib/db/schema";
import { eq } from "drizzle-orm";
import {
  createClassroom,
  getClassroomByJoinCode,
  joinClassroom,
  getClassroomMembers,
} from "@/lib/classrooms";

describe("join classroom by code", () => {
  let teacher: Awaited<ReturnType<typeof createTestUser>>;
  let student: Awaited<ReturnType<typeof createTestUser>>;

  beforeEach(async () => {
    teacher = await createTestUser({ role: "teacher", email: "teacher@school.edu" });
    student = await createTestUser({ role: "student", email: "student@school.edu" });
  });

  it("finds classroom by join code", async () => {
    const classroom = await createClassroom(testDb, {
      teacherId: teacher.id,
      name: "Join Me",
      gradeLevel: "6-8" as const,
      editorMode: "python" as const,
    });

    const found = await getClassroomByJoinCode(testDb, classroom.joinCode);
    expect(found).not.toBeNull();
    expect(found!.id).toBe(classroom.id);
  });

  it("returns null for invalid join code", async () => {
    const found = await getClassroomByJoinCode(testDb, "INVALID1");
    expect(found).toBeNull();
  });

  it("adds student to classroom via join", async () => {
    const classroom = await createClassroom(testDb, {
      teacherId: teacher.id,
      name: "Join Me",
      gradeLevel: "6-8" as const,
      editorMode: "python" as const,
    });

    const member = await joinClassroom(testDb, classroom.id, student.id);
    expect(member).toBeDefined();

    const members = await getClassroomMembers(testDb, classroom.id);
    expect(members).toHaveLength(1);
    expect(members[0].userId).toBe(student.id);
  });

  it("does not duplicate membership on re-join", async () => {
    const classroom = await createClassroom(testDb, {
      teacherId: teacher.id,
      name: "Join Me",
      gradeLevel: "6-8" as const,
      editorMode: "python" as const,
    });

    await joinClassroom(testDb, classroom.id, student.id);
    await joinClassroom(testDb, classroom.id, student.id); // duplicate

    const members = await getClassroomMembers(testDb, classroom.id);
    expect(members).toHaveLength(1);
  });
});
```

- [ ] **Step 6: Run join tests**

```bash
bun run test tests/api/classrooms-join.test.ts
```

Expected: All 4 tests pass (the implementation already covers these).

- [ ] **Step 7: Create classroom API routes**

Create `src/app/api/classrooms/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { createClassroom, listClassrooms } from "@/lib/classrooms";

const createSchema = z.object({
  name: z.string().min(1).max(255),
  description: z.string().max(1000).optional(),
  gradeLevel: z.enum(["K-5", "6-8", "9-12"]),
  editorMode: z.enum(["blockly", "python", "javascript"]),
});

export async function GET() {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const results = await listClassrooms(db, session.user.id);
  return NextResponse.json(results);
}

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (session.user.role !== "teacher" && session.user.role !== "admin") {
    return NextResponse.json(
      { error: "Only teachers can create classrooms" },
      { status: 403 }
    );
  }

  const body = await request.json();
  const parsed = createSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid input", details: parsed.error.flatten() },
      { status: 400 }
    );
  }

  const classroom = await createClassroom(db, {
    teacherId: session.user.id,
    ...parsed.data,
  });

  return NextResponse.json(classroom, { status: 201 });
}
```

Create `src/app/api/classrooms/[id]/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClassroom } from "@/lib/classrooms";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const classroom = await getClassroom(db, id);

  if (!classroom) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  return NextResponse.json(classroom);
}
```

Create `src/app/api/classrooms/[id]/members/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClassroom, getClassroomMembers } from "@/lib/classrooms";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { id } = await params;
  const classroom = await getClassroom(db, id);

  if (!classroom) {
    return NextResponse.json({ error: "Not found" }, { status: 404 });
  }

  const members = await getClassroomMembers(db, id);
  return NextResponse.json(members);
}
```

Create `src/app/api/classrooms/join/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClassroomByJoinCode, joinClassroom } from "@/lib/classrooms";

const joinSchema = z.object({
  joinCode: z.string().length(8),
});

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json();
  const parsed = joinSchema.safeParse(body);

  if (!parsed.success) {
    return NextResponse.json(
      { error: "Invalid join code" },
      { status: 400 }
    );
  }

  const classroom = await getClassroomByJoinCode(db, parsed.data.joinCode);
  if (!classroom) {
    return NextResponse.json(
      { error: "Classroom not found" },
      { status: 404 }
    );
  }

  await joinClassroom(db, classroom.id, session.user.id);

  return NextResponse.json(classroom);
}
```

- [ ] **Step 8: Commit**

```bash
git add src/lib/classrooms.ts src/app/api/classrooms/ tests/api/classrooms.test.ts tests/api/classrooms-join.test.ts
git commit -m "feat: add classroom CRUD and join-by-code

Classroom creation with auto-generated join codes, listing
(as teacher or member), get by ID, join by code with duplicate
prevention, and member listing. Full test coverage."
```

---

## Task 6: UI Shell — Layout and Auth Pages

**Files:**
- Create/Modify: `src/app/layout.tsx`, `src/app/page.tsx`, `src/app/(auth)/login/page.tsx`, `src/app/(auth)/register/page.tsx`

- [ ] **Step 1: Install shadcn/ui**

```bash
bunx shadcn@latest init -d
```

Accept defaults (New York style, Zinc color, CSS variables).

Then add required components:

```bash
bunx shadcn@latest add button card input label form
```

- [ ] **Step 2: Create SessionProvider wrapper**

Create `src/components/session-provider.tsx`:

```typescript
"use client";

import { SessionProvider as NextAuthSessionProvider } from "next-auth/react";

export function SessionProvider({ children }: { children: React.ReactNode }) {
  return <NextAuthSessionProvider>{children}</NextAuthSessionProvider>;
}
```

- [ ] **Step 3: Update root layout**

Replace `src/app/layout.tsx`:

```typescript
import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { SessionProvider } from "@/components/session-provider";

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "Bridge - Learn to Code",
  description: "A live-first K-12 coding education platform",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className={inter.className}>
        <SessionProvider>{children}</SessionProvider>
      </body>
    </html>
  );
}
```

- [ ] **Step 4: Create landing page**

Replace `src/app/page.tsx`:

```typescript
import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import Link from "next/link";
import { Button } from "@/components/ui/button";

export default async function Home() {
  const session = await auth();

  if (session) {
    redirect("/dashboard");
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 p-4">
      <h1 className="text-4xl font-bold">Bridge</h1>
      <p className="text-lg text-muted-foreground text-center max-w-md">
        A live-first coding education platform for K-12 classrooms
      </p>
      <div className="flex gap-4">
        <Button asChild>
          <Link href="/login">Log In</Link>
        </Button>
        <Button variant="outline" asChild>
          <Link href="/register">Sign Up</Link>
        </Button>
      </div>
    </main>
  );
}
```

- [ ] **Step 5: Create login page**

Create `src/app/(auth)/login/page.tsx`:

```typescript
"use client";

import { signIn } from "next-auth/react";
import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  CardFooter,
} from "@/components/ui/card";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");

    const result = await signIn("credentials", {
      email,
      password,
      redirect: false,
    });

    if (result?.error) {
      setError("Invalid email or password");
      setLoading(false);
    } else {
      router.push("/dashboard");
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Log In to Bridge</CardTitle>
          <CardDescription>
            Sign in with your account to continue
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Button
            variant="outline"
            className="w-full"
            onClick={() => signIn("google", { callbackUrl: "/dashboard" })}
          >
            Continue with Google
          </Button>

          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <span className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-background px-2 text-muted-foreground">
                Or continue with email
              </span>
            </div>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <p className="text-sm text-destructive text-center">{error}</p>
            )}
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? "Signing in..." : "Sign In"}
            </Button>
          </form>
        </CardContent>
        <CardFooter className="justify-center">
          <p className="text-sm text-muted-foreground">
            Don&apos;t have an account?{" "}
            <Link href="/register" className="underline">
              Sign up
            </Link>
          </p>
        </CardFooter>
      </Card>
    </main>
  );
}
```

- [ ] **Step 6: Create registration page**

Create `src/app/(auth)/register/page.tsx`:

```typescript
"use client";

import { useState } from "react";
import { signIn } from "next-auth/react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  CardFooter,
} from "@/components/ui/card";

export default function RegisterPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"teacher" | "student">("teacher");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");

    const res = await fetch("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, email, password, role }),
    });

    if (!res.ok) {
      const data = await res.json();
      setError(data.error || "Registration failed");
      setLoading(false);
      return;
    }

    // Auto sign-in after registration
    const result = await signIn("credentials", {
      email,
      password,
      redirect: false,
    });

    if (result?.error) {
      setError("Account created but sign-in failed. Please log in.");
      setLoading(false);
    } else {
      router.push("/dashboard");
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Create an Account</CardTitle>
          <CardDescription>
            Join Bridge to start teaching or learning
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Button
            variant="outline"
            className="w-full"
            onClick={() => signIn("google", { callbackUrl: "/dashboard" })}
          >
            Sign up with Google
          </Button>

          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <span className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-background px-2 text-muted-foreground">
                Or with email
              </span>
            </div>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <p className="text-sm text-destructive text-center">{error}</p>
            )}
            <div className="space-y-2">
              <Label htmlFor="name">Full Name</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={8}
                required
              />
            </div>
            <div className="space-y-2">
              <Label>I am a...</Label>
              <div className="flex gap-4">
                <Button
                  type="button"
                  variant={role === "teacher" ? "default" : "outline"}
                  className="flex-1"
                  onClick={() => setRole("teacher")}
                >
                  Teacher
                </Button>
                <Button
                  type="button"
                  variant={role === "student" ? "default" : "outline"}
                  className="flex-1"
                  onClick={() => setRole("student")}
                >
                  Student
                </Button>
              </div>
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? "Creating account..." : "Create Account"}
            </Button>
          </form>
        </CardContent>
        <CardFooter className="justify-center">
          <p className="text-sm text-muted-foreground">
            Already have an account?{" "}
            <Link href="/login" className="underline">
              Log in
            </Link>
          </p>
        </CardFooter>
      </Card>
    </main>
  );
}
```

- [ ] **Step 7: Commit**

```bash
git add src/app/ src/components/
git commit -m "feat: add UI shell with login and registration pages

Landing page, login with Google OAuth and credentials,
registration with role selection. shadcn/ui components."
```

---

## Task 7: UI Shell — Dashboard and Classroom Pages

**Files:**
- Create: `src/app/dashboard/layout.tsx`, `src/app/dashboard/page.tsx`, `src/app/dashboard/classrooms/page.tsx`, `src/app/dashboard/classrooms/new/page.tsx`, `src/app/dashboard/classrooms/join/page.tsx`, `src/app/dashboard/classrooms/[id]/page.tsx`

- [ ] **Step 1: Create dashboard layout with navigation**

Create `src/app/dashboard/layout.tsx`:

```typescript
import { redirect } from "next/navigation";
import { auth } from "@/lib/auth";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { SignOutButton } from "@/components/sign-out-button";

export default async function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const session = await auth();

  if (!session) {
    redirect("/login");
  }

  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b">
        <div className="container mx-auto flex items-center justify-between h-14 px-4">
          <div className="flex items-center gap-6">
            <Link href="/dashboard" className="font-bold text-lg">
              Bridge
            </Link>
            <nav className="flex gap-4">
              <Link
                href="/dashboard/classrooms"
                className="text-sm text-muted-foreground hover:text-foreground"
              >
                Classrooms
              </Link>
            </nav>
          </div>
          <div className="flex items-center gap-4">
            <span className="text-sm text-muted-foreground">
              {session.user.name} ({session.user.role})
            </span>
            <SignOutButton />
          </div>
        </div>
      </header>
      <main className="flex-1 container mx-auto p-4">{children}</main>
    </div>
  );
}
```

Create `src/components/sign-out-button.tsx`:

```typescript
"use client";

import { signOut } from "next-auth/react";
import { Button } from "@/components/ui/button";

export function SignOutButton() {
  return (
    <Button variant="ghost" size="sm" onClick={() => signOut({ callbackUrl: "/" })}>
      Sign Out
    </Button>
  );
}
```

- [ ] **Step 2: Create dashboard home page**

Create `src/app/dashboard/page.tsx`:

```typescript
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { listClassrooms } from "@/lib/classrooms";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default async function DashboardPage() {
  const session = await auth();
  const classroomList = await listClassrooms(db, session!.user.id);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <div className="flex gap-2">
          {session!.user.role === "teacher" && (
            <Button asChild>
              <Link href="/dashboard/classrooms/new">Create Classroom</Link>
            </Button>
          )}
          <Button variant="outline" asChild>
            <Link href="/dashboard/classrooms/join">Join Classroom</Link>
          </Button>
        </div>
      </div>

      {classroomList.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <p>No classrooms yet.</p>
            <p className="text-sm mt-2">
              {session!.user.role === "teacher"
                ? "Create your first classroom to get started."
                : "Ask your teacher for a join code."}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {classroomList.map((classroom) => (
            <Link key={classroom.id} href={`/dashboard/classrooms/${classroom.id}`}>
              <Card className="hover:border-primary transition-colors cursor-pointer">
                <CardHeader>
                  <CardTitle className="text-lg">{classroom.name}</CardTitle>
                  <CardDescription>
                    {classroom.gradeLevel} &middot; {classroom.editorMode}
                  </CardDescription>
                </CardHeader>
                {classroom.description && (
                  <CardContent>
                    <p className="text-sm text-muted-foreground line-clamp-2">
                      {classroom.description}
                    </p>
                  </CardContent>
                )}
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Create classrooms list page**

Create `src/app/dashboard/classrooms/page.tsx`:

```typescript
import { redirect } from "next/navigation";

// Classrooms list is the dashboard home — redirect there
export default function ClassroomsPage() {
  redirect("/dashboard");
}
```

- [ ] **Step 4: Create new classroom form**

Create `src/app/dashboard/classrooms/new/page.tsx`:

```typescript
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export default function NewClassroomPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [gradeLevel, setGradeLevel] = useState("6-8");
  const [editorMode, setEditorMode] = useState("python");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");

    const res = await fetch("/api/classrooms", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, description, gradeLevel, editorMode }),
    });

    if (!res.ok) {
      const data = await res.json();
      setError(data.error || "Failed to create classroom");
      setLoading(false);
      return;
    }

    const classroom = await res.json();
    router.push(`/dashboard/classrooms/${classroom.id}`);
  }

  return (
    <div className="max-w-lg mx-auto">
      <Card>
        <CardHeader>
          <CardTitle>Create a Classroom</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <p className="text-sm text-destructive">{error}</p>
            )}
            <div className="space-y-2">
              <Label htmlFor="name">Classroom Name</Label>
              <Input
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g., Intro to Python - Period 3"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="description">Description (optional)</Label>
              <Input
                id="description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Brief description of the class"
              />
            </div>
            <div className="space-y-2">
              <Label>Grade Level</Label>
              <Select value={gradeLevel} onValueChange={setGradeLevel}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="K-5">K-5 (Elementary)</SelectItem>
                  <SelectItem value="6-8">6-8 (Middle School)</SelectItem>
                  <SelectItem value="9-12">9-12 (High School)</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Default Editor</Label>
              <Select value={editorMode} onValueChange={setEditorMode}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="python">Python</SelectItem>
                  <SelectItem value="blockly" disabled>
                    Blockly (coming soon)
                  </SelectItem>
                  <SelectItem value="javascript" disabled>
                    JavaScript (coming soon)
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? "Creating..." : "Create Classroom"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 5: Add Select component from shadcn**

```bash
bunx shadcn@latest add select
```

- [ ] **Step 6: Create join classroom page**

Create `src/app/dashboard/classrooms/join/page.tsx`:

```typescript
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";

export default function JoinClassroomPage() {
  const router = useRouter();
  const [joinCode, setJoinCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");

    const res = await fetch("/api/classrooms/join", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ joinCode: joinCode.toUpperCase() }),
    });

    if (!res.ok) {
      const data = await res.json();
      setError(data.error || "Failed to join classroom");
      setLoading(false);
      return;
    }

    const classroom = await res.json();
    router.push(`/dashboard/classrooms/${classroom.id}`);
  }

  return (
    <div className="max-w-lg mx-auto">
      <Card>
        <CardHeader>
          <CardTitle>Join a Classroom</CardTitle>
          <CardDescription>
            Enter the code your teacher shared with you
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <p className="text-sm text-destructive">{error}</p>
            )}
            <div className="space-y-2">
              <Label htmlFor="joinCode">Join Code</Label>
              <Input
                id="joinCode"
                value={joinCode}
                onChange={(e) => setJoinCode(e.target.value.toUpperCase())}
                placeholder="e.g., ABC12345"
                maxLength={8}
                className="text-center text-2xl tracking-widest font-mono"
                required
              />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? "Joining..." : "Join Classroom"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 7: Create classroom detail page**

Create `src/app/dashboard/classrooms/[id]/page.tsx`:

```typescript
import { notFound } from "next/navigation";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import { getClassroom, getClassroomMembers } from "@/lib/classrooms";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default async function ClassroomDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const session = await auth();
  const { id } = await params;
  const classroom = await getClassroom(db, id);

  if (!classroom) {
    notFound();
  }

  const members = await getClassroomMembers(db, id);
  const isTeacher = classroom.teacherId === session!.user.id;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{classroom.name}</h1>
        <p className="text-muted-foreground">
          {classroom.gradeLevel} &middot; {classroom.editorMode}
        </p>
        {classroom.description && (
          <p className="mt-2">{classroom.description}</p>
        )}
      </div>

      {isTeacher && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Join Code</CardTitle>
            <CardDescription>Share this code with your students</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-mono tracking-widest font-bold text-center">
              {classroom.joinCode}
            </p>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">
            Students ({members.length})
          </CardTitle>
        </CardHeader>
        <CardContent>
          {members.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No students have joined yet.
            </p>
          ) : (
            <ul className="space-y-2">
              {members.map((member) => (
                <li
                  key={member.userId}
                  className="flex items-center justify-between py-2 border-b last:border-0"
                >
                  <span className="text-sm font-medium">{member.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {member.email}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 8: Verify the app builds**

```bash
bun run build
```

Expected: Build completes without errors.

- [ ] **Step 9: Commit**

```bash
git add src/app/dashboard/ src/components/sign-out-button.tsx
git commit -m "feat: add dashboard and classroom management UI

Dashboard with classroom list, create classroom form with grade
level and editor mode selection, join-by-code page, classroom
detail view showing join code and student roster."
```

---

## Task 8: Verify Full Test Suite and Final Cleanup

**Files:**
- Modify: `tests/setup.ts` (if needed)

- [ ] **Step 1: Run full test suite**

```bash
DATABASE_URL=postgresql://bridge:bridge@localhost:5432/bridge_test bun run test
```

Expected: All tests pass (smoke, schema, utils, DB integration, classrooms, join).

- [ ] **Step 2: Verify app starts**

```bash
bun run dev
```

Visit `http://localhost:3000`. Expected:
- Landing page loads with "Log In" and "Sign Up" buttons
- `/login` shows login form with Google OAuth button and email/password fields
- `/register` shows registration form with role selection
- `/dashboard` redirects to `/login` when not authenticated

- [ ] **Step 3: Delete smoke test**

Remove `tests/unit/smoke.test.ts` — it was only needed to verify the test setup.

```bash
rm tests/unit/smoke.test.ts
```

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "chore: clean up smoke test, verify full test suite passes"
```
