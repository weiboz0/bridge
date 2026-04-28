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
  boolean,
  integer,
  doublePrecision,
  primaryKey,
} from "drizzle-orm/pg-core";

// --- Enums ---

// Kept temporarily for migration compatibility — no table references it
export const userRoleEnum = pgEnum("user_role", [
  "admin",
  "teacher",
  "student",
]);

// Recorded at signup so onboarding can route the user without re-asking.
// Distinct from any role assigned by an org (orgMemberRoleEnum) — this
// is just "what the user said when they signed up." Nullable on users so
// existing rows and OAuth signups with no explicit answer remain valid.
export const signupIntentEnum = pgEnum("signup_intent", ["teacher", "student"]);

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

export const sessionStatusEnum = pgEnum("session_status", [
  "live",
  "ended",
]);

export const participantStatusEnum = pgEnum("participant_status", [
  "invited",
  "present",
  "left",
]);

export const annotationAuthorTypeEnum = pgEnum("annotation_author_type", [
  "teacher",
  "ai",
]);

export const orgTypeEnum = pgEnum("org_type", [
  "school",
  "tutoring_center",
  "bootcamp",
  "other",
]);

export const orgStatusEnum = pgEnum("org_status", [
  "pending",
  "active",
  "suspended",
]);

export const orgMemberRoleEnum = pgEnum("org_member_role", [
  "org_admin",
  "teacher",
  "student",
  "parent",
]);

export const orgMemberStatusEnum = pgEnum("org_member_status", [
  "pending",
  "active",
  "suspended",
]);

export const classStatusEnum = pgEnum("class_status", ["active", "archived"]);

export const classMemberRoleEnum = pgEnum("class_member_role", [
  "instructor",
  "ta",
  "student",
  "observer",
  "guest",
  "parent",
]);

export const programmingLanguageEnum = pgEnum("programming_language", [
  "python",
  "javascript",
  "blockly",
]);

// --- Tables ---

export const organizations = pgTable(
  "organizations",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    name: varchar("name", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }).notNull(),
    type: orgTypeEnum("type").notNull(),
    status: orgStatusEnum("status").notNull().default("pending"),
    contactEmail: varchar("contact_email", { length: 255 }).notNull(),
    contactName: varchar("contact_name", { length: 255 }).notNull(),
    domain: varchar("domain", { length: 255 }),
    settings: jsonb("settings").default({}),
    verifiedAt: timestamp("verified_at"),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("organizations_slug_idx").on(table.slug),
    index("organizations_status_idx").on(table.status),
  ]
);

export const users = pgTable(
  "users",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    name: varchar("name", { length: 255 }).notNull(),
    email: varchar("email", { length: 255 }).notNull(),
    avatarUrl: text("avatar_url"),
    passwordHash: text("password_hash"),
    isPlatformAdmin: boolean("is_platform_admin").notNull().default(false),
    intendedRole: signupIntentEnum("intended_role"),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [uniqueIndex("users_email_idx").on(table.email)]
);

export const orgMemberships = pgTable(
  "org_memberships",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: uuid("org_id")
      .notNull()
      .references(() => organizations.id, { onDelete: "cascade" }),
    userId: uuid("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    role: orgMemberRoleEnum("role").notNull(),
    status: orgMemberStatusEnum("status").notNull().default("pending"),
    invitedBy: uuid("invited_by").references(() => users.id),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("org_memberships_org_idx").on(table.orgId),
    index("org_memberships_user_idx").on(table.userId),
    uniqueIndex("org_memberships_org_user_role_idx").on(
      table.orgId,
      table.userId,
      table.role
    ),
  ]
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

export const sessions = pgTable(
  "sessions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    classId: uuid("class_id").references(() => classes.id, { onDelete: "cascade" }),
    teacherId: uuid("teacher_id")
      .notNull()
      .references(() => users.id),
    title: varchar("title", { length: 255 }).notNull(),
    inviteToken: varchar("invite_token", { length: 24 }),
    inviteExpiresAt: timestamp("invite_expires_at", { withTimezone: true }),
    scheduledSessionId: uuid("scheduled_session_id"),
    status: sessionStatusEnum("status").notNull().default("live"),
    settings: jsonb("settings").default({}),
    startedAt: timestamp("started_at").defaultNow().notNull(),
    endedAt: timestamp("ended_at"),
    createdAt: timestamp("created_at", { withTimezone: true }).defaultNow().notNull(),
    updatedAt: timestamp("updated_at", { withTimezone: true }).defaultNow().notNull(),
  },
  (table) => [
    index("sessions_class_idx").on(table.classId),
    index("sessions_class_status_idx").on(table.classId, table.status),
  ]
);

