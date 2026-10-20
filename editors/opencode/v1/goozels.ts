// goozels plugin for opencode v1 (1.x). After the agent edits a Go file or a
// gooze config, it appends `goozels check` output, with its stderr notes, to
// the tool result the model reads. Copy to .opencode/plugins/goozels.ts in your project.
import type { Plugin } from "@opencode-ai/plugin"

const wanted = (f?: string): f is string => !!f && (f.endsWith(".go") || /(^|\/)\.?gooze\.ya?ml$/.test(f))

export const Goozels: Plugin = async ({ $, directory }) => ({
  "tool.execute.after": async (input, output) => {
    let files: (string | undefined)[] = []
    if (input.tool === "edit" || input.tool === "write") files = [input.args?.filePath]
    else if (input.tool === "apply_patch")
      files = (output.metadata?.files ?? [])
        .filter((f: any) => f.type !== "delete")
        .map((f: any) => f.movePath ?? f.filePath)

    const checked = files.filter(wanted)
    if (!checked.length) return

    // Notes such as "no gooze results for x.go yet" go to stderr; keep them.
    const res = await $`goozels check --max 40 ${checked}`.cwd(directory).nothrow().quiet()
    const found = [res.stderr, res.stdout].map((b) => b.toString().trim()).filter(Boolean).join("\n")
    if (found) output.output += `\n\ngooze results from the last run:\n${found}`
  },
})
