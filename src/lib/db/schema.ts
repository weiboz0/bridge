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
} from "drizzle-orm/pg-core";

// --- Enums ---

// Kept temporarily for migration compatibility — no table references it
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

export const sessionStatusEnum = pgEnum("session_status", [
  "active",
  "ended",
]);

export const participantStatusEnum = pgEnum("participant_status", [
  "active",
  "idle",
  "needs_help",
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

export const classrooms = pgTable(
  "classrooms",
  {
    id: uuid("id").primaryKey().defaultRandom(),
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

export const liveSessions = pgTable(
  "live_sessions",
  {
    id: uuid("id").primaryKey().defaultRandom(),
    classroomId: uuid("classroom_id")
      .notNull()
      .references(() => classrooms.id, { onDelete: "cascade" }),
    teacherId: uuid("teacher_id")
      .notNull()
      .references(() => users.id),
    status: sessionStatusEnum("status").notNull().default("active"),
    settings: jsonb("settings").default({}),
    startedAt: timestamp("started_at").defaultNow().notNull(),
    endedAt: timestamp("ended_at"),
  },
  (table) => [
    index("live_sessions_classroom_idx").on(table.classroomId),
    index("live_sessions_status_idx").on(table.classroomId, table.status),
  ]
);

export const sessionParticipants = pgTable(
  "session_participants",
  {
    sessionId: uuid("session_id")
      .notNull()
      .references(() => liveSessions.id, { onDelete: "cascade" }),
    studentId: uuid("student_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    status: participantStatusEnum("status").notNull().default("active"),
    joinedAt: timestamp("joined_at").defaultNow().notNull(),
    leftAt: timestamp("left_at"),
  },
  (table) => [
    uniqueIndex("session_participant_unique_idx").on(
      table.sessionId,
      table.studentId
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
      .references(() => liveSessions.id, { onDelete: "cascade" }),
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
    lessonContent: jsonb("lesson_content").default({}),
    starterCode: text("starter_code"),
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

export const newClassrooms = pgTable(
  "new_classrooms",
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
    uniqueIndex("new_classrooms_class_idx").on(table.classId),
  ]
);

export const sessionTopics = pgTable(
  "session_topics",
  {
    sessionId: uuid("session_id")
      .notNull()
      .references(() => liveSessions.id, { onDelete: "cascade" }),
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