export const sessionParticipants = pgTable(
  "session_participants",
  {
    sessionId: uuid("session_id")
      .notNull()
      .references(() => sessions.id, { onDelete: "cascade" }),
    userId: uuid("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    status: participantStatusEnum("status").notNull().default("present"),
    joinedAt: timestamp("joined_at"),
    leftAt: timestamp("left_at"),
    invitedBy: uuid("invited_by").references(() => users.id, { onDelete: "set null" }),
    invitedAt: timestamp("invited_at", { withTimezone: true }),
    helpRequestedAt: timestamp("help_requested_at", { withTimezone: true }),
  },
  (table) => [
    uniqueIndex("session_participant_unique_idx").on(
      table.sessionId,
      table.userId
    ),
    index("session_participants_session_idx").on(table.sessionId),
  ]
);

export const aiInteractions = pgTable(
  "ai_interactions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    studentId: uuid("student_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    sessionId: uuid("session_id")
      .notNull()
      .references(() => sessions.id, { onDelete: "cascade" }),
    enabledByTeacherId: uuid("enabled_by_teacher_id")
      .notNull()
      .references(() => users.id),
    messages: jsonb("messages").default([]).notNull(),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("ai_interactions_session_idx").on(table.sessionId),
    index("ai_interactions_student_idx").on(table.studentId, table.sessionId),
  ]
);

export const codeAnnotations = pgTable(
  "code_annotations",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    documentId: varchar("document_id", { length: 255 }).notNull(),
    authorId: uuid("author_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    authorType: annotationAuthorTypeEnum("author_type").notNull(),
    lineStart: varchar("line_start", { length: 10 }).notNull(),
    lineEnd: varchar("line_end", { length: 10 }).notNull(),
    content: text("content").notNull(),
    resolved: timestamp("resolved_at"),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("code_annotations_document_idx").on(table.documentId),
  ]
);

// --- Course Hierarchy Tables ---

export const courses = pgTable(
  "courses",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    orgId: uuid("org_id")
      .notNull()
      .references(() => organizations.id, { onDelete: "cascade" }),
    createdBy: uuid("created_by")
      .notNull()
      .references(() => users.id),
    title: varchar("title", { length: 255 }).notNull(),
    description: text("description").default(""),
    gradeLevel: gradeLevelEnum("grade_level").notNull(),
    language: programmingLanguageEnum("language").notNull().default("python"),
    isPublished: boolean("is_published").notNull().default(false),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    index("courses_org_idx").on(table.orgId),
    index("courses_created_by_idx").on(table.createdBy),
  ]
);

export const topics = pgTable(
  "topics",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    courseId: uuid("course_id")
      .notNull()
      .references(() => courses.id, { onDelete: "cascade" }),
    title: varchar("title", { length: 255 }).notNull(),
    description: text("description").default(""),
    sortOrder: integer("sort_order").notNull().default(0),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    index("topics_course_idx").on(table.courseId),
    index("topics_sort_idx").on(table.courseId, table.sortOrder),
  ]
);

export const classes = pgTable(
  "classes",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    courseId: uuid("course_id")
      .notNull()
      .references(() => courses.id, { onDelete: "cascade" }),
    orgId: uuid("org_id")
      .notNull()
      .references(() => organizations.id, { onDelete: "cascade" }),
    title: varchar("title", { length: 255 }).notNull(),
    term: varchar("term", { length: 100 }).default(""),
    joinCode: varchar("join_code", { length: 10 }).notNull(),
    status: classStatusEnum("status").notNull().default("active"),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("classes_join_code_idx").on(table.joinCode),
    index("classes_course_idx").on(table.courseId),
    index("classes_org_idx").on(table.orgId),
  ]
);

