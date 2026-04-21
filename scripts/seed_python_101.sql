-- Python 101 demo course — a full teachable unit exercising the new
-- Problem / Attempt / Test workflow (plans 024–026, 028).
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
-- Problems (new schema: scope, scope_id, starter_code as jsonb)
-- =========================================================
-- scope      = 'org'
-- scope_id   = 'd386983b-6da4-4cb8-8057-f2aa70d27c07'  (Bridge Demo School)
-- created_by = 'd0d3b031-a483-4214-97fb-48c9584f4dcb'  (eve@demo.edu)
-- starter_code is jsonb: jsonb_build_object('python', '<code>')
-- No topic_id, language, or "order" columns.

-- ---- Topic 1: Hello, World ----

INSERT INTO problems (id, scope, scope_id, title, description, starter_code, difficulty, grade_level, tags, status, created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220101',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'Hello, World',
    $md$Print the exact text:

```
Hello, World!
```

Use the `print` function. No input.$md$,
    jsonb_build_object('python', $py$print("Hello, World!")
$py$),
    'easy', '9-12', ARRAY['output', 'print'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220102',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'Three Lines',
    $md$Print three lines exactly:

```
line 1
line 2
line 3
```
$md$,
    jsonb_build_object('python', $py$# Print each line on its own line.
$py$),
    'easy', '9-12', ARRAY['output', 'print'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 2: Variables & Input ----

INSERT INTO problems (id, scope, scope_id, title, description, starter_code, difficulty, grade_level, tags, status, created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220201',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'Greet by Name',
    $md$Read a single name from input and greet that person.

**Input:** one line with the name.

**Output:** `Hello, {name}!` (exactly, with the exclamation mark).$md$,
    jsonb_build_object('python', $py$name = input()
print(f"Hello, {name}!")
$py$),
    'easy', '9-12', ARRAY['input', 'variables', 'strings'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220202',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
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
    jsonb_build_object('python', $py$name = input()
age = input()
# Use an f-string to combine them into one sentence.
$py$),
    'easy', '9-12', ARRAY['input', 'variables', 'strings'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 3: Numbers & Arithmetic ----

INSERT INTO problems (id, scope, scope_id, title, description, starter_code, difficulty, grade_level, tags, status, created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220301',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
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
    jsonb_build_object('python', $py$a = int(input())
b = int(input())
print(a + b)
$py$),
    'easy', '9-12', ARRAY['arithmetic', 'integers'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220302',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
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
    jsonb_build_object('python', $py$w = int(input())
h = int(input())
# Multiply and print.
$py$),
    'easy', '9-12', ARRAY['arithmetic', 'integers'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 4: Conditionals ----

INSERT INTO problems (id, scope, scope_id, title, description, starter_code, difficulty, grade_level, tags, status, created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220401',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'Even or Odd',
    $md$Read an integer and print `Even` if it's even, `Odd` if it's odd.

**Example 1 — Input:** `4` → **Output:** `Even`

**Example 2 — Input:** `7` → **Output:** `Odd`

Watch your capitalization — the output is case-sensitive.$md$,
    jsonb_build_object('python', $py$n = int(input())
# Hint: use n % 2
$py$),
    'easy', '9-12', ARRAY['conditionals', 'modulo'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220402',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'Pass or Fail',
    $md$Grading: a student passes if their score is 60 or higher. Read a score (integer 0–100) and print `Pass` or `Fail`.$md$,
    jsonb_build_object('python', $py$score = int(input())
if score >= 60:
    print("Pass")
else:
    print("Fail")
$py$),
    'easy', '9-12', ARRAY['conditionals'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 5: Loops ----

INSERT INTO problems (id, scope, scope_id, title, description, starter_code, difficulty, grade_level, tags, status, created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220501',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
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
    jsonb_build_object('python', $py$n = int(input())
for i in range(1, n + 1):
    print(i)
$py$),
    'easy', '9-12', ARRAY['loops', 'range'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220502',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'FizzBuzz (short)',
    $md$Read a positive integer N. For each integer from 1 to N, print:

- `Fizz` if it's divisible by 3
- `Buzz` if it's divisible by 5
- `FizzBuzz` if it's divisible by both
- Otherwise print the number itself

Each value goes on its own line. Classic interview question.$md$,
    jsonb_build_object('python', $py$n = int(input())
for i in range(1, n + 1):
    # Decide what to print for i.
    pass
$py$),
    'medium', '9-12', ARRAY['loops', 'conditionals', 'modulo'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- ---- Topic 6: Lists ----

INSERT INTO problems (id, scope, scope_id, title, description, starter_code, difficulty, grade_level, tags, status, created_by) VALUES
  (
    '00000000-0000-0000-0000-000000220601',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'Sum of a List',
    $md$Read one line of space-separated integers. Print their sum.

**Input:** `3 1 4 1 5 9 2 6`

**Output:** `31`$md$,
    jsonb_build_object('python', $py$nums = list(map(int, input().split()))
print(sum(nums))
$py$),
    'easy', '9-12', ARRAY['lists', 'builtins'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  (
    '00000000-0000-0000-0000-000000220602',
    'org',
    'd386983b-6da4-4cb8-8057-f2aa70d27c07',
    'Max in a List',
    $md$Read one line of space-separated integers and print the largest one.

**Input:** `4 -2 11 7`

**Output:** `11`

Don't use Python's `max` — practice writing the loop yourself.$md$,
    jsonb_build_object('python', $py$nums = list(map(int, input().split()))
# Walk through nums and track the largest value seen.
$py$),
    'medium', '9-12', ARRAY['lists', 'loops'], 'published',
    'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  )
  ON CONFLICT (id) DO NOTHING;

-- =========================================================
-- topic_problems — attach each problem to its topic
-- =========================================================
-- sort_order matches the old "order" column value.
-- attached_by = eve@demo.edu

INSERT INTO topic_problems (topic_id, problem_id, sort_order, attached_by) VALUES
  -- Topic 1: Hello, World
  ('00000000-0000-0000-0000-000000110001', '00000000-0000-0000-0000-000000220101', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  ('00000000-0000-0000-0000-000000110001', '00000000-0000-0000-0000-000000220102', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  -- Topic 2: Variables & Input
  ('00000000-0000-0000-0000-000000110002', '00000000-0000-0000-0000-000000220201', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  ('00000000-0000-0000-0000-000000110002', '00000000-0000-0000-0000-000000220202', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  -- Topic 3: Numbers & Arithmetic
  ('00000000-0000-0000-0000-000000110003', '00000000-0000-0000-0000-000000220301', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  ('00000000-0000-0000-0000-000000110003', '00000000-0000-0000-0000-000000220302', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  -- Topic 4: Conditionals
  ('00000000-0000-0000-0000-000000110004', '00000000-0000-0000-0000-000000220401', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  ('00000000-0000-0000-0000-000000110004', '00000000-0000-0000-0000-000000220402', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  -- Topic 5: Loops
  ('00000000-0000-0000-0000-000000110005', '00000000-0000-0000-0000-000000220501', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  ('00000000-0000-0000-0000-000000110005', '00000000-0000-0000-0000-000000220502', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  -- Topic 6: Lists
  ('00000000-0000-0000-0000-000000110006', '00000000-0000-0000-0000-000000220601', 0, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'),
  ('00000000-0000-0000-0000-000000110006', '00000000-0000-0000-0000-000000220602', 1, 'd0d3b031-a483-4214-97fb-48c9584f4dcb')
ON CONFLICT DO NOTHING;

-- =========================================================
-- problem_solutions — one canonical Python solution per problem
-- =========================================================
-- Solution UUID scheme: 55PPNN where PP = topic (01..06), NN = problem index.
-- Fixed UUIDs ensure ON CONFLICT (id) DO NOTHING is truly idempotent.

INSERT INTO problem_solutions (id, problem_id, language, title, code, is_published, created_by) VALUES
  -- 1.1 Hello, World
  (
    '00000000-0000-0000-0000-000000550101',
    '00000000-0000-0000-0000-000000220101', 'python', 'Canonical solution',
    $sol$print("Hello, World!")
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 1.2 Three Lines
  (
    '00000000-0000-0000-0000-000000550102',
    '00000000-0000-0000-0000-000000220102', 'python', 'Canonical solution',
    $sol$print("line 1")
print("line 2")
print("line 3")
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 2.1 Greet by Name
  (
    '00000000-0000-0000-0000-000000550201',
    '00000000-0000-0000-0000-000000220201', 'python', 'Canonical solution',
    $sol$name = input()
print(f"Hello, {name}!")
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 2.2 Name Then Age
  (
    '00000000-0000-0000-0000-000000550202',
    '00000000-0000-0000-0000-000000220202', 'python', 'Canonical solution',
    $sol$name = input()
age = input()
print(f"{name} is {age} years old.")
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 3.1 Sum Two Numbers
  (
    '00000000-0000-0000-0000-000000550301',
    '00000000-0000-0000-0000-000000220301', 'python', 'Canonical solution',
    $sol$a = int(input())
b = int(input())
print(a + b)
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 3.2 Area of a Rectangle
  (
    '00000000-0000-0000-0000-000000550302',
    '00000000-0000-0000-0000-000000220302', 'python', 'Canonical solution',
    $sol$w = int(input())
h = int(input())
print(w * h)
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 4.1 Even or Odd
  (
    '00000000-0000-0000-0000-000000550401',
    '00000000-0000-0000-0000-000000220401', 'python', 'Canonical solution',
    $sol$n = int(input())
if n % 2 == 0:
    print("Even")
else:
    print("Odd")
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 4.2 Pass or Fail
  (
    '00000000-0000-0000-0000-000000550402',
    '00000000-0000-0000-0000-000000220402', 'python', 'Canonical solution',
    $sol$score = int(input())
if score >= 60:
    print("Pass")
else:
    print("Fail")
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 5.1 Count to N
  (
    '00000000-0000-0000-0000-000000550501',
    '00000000-0000-0000-0000-000000220501', 'python', 'Canonical solution',
    $sol$n = int(input())
for i in range(1, n + 1):
    print(i)
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 5.2 FizzBuzz
  (
    '00000000-0000-0000-0000-000000550502',
    '00000000-0000-0000-0000-000000220502', 'python', 'Canonical solution',
    $sol$n = int(input())
for i in range(1, n + 1):
    if i % 15 == 0:
        print("FizzBuzz")
    elif i % 3 == 0:
        print("Fizz")
    elif i % 5 == 0:
        print("Buzz")
    else:
        print(i)
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 6.1 Sum of a List
  (
    '00000000-0000-0000-0000-000000550601',
    '00000000-0000-0000-0000-000000220601', 'python', 'Canonical solution',
    $sol$nums = list(map(int, input().split()))
print(sum(nums))
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
  ),
  -- 6.2 Max in a List
  (
    '00000000-0000-0000-0000-000000550602',
    '00000000-0000-0000-0000-000000220602', 'python', 'Canonical solution',
    $sol$nums = list(map(int, input().split()))
best = nums[0]
for n in nums[1:]:
    if n > best:
        best = n
print(best)
$sol$,
    true, 'd0d3b031-a483-4214-97fb-48c9584f4dcb'
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

INSERT INTO class_settings (id, class_id, editor_mode)
VALUES ('00000000-0000-0000-0000-000000400301', '00000000-0000-0000-0000-000000400101', 'python')
ON CONFLICT (id) DO NOTHING;

COMMIT;

-- =========================================================
-- Summary
-- =========================================================

\echo ''
\echo '=== Python 101 seed complete ==='

SELECT
  (SELECT COUNT(*) FROM problems WHERE scope = 'org' AND scope_id = 'd386983b-6da4-4cb8-8057-f2aa70d27c07'
    AND id IN (SELECT problem_id FROM topic_problems WHERE topic_id IN (
      SELECT id FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001'
    ))
  ) AS problems,
  (SELECT COUNT(*) FROM test_cases WHERE problem_id IN (
    SELECT problem_id FROM topic_problems WHERE topic_id IN (
      SELECT id FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001'
    )
  )) AS test_cases,
  (SELECT COUNT(*) FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001') AS topics,
  (SELECT COUNT(*) FROM topic_problems WHERE topic_id IN (
    SELECT id FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001'
  )) AS topic_problem_links,
  (SELECT COUNT(*) FROM problem_solutions WHERE problem_id IN (
    SELECT problem_id FROM topic_problems WHERE topic_id IN (
      SELECT id FROM topics WHERE course_id = '00000000-0000-0000-0000-00000aa00001'
    )
  )) AS solutions;

\echo ''
\echo 'Class URL (alice@demo.edu):'
\echo '  /student/classes/00000000-0000-0000-0000-000000400101'
\echo ''
\echo 'First problem (Hello, World):'
\echo '  /student/classes/00000000-0000-0000-0000-000000400101/problems/00000000-0000-0000-0000-000000220101'
\echo ''
\echo 'Teacher watch view (eve, watching alice on Hello, World):'
\echo '  /teacher/classes/00000000-0000-0000-0000-000000400101/problems/00000000-0000-0000-0000-000000220101/students/242fea26-1527-4a10-b208-af4cad1e1102'
