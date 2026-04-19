-- Python 101 demo course — a full teachable unit exercising the new
-- Problem / Attempt / Test workflow (plans 024–026).
--
-- Structure:
--   Course: Python 101 — Introduction to Programming
--   6 topics, 12 problems, ~50 test cases (examples + hidden).
--
-- Problems are sequenced from simplest (print a line) to hardest
-- (prime-check, FizzBuzz, palindrome). Each has at least 2 example
-- cases (visible to students) and 1–3 hidden cases (graded server-
-- side; students see only pass/fail).
--
-- Idempotent: deterministic UUIDs + ON CONFLICT DO NOTHING. Re-runs
-- are a no-op.
--
-- UUID scheme (all hex, last 12 digits encode kind):
--   0aa000000001   = course
--   11P000         = topic (P = 1..6)
--   22PPNN         = problem (PP = topic, NN = problem index)
--   33PPNNCC       = test case (CC = case index within problem)
--   40CC00         = class / membership / classroom helpers
--
-- Apply:
--   psql postgresql://work@127.0.0.1:5432/bridge -f scripts/seed_python_101.sql

BEGIN;

-- =========================================================
-- Course
-- =========================================================

INSERT INTO courses (id, org_id, created_by, title, description, grade_level, language, is_published)
VALUES (
  '00000000-0000-0000-0000-00000aa00001',
  'd386983b-6da4-4cb8-8057-f2aa70d27c07',  -- Bridge Demo School
  'd0d3b031-a483-4214-97fb-48c9584f4dcb',  -- eve@demo.edu
  'Python 101 — Introduction to Programming',
  'A first course in Python. You''ll learn variables, input/output, arithmetic, branching, loops, and lists by solving small problems that build on each other.',
  '9-12',
  'python',
  true
)
ON CONFLICT (id) DO NOTHING;

-- =========================================================
-- Topics
-- =========================================================