export const classMemberships = pgTable(
  "class_memberships",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    classId: uuid("class_id")
      .notNull()
      .references(() => classes.id, { onDelete: "cascade" }),
    userId: uuid("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    role: classMemberRoleEnum("role").notNull().default("student"),
    joinedAt: timestamp("joined_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("class_membership_unique_idx").on(table.classId, table.userId),
    index("class_memberships_class_idx").on(table.classId),
    index("class_memberships_user_idx").on(table.userId),
  ]
);

export const classSettings = pgTable(
  "class_settings",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    classId: uuid("class_id")
      .notNull()
      .references(() => classes.id, { onDelete: "cascade" })
      .unique(),
    editorMode: editorModeEnum("editor_mode").notNull().default("python"),
    settings: jsonb("settings").default({}),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("class_settings_class_idx").on(table.classId),
  ]
);

export const sessionTopics = pgTable(
  "session_topics",
  {
    sessionId: uuid("session_id")
      .notNull()
      .references(() => sessions.id, { onDelete: "cascade" }),
    topicId: uuid("topic_id")
      .notNull()
      .references(() => topics.id, { onDelete: "cascade" }),
  },
  (table) => [
    uniqueIndex("session_topic_unique_idx").on(table.sessionId, table.topicId),
  ]
);

// --- Documents ---

export const documents = pgTable(
  "documents",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    ownerId: uuid("owner_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    /** @deprecated Legacy column name — use classId in application code. Rename requires migration. */
    classroomId: uuid("classroom_id"),
    sessionId: uuid("session_id"),
    topicId: uuid("topic_id"),
    language: programmingLanguageEnum("language").notNull().default("python"),
    yjsState: text("yjs_state"),
    plainText: text("plain_text").default(""),
    createdAt: timestamp("created_at").defaultNow().notNull(),
    updatedAt: timestamp("updated_at").defaultNow().notNull(),
  },
  (table) => [
    index("documents_owner_idx").on(table.ownerId),
    index("documents_classroom_idx").on(table.classroomId),
    index("documents_session_idx").on(table.sessionId),
  ]
);

// --- Assignments ---

export const assignments = pgTable(
  "assignments",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    topicId: uuid("topic_id").references(() => topics.id, { onDelete: "cascade" }),
    classId: uuid("class_id")
      .notNull()
      .references(() => classes.id, { onDelete: "cascade" }),
    title: varchar("title", { length: 255 }).notNull(),
    description: text("description").default(""),
    starterCode: text("starter_code"),
    dueDate: timestamp("due_date"),
    rubric: jsonb("rubric").default({}),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("assignments_class_idx").on(table.classId),
    index("assignments_topic_idx").on(table.topicId),
  ]
);

export const submissions = pgTable(
  "submissions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    assignmentId: uuid("assignment_id")
      .notNull()
      .references(() => assignments.id, { onDelete: "cascade" }),
    studentId: uuid("student_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    documentId: uuid("document_id"),
    grade: doublePrecision("grade"),
    feedback: text("feedback"),
    submittedAt: timestamp("submitted_at").defaultNow().notNull(),
  },
  (table) => [
    uniqueIndex("submission_unique_idx").on(table.assignmentId, table.studentId),
    index("submissions_assignment_idx").on(table.assignmentId),
    index("submissions_student_idx").on(table.studentId),
  ]
);

// --- Problem Bank ---

// NOTE: 0013 creates these as varchar(16) CHECK'd columns, not true PG enums,
// to keep the migration additive. We expose Drizzle enums purely for TS
// type-narrowing — the DB column remains varchar. If the codebase later
// converts to true enums, keep the TS names aligned.
export const problemScopeEnum = pgEnum("problem_scope", [
  "platform",
  "org",
  "personal",
]);
export const problemDifficultyEnum = pgEnum("problem_difficulty", [
  "easy",
  "medium",
  "hard",
]);
export const problemStatusEnum = pgEnum("problem_status", [
  "draft",
  "published",
  "archived",
]);

