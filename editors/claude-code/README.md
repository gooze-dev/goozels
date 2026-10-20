# Claude Code setup

The Claude Code plugin for gooze now lives in its own marketplace repository,
[gooze-dev/claude-plugin](https://github.com/gooze-dev/claude-plugin). Install it
in a Claude Code session:

```text
/plugin marketplace add gooze-dev/claude-plugin
/plugin install gooze@gooze
```

It adds a `/gooze:mutation-testing` skill and a post-edit hook that runs
`goozels hook claude`, so Claude sees the mutation score and surviving mutants
after every Go edit. It works alongside the official `gopls-lsp` plugin.

See [Use with AI coding agents](../../README.md#use-with-ai-coding-agents) in the
main README for the workflow, and the plugin's README for setup details.