INSERT INTO topics (id, course_id, title, description, sort_order, lesson_content)
VALUES
  ('00000000-0000-0000-0000-000000110001', '00000000-0000-0000-0000-00000aa00001', '1. Hello, World',            'Your first program: printing output.',                                    0, '{}'::jsonb),
  ('00000000-0000-0000-0000-000000110002', '00000000-0000-0000-0000-00000aa00001', '2. Variables & Input',       'Store values in variables; read from the user.',                          1, '{}'::jsonb),
  ('00000000-0000-0000-0000-000000110003', '00000000-0000-0000-0000-00000aa00001', '3. Numbers & Arithmetic',    'Integers, floats, and the four basic operators.',                         2, '{}'::jsonb),
  ('00000000-0000-0000-0000-000000110004', '00000000-0000-0000-0000-00000aa00001',        '4. Conditionals',     'if / elif / else — make decisions.',                                      3, '{}'::jsonb),
  ('00000000-0000-0000-0000-000000110005', '00000000-0000-0000-0000-00000aa00001', '5. Loops',                   'for and while — repeat until done.',                                      4, '{}'::jsonb),
  ('00000000-0000-0000-0000-000000110006', '00000000-0000-0000-0000-00000aa00001', '6. Lists',                   'Collections of values; index, slice, iterate.',                           5, '{}'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- =========================================================
-- Problems
-- =========================================================
-- Starter code uses dollar-quoting ($py$...$py$) so we can include
-- newlines, backticks, and quotes without escaping gymnastics.
-- Description uses $md$...$md$.

-- ---- Topic 1: Hello, World ----

INSERT INTO problems (id, topic_id, title, description, starter_code, language, "order", created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220101',
    '00000000-0000-0000-0000-000000110001',
    'Hello, World',
    $md$Print the exact text:

```
Hello, World!
```

Use the `print` function. No input.$md$,
    $py$print("Hello, World!")
$py$,
    'python', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220102',
    '00000000-0000-0000-0000-000000110001',
    'Three Lines',
    $md$Print three lines exactly:

```
line 1
line 2
line 3
```
$md$,
    $py$# Print each line on its own line.
$py$,
    'python', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 2: Variables & Input ----

INSERT INTO problems (id, topic_id, title, description, starter_code, language, "order", created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220201',
    '00000000-0000-0000-0000-000000110002',
    'Greet by Name',
    $md$Read a single name from input and greet that person.

**Input:** one line with the name.

**Output:** `Hello, {name}!` (exactly, with the exclamation mark).$md$,
    $py$name = input()
print(f"Hello, {name}!")
$py$,
    'python', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220202',
    '00000000-0000-0000-0000-000000110002',
    'Name Then Age',
    $md$Read a name and an age (each on their own line). Print a sentence:

```
{name} is {age} years old.
```

**Input:**
```
Ada
23
```

**Output:**
```
Ada is 23 years old.
```
$md$,
    $py$name = input()
age = input()
# Use an f-string to combine them into one sentence.
$py$,
    'python', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 3: Numbers & Arithmetic ----

INSERT INTO problems (id, topic_id, title, description, starter_code, language, "order", created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220301',
    '00000000-0000-0000-0000-000000110003',
    'Sum Two Numbers',
    $md$Read two integers (one per line) and print their sum.

**Input:**
```
3
4
```

**Output:**
```
7
```
$md$,
    $py$a = int(input())
b = int(input())
print(a + b)
$py$,
    'python', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220302',
    '00000000-0000-0000-0000-000000110003',
    'Area of a Rectangle',
    $md$Read a width and height (integers, one per line). Print the area.

Inputs will always be integers, so the area is an integer too.

**Input:**
```
3
5
```

**Output:**
```
15
```
$md$,
    $py$w = int(input())
h = int(input())
# Multiply and print.
$py$,
    'python', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 4: Conditionals ----

INSERT INTO problems (id, topic_id, title, description, starter_code, language, "order", created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220401',
    '00000000-0000-0000-0000-000000110004',
    'Even or Odd',
    $md$Read an integer and print `Even` if it's even, `Odd` if it's odd.

**Example 1 — Input:** `4` → **Output:** `Even`

**Example 2 — Input:** `7` → **Output:** `Odd`

Watch your capitalization — the output is case-sensitive.$md$,
    $py$n = int(input())
# Hint: use n % 2
$py$,
    'python', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220402',
    '00000000-0000-0000-0000-000000110004',
    'Pass or Fail',
    $md$Grading: a student passes if their score is 60 or higher. Read a score (integer 0–100) and print `Pass` or `Fail`.$md$,
    $py$score = int(input())
if score >= 60:
    print("Pass")
else:
    print("Fail")
$py$,
    'python', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 5: Loops ----

INSERT INTO problems (id, topic_id, title, description, starter_code, language, "order", created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220501',
    '00000000-0000-0000-0000-000000110005',
    'Count to N',
    $md$Read a positive integer N. Print the integers 1 through N, each on its own line.

**Input:** `4`

**Output:**
```
1
2
3
4
```
$md$,
    $py$n = int(input())
for i in range(1, n + 1):
    print(i)
$py$,
    'python', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220502',
    '00000000-0000-0000-0000-000000110005',
    'FizzBuzz (short)',
    $md$Read a positive integer N. For each integer from 1 to N, print:

- `Fizz` if it's divisible by 3
- `Buzz` if it's divisible by 5
- `FizzBuzz` if it's divisible by both
- Otherwise print the number itself

Each value goes on its own line. Classic interview question.$md$,
    $py$n = int(input())
for i in range(1, n + 1):
    # Decide what to print for i.
    pass
$py$,
    'python', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 6: Lists ----

INSERT INTO problems (id, topic_id, title, description, starter_code, language, "order", created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220601',
    '00000000-0000-0000-0000-000000110006',
    'Sum of a List',
    $md$Read one line of space-separated integers. Print their sum.

**Input:** `3 1 4 1 5 9 2 6`

**Output:** `31`$md$,
    $py$nums = list(map(int, input().split()))
print(sum(nums))
$py$,
    'python', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220602',
    '00000000-0000-0000-0000-000000110006',
    'Max in a List',
    $md$Read one line of space-separated integers and print the largest one.

**Input:** `4 -2 11 7`

**Output:** `11`

Don't use Python's `max` — practice writing the loop yourself.$md$,
    $py$nums = list(map(int, input().split()))
# Walk through nums and track the largest value seen.
$py$,
    'python', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- =========================================================
-- Test cases
-- Each case:    canonical (owner_id NULL)
--               is_example = true  -> student sees inline
--               is_example = false -> hidden, graded only
-- Case UUID: 33PPNNCC where PP = topic (01..06), NN = problem, CC = case.
-- =========================================================

INSERT INTO test_cases (id, problem_id, owner_id, name, stdin, expected_stdout, is_example, "order") VALUES
  -- 1.1 Hello, World
  ('00000000-0000-0000-0000-000033010101', '00000000-0000-0000-0000-000000220101', NULL, 'Example',              '',                 'Hello, World!',                    true,  0),
  ('00000000-0000-0000-0000-000033010102', '00000000-0000-0000-0000-000000220101', NULL, 'Hidden: exact match',  '',                 'Hello, World!',                    false, 1),

  -- 1.2 Three Lines
  ('00000000-0000-0000-0000-000033010201', '00000000-0000-0000-0000-000000220102', NULL, 'Example',              '',                 E'line 1\nline 2\nline 3',          true,  0),

  -- 2.1 Greet by Name
  ('00000000-0000-0000-0000-000033020101', '00000000-0000-0000-0000-000000220201', NULL, 'Example 1',            'Ada',              'Hello, Ada!',                      true,  0),
  ('00000000-0000-0000-0000-000033020102', '00000000-0000-0000-0000-000000220201', NULL, 'Example 2',            'Grace Hopper',     'Hello, Grace Hopper!',             true,  1),
  ('00000000-0000-0000-0000-000033020103', '00000000-0000-0000-0000-000000220201', NULL, 'Hidden: emoji',        'Ada 💡',           'Hello, Ada 💡!',                   false, 2),
  ('00000000-0000-0000-0000-000033020104', '00000000-0000-0000-0000-000000220201', NULL, 'Hidden: single char',  'X',                'Hello, X!',                        false, 3),

  -- 2.2 Name Then Age
  ('00000000-0000-0000-0000-000033020201', '00000000-0000-0000-0000-000000220202', NULL, 'Example',              E'Ada\n23',         'Ada is 23 years old.',             true,  0),
  ('00000000-0000-0000-0000-000033020202', '00000000-0000-0000-0000-000000220202', NULL, 'Hidden: older',        E'Grace\n85',       'Grace is 85 years old.',           false, 1),
  ('00000000-0000-0000-0000-000033020203', '00000000-0000-0000-0000-000000220202', NULL, 'Hidden: zero',         E'Baby\n0',         'Baby is 0 years old.',             false, 2),

  -- 3.1 Sum Two Numbers
  ('00000000-0000-0000-0000-000033030101', '00000000-0000-0000-0000-000000220301', NULL, 'Example 1',            E'3\n4',            '7',                                true,  0),
  ('00000000-0000-0000-0000-000033030102', '00000000-0000-0000-0000-000000220301', NULL, 'Example 2',            E'10\n-3',          '7',                                true,  1),
  ('00000000-0000-0000-0000-000033030103', '00000000-0000-0000-0000-000000220301', NULL, 'Hidden: negatives',    E'-100\n50',        '-50',                              false, 2),
  ('00000000-0000-0000-0000-000033030104', '00000000-0000-0000-0000-000000220301', NULL, 'Hidden: zero',         E'0\n0',            '0',                                false, 3),
  ('00000000-0000-0000-0000-000033030105', '00000000-0000-0000-0000-000000220301', NULL, 'Hidden: big',          E'999999\n1',       '1000000',                          false, 4),

  -- 3.2 Area of a Rectangle
  ('00000000-0000-0000-0000-000033030201', '00000000-0000-0000-0000-000000220302', NULL, 'Example',              E'3\n5',            '15',                               true,  0),
  ('00000000-0000-0000-0000-000033030202', '00000000-0000-0000-0000-000000220302', NULL, 'Hidden: square',       E'7\n7',            '49',                               false, 1),
  ('00000000-0000-0000-0000-000033030203', '00000000-0000-0000-0000-000000220302', NULL, 'Hidden: one dimension', E'1\n100',         '100',                              false, 2),

  -- 4.1 Even or Odd
  ('00000000-0000-0000-0000-000033040101', '00000000-0000-0000-0000-000000220401', NULL, 'Example 1 (even)',     '4',                'Even',                             true,  0),
  ('00000000-0000-0000-0000-000033040102', '00000000-0000-0000-0000-000000220401', NULL, 'Example 2 (odd)',      '7',                'Odd',                              true,  1),
  ('00000000-0000-0000-0000-000033040103', '00000000-0000-0000-0000-000000220401', NULL, 'Hidden: zero is even', '0',                'Even',                             false, 2),
  ('00000000-0000-0000-0000-000033040104', '00000000-0000-0000-0000-000000220401', NULL, 'Hidden: negative odd', '-5',               'Odd',                              false, 3),

  -- 4.2 Pass or Fail
  ('00000000-0000-0000-0000-000033040201', '00000000-0000-0000-0000-000000220402', NULL, 'Example (pass)',       '75',               'Pass',                             true,  0),
  ('00000000-0000-0000-0000-000033040202', '00000000-0000-0000-0000-000000220402', NULL, 'Example (fail)',       '45',               'Fail',                             true,  1),
  ('00000000-0000-0000-0000-000033040203', '00000000-0000-0000-0000-000000220402', NULL, 'Hidden: exactly 60',   '60',               'Pass',                             false, 2),
  ('00000000-0000-0000-0000-000033040204', '00000000-0000-0000-0000-000000220402', NULL, 'Hidden: zero',         '0',                'Fail',                             false, 3),
  ('00000000-0000-0000-0000-000033040205', '00000000-0000-0000-0000-000000220402', NULL, 'Hidden: perfect',      '100',              'Pass',                             false, 4),

  -- 5.1 Count to N
  ('00000000-0000-0000-0000-000033050101', '00000000-0000-0000-0000-000000220501', NULL, 'Example',              '4',                E'1\n2\n3\n4',                      true,  0),
  ('00000000-0000-0000-0000-000033050102', '00000000-0000-0000-0000-000000220501', NULL, 'Hidden: one',          '1',                '1',                                false, 1),
  ('00000000-0000-0000-0000-000033050103', '00000000-0000-0000-0000-000000220501', NULL, 'Hidden: ten',          '10',               E'1\n2\n3\n4\n5\n6\n7\n8\n9\n10',   false, 2),

  -- 5.2 FizzBuzz
  ('00000000-0000-0000-0000-000033050201', '00000000-0000-0000-0000-000000220502', NULL, 'Example N=5',          '5',                E'1\n2\nFizz\n4\nBuzz',             true,  0),
  ('00000000-0000-0000-0000-000033050202', '00000000-0000-0000-0000-000000220502', NULL, 'Example N=15',         '15',               E'1\n2\nFizz\n4\nBuzz\nFizz\n7\n8\nFizz\nBuzz\n11\nFizz\n13\n14\nFizzBuzz', true, 1),
  ('00000000-0000-0000-0000-000033050203', '00000000-0000-0000-0000-000000220502', NULL, 'Hidden: N=3',          '3',                E'1\n2\nFizz',                      false, 2),
  ('00000000-0000-0000-0000-000033050204', '00000000-0000-0000-0000-000000220502', NULL, 'Hidden: N=1',          '1',                '1',                                false, 3),

  -- 6.1 Sum of a List
  ('00000000-0000-0000-0000-000033060101', '00000000-0000-0000-0000-000000220601', NULL, 'Example',              '3 1 4 1 5 9 2 6',  '31',                               true,  0),
  ('00000000-0000-0000-0000-000033060102', '00000000-0000-0000-0000-000000220601', NULL, 'Hidden: singleton',    '42',               '42',                               false, 1),
  ('00000000-0000-0000-0000-000033060103', '00000000-0000-0000-0000-000000220601', NULL, 'Hidden: negatives',    '5 -3 -2',          '0',                                false, 2),
  ('00000000-0000-0000-0000-000033060104', '00000000-0000-0000-0000-000000220601', NULL, 'Hidden: all zeros',    '0 0 0',            '0',                                false, 3),

  -- 6.2 Max in a List
  ('00000000-0000-0000-0000-000033060201', '00000000-0000-0000-0000-000000220602', NULL, 'Example',              '4 -2 11 7',        '11',                               true,  0),
  ('00000000-0000-0000-0000-000033060202', '00000000-0000-0000-0000-000000220602', NULL, 'Hidden: all negative', '-3 -1 -4 -2',      '-1',                               false, 1),
  ('00000000-0000-0000-0000-000033060203', '00000000-0000-0000-0000-000000220602', NULL, 'Hidden: singleton',    '42',               '42',                               false, 2),
  ('00000000-0000-0000-0000-000033060204', '00000000-0000-0000-0000-000000220602', NULL, 'Hidden: duplicates',   '5 5 5 5',          '5',                                false, 3)
ON CONFLICT (id) DO NOTHING;

-- =========================================================
-- Class + memberships + classroom
-- =========================================================

INSERT INTO classes (id, course_id, org_id, title, term, join_code, status)
VALUES (
  '00000000-0000-0000-0000-000000400101',
  '00000000-0000-0000-0000-00000aa00001',
  'd386983b-6da4-4cb8-8057-f2aa70d27c07',
  'Python 101 · Period 3',
  'Spring 2026',
  'PY101P3X',
  'active'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO class_memberships (id, class_id, user_id, role)
VALUES
  ('00000000-0000-0000-0000-000000400201', '00000000-0000-0000-0000-000000400101', 'd0d3b031-a483-4214-97fb-48c9584f4dcb', 'instructor'),  -- eve
  ('00000000-0000-0000-0000-000000400202', '00000000-0000-0000-0000-000000400101', '242fea26-1527-4a10-b208-af4cad1e1102', 'student'),     -- alice
  ('00000000-0000-0000-0000-000000400203', '00000000-0000-0000-0000-000000400101', '179aee9f-cce3-46f1-ac5f-f5cfbeb0531b', 'student'),     -- bob
  ('00000000-0000-0000-0000-000000400204', '00000000-0000-0000-0000-000000400101', 'bca1b87e-2c20-45d0-9b7a-cd35d9efef36', 'student')      -- charlie
ON CONFLICT (id) DO NOTHING;

INSERT INTO new_classrooms (id, class_id, editor_mode)
VALUES ('00000000-0000-0000-0000-000000400301', '00000000-0000-0000-0000-000000400101', 'python')
ON CONFLICT (id) DO NOTHING;

COMMIT;

-- =========================================================
-- Summary
-- =========================================================

\echo ''
\echo '=== Python 101 seed complete ==='

SELECT
  (SELECT COUNT(*) FROM problems WHERE topic_id IN (
    SELECT id FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001'
  )) AS problems,
  (SELECT COUNT(*) FROM test_cases WHERE problem_id IN (
    SELECT id FROM problems WHERE topic_id IN (
      SELECT id FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001'
    )
  )) AS test_cases,
  (SELECT COUNT(*) FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001') AS topics;

\echo ''
\echo 'Class URL (alice@demo.edu):'
\echo '  /student/classes/00000000-0000-0000-0000-000000400101'
\echo ''
\echo 'First problem (Hello, World):'
\echo '  /student/classes/00000000-0000-0000-0000-000000400101/problems/00000000-0000-0000-0000-000000220101'
\echo ''
\echo 'Teacher watch view (eve, watching alice on Hello, World):'
\echo '  /teacher/classes/00000000-0000-0000-0000-000000400101/problems/00000000-0000-0000-0000-000000220101/students/242fea26-1527-4a10-b208-af4cad1e1102'
