import type { Plugin } from "@opencode-ai/plugin"

export const PknPlugin: Plugin = async ({ $ }) => {
  return {
    "file.edited": async (input: { filePath: string }) => {
      if (input.filePath.endsWith(".py")) {
        await $`__BINARY__ check`
      }
    },
  }
}
