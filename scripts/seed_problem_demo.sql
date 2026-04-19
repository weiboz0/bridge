-- Demo seed for the Problem / Attempt workflow (plans 024–026).
--
-- Creates:
--   • 1 course (under Bridge Demo School, authored by eve@demo.edu)
--   • 2 topics (Warm-ups, Arrays)
--   • 4 problems with starter code + description
--   • Canonical test cases (examples + hidden)
--   • 1 class with eve as instructor, alice + bob enrolled as students
--   • A new_classrooms row so live sessions work
--
-- Idempotent: all rows use fixed UUIDs and are inserted with
-- ON CONFLICT DO NOTHING. Re-running is a no-op.
--
-- UUID scheme (all hex): the last 12 digits encode what the row is.
--   aa-prefix  = course
--   10XXX      = topic
--   20XXX      = problem
--   30PCCC     = test case, where P=problem index, C=case index
--   40XXX      = class
--   50XXX      = class_membership
--   60XXX      = new_classroom
--
-- Apply:
--   psql postgresql://work@127.0.0.1:5432/bridge -f scripts/seed_problem_demo.sql

BEGIN;

-- ---------- Course ----------

INSERT INTO courses (id, org_id, created_by, title, description, grade_level, language, is_published)
VALUES (
  '00000000-0000-0000-0000-0000000aa001',
  'd386983b-6da4-4cb8-8057-f2aa70d27c07',  -- Bridge Demo School
  'd0d3b031-a483-4214-97fb-48c9584f4dcb',  -- eve@demo.edu
  'Intro to Python — Problem Demo',
  'A small set of problems exercising the new Problem / Attempt workflow: input parsing, simple math, and a classic two-sum.',
  '9-12',
  'python',
  true
)
ON CONFLICT (id) DO NOTHING;

-- ---------- Topics ----------

INSERT INTO topics (id, course_id, title, description, sort_order, lesson_content)
VALUES
  (
    '00000000-0000-0000-0000-000000010001',
    '00000000-0000-0000-0000-0000000aa001',
    'Warm-ups',
    'Simple I/O: read from input(), print a result.',
    0,
    '{}'::jsonb
  ),
  (
    '00000000-0000-0000-0000-000000010002',
    '00000000-0000-0000-0000-0000000aa001',
    'Arrays',
    'Work with lists of numbers.',
    1,
    '{}'::jsonb
  )
ON CONFLICT (id) DO NOTHING;

-- ---------- Problems ----------

