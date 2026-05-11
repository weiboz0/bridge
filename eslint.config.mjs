import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
  {
    files: ["**/*.ts", "**/*.tsx"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          paths: [
            {
              name: "@/lib/api-client",
              importNames: ["ApiError"],
              message:
                "Import ApiError from '@/lib/api-error' instead. @/lib/api-client transitively imports next/headers and cannot be used in client components.",
            },
          ],
        },
      ],
    },
  },
]);

export default eslintConfig;