export const problems = pgTable(
  "problems",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    scope: varchar("scope", { length: 16 })
      .$type<"platform" | "org" | "personal">()
      .notNull(),
    scopeId: uuid("scope_id"),
    title: varchar("title", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }),
    description: text("description").notNull(),
    starterCode: jsonb("starter_code")
      .$type<Record<string, string>>()
      .notNull()
      .default({}),
    difficulty: varchar("difficulty", { length: 16 })
      .$type<"easy" | "medium" | "hard">()
      .notNull()
      .default("easy"),
    gradeLevel: varchar("grade_level", { length: 8 }).$type<
      "K-5" | "6-8" | "9-12" | null
    >(),
    tags: text("tags").array().notNull().default([]),
    status: varchar("status", { length: 16 })
      .$type<"draft" | "published" | "archived">()
      .notNull()
      .default("draft"),
    forkedFrom: uuid("forked_from"),
    timeLimitMs: integer("time_limit_ms"),
    memoryLimitMb: integer("memory_limit_mb"),
    createdBy: uuid("created_by")
      .notNull()
      .references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
  },
  (t) => ({
    scopeStatusIdx: index("problems_scope_scope_id_status_idx").on(
      t.scope,
      t.scopeId,
      t.status
    ),
    createdByIdx: index("problems_created_by_idx").on(t.createdBy),
  })
);

export const topicProblems = pgTable(
  "topic_problems",
  {
    topicId: uuid("topic_id")
      .notNull()
      .references(() => topics.id, { onDelete: "cascade" }),
    problemId: uuid("problem_id")
      .notNull()
      .references(() => problems.id, { onDelete: "cascade" }),
    sortOrder: integer("sort_order").notNull().default(0),
    attachedBy: uuid("attached_by")
      .notNull()
      .references(() => users.id),
    attachedAt: timestamp("attached_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
  },
  (t) => ({
    pk: primaryKey({ columns: [t.topicId, t.problemId] }),
    problemIdx: index("topic_problems_problem_idx").on(t.problemId),
  })
);

export const problemSolutions = pgTable(
  "problem_solutions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    problemId: uuid("problem_id")
      .notNull()
      .references(() => problems.id, { onDelete: "cascade" }),
    language: varchar("language", { length: 32 }).notNull(),
    title: varchar("title", { length: 120 }),
    code: text("code").notNull(),
    notes: text("notes"),
    approachTags: text("approach_tags").array().notNull().default([]),
    isPublished: boolean("is_published").notNull().default(false),
    createdBy: uuid("created_by")
      .notNull()
      .references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
  },
  (t) => ({
    problemLangIdx: index("problem_solutions_problem_language_idx").on(
      t.problemId,
      t.language
    ),
  })
);

export const teachingUnits = pgTable(
  "teaching_units",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    scope: varchar("scope", { length: 16 })
      .$type<"platform" | "org" | "personal">()
      .notNull(),
    scopeId: uuid("scope_id"),
    title: varchar("title", { length: 255 }).notNull(),
    slug: varchar("slug", { length: 255 }),
    summary: text("summary").notNull().default(""),
    gradeLevel: varchar("grade_level", { length: 8 }).$type<
      "K-5" | "6-8" | "9-12" | null
    >(),
    subjectTags: text("subject_tags").array().notNull().default([]),
    standardsTags: text("standards_tags").array().notNull().default([]),
    estimatedMinutes: integer("estimated_minutes"),
    materialType: varchar("material_type", { length: 16 })
      .$type<"notes" | "slides" | "worksheet" | "reference">()
      .notNull()
      .default("notes"),
    status: varchar("status", { length: 24 })
      .$type<"draft" | "reviewed" | "classroom_ready" | "coach_ready" | "archived">()
      .notNull()
      .default("draft"),
    topicId: uuid("topic_id").references(() => topics.id, { onDelete: "set null" }),
    createdBy: uuid("created_by").notNull().references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
  },
  (t) => ({
    scopeStatusIdx: index("teaching_units_scope_scope_id_status_idx").on(
      t.scope,
      t.scopeId,
      t.status
    ),
    createdByIdx: index("teaching_units_created_by_idx").on(t.createdBy),
  })
);

