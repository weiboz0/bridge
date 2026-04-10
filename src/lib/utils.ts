import { customAlphabet } from "nanoid";

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"; // no 0/O/1/I to avoid confusion
const generate = customAlphabet(alphabet, 8);

export function generateJoinCode(): string {
  return generate();
}
