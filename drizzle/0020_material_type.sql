-- Add material_type to teaching_units.
-- Types: notes (verbose, detailed), slides (concise, bullet points),
-- worksheet (practice-focused, problems + exercises), reference (lookup/cheat sheet).

ALTER TABLE teaching_units
  ADD COLUMN IF NOT EXISTS material_type varchar(16) NOT NULL DEFAULT 'notes';
