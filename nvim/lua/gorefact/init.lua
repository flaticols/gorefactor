local rpc = require("gorefact.rpc")

local M = {}

local state = {
  config = {
    binary = "gorefact",
    dir = vim.fn.getcwd(),
    tests = false,
    filter_pkg = "",
    rules = "rules.toml",
    patterns = { "./..." },
    server_args = {},
    keys = {
      explore = "<leader>ge",
      callers = "<leader>gc",
      callees = "<leader>gC",
      check = "<leader>gv",
    },
  },
  commands_registered = false,
  keymaps_registered = false,
}

local function deep_copy(value)
  if type(value) ~= "table" then
    return value
  end
  local out = {}
  for k, v in pairs(value) do
    out[k] = deep_copy(v)
  end
  return out
end

local function merge_config(opts)
  return vim.tbl_deep_extend("force", deep_copy(state.config), opts or {})
end

local function current_location()
  local file = vim.api.nvim_buf_get_name(0)
  local line = vim.api.nvim_win_get_cursor(0)[1]
  return file, line
end

local function set_quickfix(violations)
  local items = {}
  for _, violation in ipairs(violations or {}) do
    local caller = violation.caller or {}
    local callee = violation.callee or {}
    table.insert(items, {
      filename = caller.file,
      lnum = caller.line,
      col = caller.col or 1,
      text = ("%s -> %s: %s"):format(caller.name or "?", callee.name or "?", violation.rule and violation.rule.reason or ""),
    })
  end
  vim.fn.setqflist({}, " ", {
    title = "gorefact violations",
    items = items,
  })
  vim.cmd("copen")
end

local function register_commands()
  if state.commands_registered then
    return
  end
  state.commands_registered = true

  vim.api.nvim_create_user_command("GorefactExplore", function(opts)
    require("gorefact.search").open(table.concat(opts.fargs, " "))
  end, { nargs = "*" })

  vim.api.nvim_create_user_command("GorefactCallers", function()
    require("gorefact.tree").open({ group = "method" })
  end, {})

  vim.api.nvim_create_user_command("GorefactCallees", function()
    require("gorefact.tree").open({ group = "caller" })
  end, {})

  vim.api.nvim_create_user_command("GorefactCheck", function()
    vim.notify("gorefact: running check", vim.log.levels.INFO, { title = "gorefact" })
    rpc.request("gorefact.check", {}, function(result, err)
      if err then
        vim.notify(vim.inspect(err), vim.log.levels.ERROR, { title = "gorefact check" })
        return
      end
      set_quickfix(result and result.violations or {})
    end)
  end, {})

  vim.api.nvim_create_user_command("GorefactRestart", function()
    rpc.restart()
  end, {})
end

local function register_keymaps()
  if state.keymaps_registered then
    return
  end
  state.keymaps_registered = true

  local keys = state.config.keys or {}
  if keys.explore then
    vim.keymap.set("n", keys.explore, "<cmd>GorefactExplore<cr>", { silent = true, desc = "Gorefact explore" })
  end
  if keys.callers then
    vim.keymap.set("n", keys.callers, "<cmd>GorefactCallers<cr>", { silent = true, desc = "Gorefact callers" })
  end
  if keys.callees then
    vim.keymap.set("n", keys.callees, "<cmd>GorefactCallees<cr>", { silent = true, desc = "Gorefact callees" })
  end
  if keys.check then
    vim.keymap.set("n", keys.check, "<cmd>GorefactCheck<cr>", { silent = true, desc = "Gorefact check" })
  end
end

function M.setup(opts)
  state.config = merge_config(opts)
  rpc.setup(state.config)
  register_commands()
  register_keymaps()
end

function M.config()
  return state.config
end

function M.explore(query)
  require("gorefact.search").open(query)
end

function M.callers()
  require("gorefact.tree").open({ group = "method" })
end

function M.callees()
  require("gorefact.tree").open({ group = "caller" })
end

function M.check()
  vim.cmd("GorefactCheck")
end

function M.restart()
  vim.cmd("GorefactRestart")
end

function M.current_location()
  return current_location()
end

function M.statusline()
  local status = require("gorefact.tree").status()
  if not status.func_id then
    return ""
  end
  local parts = { "gorefact", status.group or "method" }
  table.insert(parts, tostring(status.violations or 0) .. "v")
  if status.violations_only then
    table.insert(parts, "filtered")
  end
  return table.concat(parts, " ")
end

return M
