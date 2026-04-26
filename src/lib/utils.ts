import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import { customAlphabet } from "nanoid";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"; // no 0/O/1/I to avoid confusion
const generate = customAlphabet(alphabet, 8);

export function generateJoinCode(): string {
  return generate();
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Return true when the string is a well-formed UUID v4 (lowercase or uppercase). */
export function isValidUUID(value: string): boolean {
  return UUID_RE.test(value);
}
