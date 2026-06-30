import type { Plugin } from "@opencode-ai/plugin"

export const KpsPlugin: Plugin = async ({ $ }) => {
  return {
    "file.edited": async (input: { filePath: string }) => {
      if (input.filePath.endsWith(".ts") || input.filePath.endsWith(".tsx")) {
        await $`tsguard check`
      }
    },
  }
}