INSERT INTO problems (id, topic_id, title, description, starter_code, language, "order", created_by)
VALUES
  (
    '00000000-0000-0000-0000-000000020001',
    '00000000-0000-0000-0000-000000010001',
    'Hello, name',
    E'Read a name from standard input and greet that person.\n\n**Input:** a single line containing the name.\n\n**Output:** `Hello, {name}!`',
    E'name = input()\nprint(f"Hello, {name}!")\n',
    'python',
    0,
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000020002',
    '00000000-0000-0000-0000-000000010001',
    'Sum two numbers',
    E'Read two integers from standard input (one per line) and print their sum.\n\n**Input:**\n```\n3\n4\n```\n\n**Output:** `7`',
    E'a = int(input())\nb = int(input())\nprint(a + b)\n',
    'python',
    1,
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000020003',
    '00000000-0000-0000-0000-000000010002',
    'List average',
    E'Read a list of integers on a single line (space-separated) and print the average to one decimal place.\n\n**Input:** `4 2 7 11 15`\n\n**Output:** `7.8`',
    E'nums = list(map(int, input().split()))\nprint(f"{sum(nums)/len(nums):.1f}")\n',
    'python',
    0,
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000020004',
    '00000000-0000-0000-0000-000000010002',
    'Two Sum',
    E'Given a list of integers and a target number, return the indices of the two numbers that add up to the target.\n\n**Input:** two lines — the list of integers, then the target.\n\n**Output:** two indices separated by a space, in any order.\n\nExactly one solution is guaranteed per input; elements are not reused.',
    E'def solve(nums, target):\n    seen = {}\n    for i, n in enumerate(nums):\n        if target - n in seen:\n            return seen[target - n], i\n        seen[n] = i\n\nnums = list(map(int, input().split()))\ntarget = int(input())\na, b = solve(nums, target)\nprint(a, b)\n',
    'python',
    1,
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
ON CONFLICT (id) DO NOTHING;

-- ---------- Test cases ----------
-- Each problem gets canonical examples (is_example=true, student sees
-- them inline) plus hidden cases (is_example=false, graded server-side
-- but never disclosed).
-- Case UUID tail: 30PCCC where P = problem index (1–4), C = case index.

INSERT INTO test_cases (id, problem_id, owner_id, name, stdin, expected_stdout, is_example, "order")
VALUES
  -- Hello, name (P=1)
  ('00000000-0000-0000-0000-000000301001', '00000000-0000-0000-0000-000000020001', NULL, 'Example 1',           'Ada',              'Hello, Ada!',      true,  0),
  ('00000000-0000-0000-0000-000000301002', '00000000-0000-0000-0000-000000020001', NULL, 'Hidden: long name',   'Dijkstra',         'Hello, Dijkstra!', false, 1),
  -- Sum two numbers (P=2)
  ('00000000-0000-0000-0000-000000302001', '00000000-0000-0000-0000-000000020002', NULL, 'Example 1',           E'3\n4',             '7',                true,  0),
  ('00000000-0000-0000-0000-000000302002', '00000000-0000-0000-0000-000000020002', NULL, 'Example 2',           E'10\n-3',           '7',                true,  1),
  ('00000000-0000-0000-0000-000000302003', '00000000-0000-0000-0000-000000020002', NULL, 'Hidden: negatives',   E'-100\n50',         '-50',              false, 2),
  -- List average (P=3)
  ('00000000-0000-0000-0000-000000303001', '00000000-0000-0000-0000-000000020003', NULL, 'Example 1',           '4 2 7 11 15',       '7.8',              true,  0),
  ('00000000-0000-0000-0000-000000303002', '00000000-0000-0000-0000-000000020003', NULL, 'Hidden: singleton',   '42',                '42.0',             false, 1),
  ('00000000-0000-0000-0000-000000303003', '00000000-0000-0000-0000-000000020003', NULL, 'Hidden: negatives',   '-5 -3 -10',         '-6.0',             false, 2),
  -- Two Sum (P=4)
  ('00000000-0000-0000-0000-000000304001', '00000000-0000-0000-0000-000000020004', NULL, 'Example 1',           E'4 2 7 11 15\n9',   '0 1',              true,  0),
  ('00000000-0000-0000-0000-000000304002', '00000000-0000-0000-0000-000000020004', NULL, 'Example 2',           E'3 3 2 4\n6',       '0 1',              true,  1),
  ('00000000-0000-0000-0000-000000304003', '00000000-0000-0000-0000-000000020004', NULL, 'Hidden: negatives',   E'-3 -1 -4 -2\n-5',  '0 3',              false, 2),
  ('00000000-0000-0000-0000-000000304004', '00000000-0000-0000-0000-000000020004', NULL, 'Hidden: end of list', E'1 5 7 2 8 11\n19', '4 5',              false, 3)
ON CONFLICT (id) DO NOTHING;

-- ---------- Class + memberships + classroom ----------

INSERT INTO classes (id, course_id, org_id, title, term, join_code, status)
VALUES (
  '00000000-0000-0000-0000-000000040001',
  '00000000-0000-0000-0000-0000000aa001',
  'd386983b-6da4-4cb8-8057-f2aa70d27c07',
  'Problem Demo · Period 3',
  'Spring 2026',
  'DEMOP3AB',
  'active'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO class_memberships (id, class_id, user_id, role)
VALUES
  ('00000000-0000-0000-0000-000000050001', '00000000-0000-0000-0000-000000040001', 'd0d3b031-a483-4214-97fb-48c9584f4dcb', 'instructor'),  -- eve
  ('00000000-0000-0000-0000-000000050002', '00000000-0000-0000-0000-000000040001', '242fea26-1527-4a10-b208-af4cad1e1102', 'student'),     -- alice
  ('00000000-0000-0000-0000-000000050003', '00000000-0000-0000-0000-000000040001', '179aee9f-cce3-46f1-ac5f-f5cfbeb0531b', 'student')      -- bob
ON CONFLICT (id) DO NOTHING;

INSERT INTO new_classrooms (id, class_id, editor_mode)
VALUES ('00000000-0000-0000-0000-000000060001', '00000000-0000-0000-0000-000000040001', 'python')
ON CONFLICT (id) DO NOTHING;

COMMIT;

-- ---------- Summary ----------

\echo ''
\echo '=== Seed complete ==='
SELECT 'course' AS kind, id::text, title FROM courses WHERE id = '00000000-0000-0000-0000-0000000aa001'
UNION ALL SELECT 'class' AS kind, id::text, title FROM classes WHERE id = '00000000-0000-0000-0000-000000040001'
UNION ALL SELECT 'topic ' || sort_order::text, id::text, title FROM topics WHERE course_id = '00000000-0000-0000-0000-0000000aa001'
UNION ALL SELECT 'problem ' || "order"::text, id::text, title FROM problems WHERE topic_id IN (
  '00000000-0000-0000-0000-000000010001',
  '00000000-0000-0000-0000-000000010002'
)
ORDER BY kind;

\echo ''
\echo 'Open in browser (as alice@demo.edu):'
\echo '  /student/classes/00000000-0000-0000-0000-000000040001'
\echo '  /student/classes/00000000-0000-0000-0000-000000040001/problems/00000000-0000-0000-0000-000000020001'
\echo ''
\echo 'Teacher watch view (as eve@demo.edu, while alice is open in another browser):'
\echo '  /teacher/classes/00000000-0000-0000-0000-000000040001/problems/00000000-0000-0000-0000-000000020001/students/242fea26-1527-4a10-b208-af4cad1e1102'
