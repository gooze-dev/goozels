// goozels plugin for opencode v2 (2.x). After the agent edits a Go file or a
// gooze config, it appends `goozels check` output to the tool result the model
// reads. Copy to .opencode/plugins/goozels.ts in your project.
import { execFile } from "node:child_process"
import path from "node:path"

const wanted = (f?: string): f is string => !!f && (f.endsWith(".go") || /(^|\/)\.?gooze\.ya?ml$/.test(f))

const check = (files: string[], cwd: string) =>
  new Promise<string>((resolve) =>
    execFile("goozels", ["check", "--max", "40", ...files], { cwd, timeout: 60_000 }, (_err, stdout) =>
      resolve(String(stdout ?? "").trim()),
    ),
  )

export default {
  id: "goozels.check",
  async setup(ctx: any) {
    const root: string = ctx.location.directory
    await ctx.tool.hook("execute.after", async (event: any) => {
      if (event.status !== "completed") return

      let files: (string | undefined)[] = []
      if (event.tool === "edit" || event.tool === "write") files = [event.input?.path && path.resolve(root, event.input.path)]
      else if (event.tool === "patch")
        files = (event.result.output?.applied ?? []).filter((a: any) => a.type !== "delete").map((a: any) => a.target)

      const checked = files.filter(wanted)
      if (!checked.length) return

      const found = await check(checked, root)
      if (!found) return

      const prior = event.result.content
      const content =
        typeof prior === "string"
          ? [{ type: "text", text: prior }]
          : prior?.length
            ? [...prior]
            : [{ type: "text", text: JSON.stringify(event.result.output) }]
      content.push({ type: "text", text: `gooze results from the last run:\n${found}` })
      event.result = { ...event.result, content }
    })
  },
}
