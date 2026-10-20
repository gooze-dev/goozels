# Neovim setup

goozels is a drop-in proxy in front of `gopls`: point your Go LSP config at the
`goozels` binary instead of `gopls`, and additionally attach it to gooze config
files. It finds the real `gopls` on `PATH` (override with `--gopls /path` or the
`GOOZELS_GOPLS` env var).

## nvim-lspconfig

```lua
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

-- Reuse gopls' settings/root detection, but launch goozels instead.
lspconfig.gopls.setup({
  cmd = { 'goozels' },            -- goozels proxies the real gopls
  -- cmd = { 'goozels', '--gopls', '/usr/local/bin/gopls' },
  filetypes = { 'go', 'gomod', 'gowork', 'gotmpl' },
})

-- Attach the same server to gooze config files for completion/diagnostics there.
if not configs.goozels_config then
  configs.goozels_config = {
    default_config = {
      cmd = { 'goozels' },
      filetypes = { 'yaml.gooze' },
      root_dir = lspconfig.util.root_pattern('gooze.yaml', '.git'),
      single_file_support = true,
    },
  }
end
lspconfig.goozels_config.setup({})

-- Map the gooze config file to a dedicated filetype so the server attaches.
vim.filetype.add({
  filename = {
    ['gooze.yaml'] = 'yaml.gooze',
  },
})
```

With this in place, opening a `.go` file gives you everything gopls provides plus
`//gooze:ignore` mutagen completion, hover and diagnostics; opening `gooze.yaml`
gives config-key completion, hover and validation.
