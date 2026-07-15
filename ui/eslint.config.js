import js from "@eslint/js";
import prettier from "eslint-config-prettier";
import svelte from "eslint-plugin-svelte";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: [".svelte-kit/**", "build/**", "node_modules/**"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...svelte.configs["flat/recommended"],
  {
    files: ["**/*.svelte"],
    languageOptions: {
      parserOptions: { parser: tseslint.parser },
      globals: globals.browser,
    },
  },
  {
    files: ["**/*.ts"],
    languageOptions: { globals: { ...globals.browser, ...globals.node } },
  },
  prettier,
);