export const unitDocuments = pgTable("unit_documents", {
  unitId: uuid("unit_id")
    .primaryKey()
    .references(() => teachingUnits.id, { onDelete: "cascade" }),
  blocks: jsonb("blocks")
    .$type<Record<string, unknown>>()
    .notNull()
    .default({ type: "doc", content: [] }),
  updatedAt: timestamp("updated_at", { withTimezone: true })
    .notNull()
    .defaultNow(),
});

export const unitRevisions = pgTable(
  "unit_revisions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    unitId: uuid("unit_id")
      .notNull()
      .references(() => teachingUnits.id, { onDelete: "cascade" }),
    blocks: jsonb("blocks").$type<Record<string, unknown>>().notNull(),
    reason: varchar("reason", { length: 255 }),
    createdBy: uuid("created_by").notNull().references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
  },
  (t) => ({
    unitCreatedIdx: index("unit_revisions_unit_created_idx").on(
      t.unitId,
      t.createdAt
    ),
  })
);

export const unitOverlays = pgTable("unit_overlays", {
  childUnitId: uuid("child_unit_id").primaryKey().references(() => teachingUnits.id, { onDelete: "cascade" }),
  parentUnitId: uuid("parent_unit_id").notNull().references(() => teachingUnits.id, { onDelete: "cascade" }),
  parentRevisionId: uuid("parent_revision_id").references(() => unitRevisions.id, { onDelete: "set null" }),
  blockOverrides: jsonb("block_overrides").$type<Record<string, { action: string; block?: unknown }>>().notNull().default({}),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull().defaultNow(),
}, (t) => ({
  parentIdx: index("unit_overlays_parent_idx").on(t.parentUnitId),
}));

// --- Unit Collections ---

export const unitCollections = pgTable(
  "unit_collections",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    scope: varchar("scope", { length: 16 })
      .$type<"platform" | "org" | "personal">()
      .notNull(),
    scopeId: uuid("scope_id"),
    title: varchar("title", { length: 255 }).notNull(),
    description: text("description").notNull().default(""),
    createdBy: uuid("created_by")
      .notNull()
      .references(() => users.id),
    createdAt: timestamp("created_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
    updatedAt: timestamp("updated_at", { withTimezone: true })
      .notNull()
      .defaultNow(),
  },
  (t) => ({
    scopeIdx: index("unit_collections_scope_idx").on(t.scope, t.scopeId),
  })
);

export const unitCollectionItems = pgTable(
  "unit_collection_items",
  {
    collectionId: uuid("collection_id")
      .notNull()
      .references(() => unitCollections.id, { onDelete: "cascade" }),
    unitId: uuid("unit_id")
      .notNull()
      .references(() => teachingUnits.id, { onDelete: "cascade" }),
    sortOrder: integer("sort_order").notNull().default(0),
  },
  (t) => ({
    pk: primaryKey({ columns: [t.collectionId, t.unitId] }),
  })
);

// --- Parent Reports ---

export const parentReports = pgTable(
  "parent_reports",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    studentId: uuid("student_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    generatedBy: uuid("generated_by")
      .notNull()
      .references(() => users.id),
    periodStart: timestamp("period_start").notNull(),
    periodEnd: timestamp("period_end").notNull(),
    content: text("content").notNull(),
    summary: jsonb("summary").default({}),
    createdAt: timestamp("created_at").defaultNow().notNull(),
  },
  (table) => [
    index("parent_reports_student_idx").on(table.studentId),
    index("parent_reports_period_idx").on(table.studentId, table.periodStart),
  ]
);
