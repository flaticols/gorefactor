local rpc = require("gorefact.rpc")
local detail = require("gorefact.detail")

local M = {}

local modes = { "method", "pkg", "caller" }

vim.fn.sign_define("GorefactWarn", { text = "⚠", texthl = "DiagnosticWarn" })
vim.fn.sign_define("GorefactOk", { text = "✓", texthl = "DiagnosticOk" })

local state = {
  buf = nil,
  win = nil,
  func_id = nil,
  group = "method",
  violations_only = false,
  nodes = {},
  rows = {},
  expanded = {},
}

local render
local reload
local update_detail_from_cursor

local function ensure_buffer()
  if state.buf and vim.api.nvim_buf_is_valid(state.buf) then
    return state.buf
  end

  state.buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_name(state.buf, "gorefact-tree")
  vim.bo[state.buf].buftype = "nofile"
  vim.bo[state.buf].bufhidden = "wipe"
  vim.bo[state.buf].swapfile = false
  vim.bo[state.buf].modifiable = false
  vim.bo[state.buf].filetype = "gorefact-tree"
  return state.buf
end

local function ensure_window()
  local buf = ensure_buffer()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_set_buf(state.win, buf)
    return state.win
  end
  state.win = vim.api.nvim_get_current_win()
  vim.api.nvim_win_set_buf(state.win, buf)
  vim.wo[state.win].wrap = false
  vim.wo[state.win].number = false
  vim.wo[state.win].relativenumber = false
  vim.wo[state.win].cursorline = true
  vim.wo[state.win].signcolumn = "yes"
  return state.win
end

local function clear_buffer(buf)
  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, {})
  vim.bo[buf].modifiable = false
end

local function node_has_children(node)
  return node and node.children and #node.children > 0
end

local function current_entry()
  if not state.rows or #state.rows == 0 then
    return nil
  end
  local line = vim.api.nvim_win_get_cursor(state.win)[1]
  return state.rows[line]
end

local function open_source(entry, split)
  if not entry or not entry.node or not entry.node.file or not entry.node.line then
    return
  end
  local cmd = split and "vsplit " or "edit "
  vim.cmd(cmd .. vim.fn.fnameescape(entry.node.file))
  vim.api.nvim_win_set_cursor(0, { entry.node.line, 1 })
end

render = function()
  local buf = ensure_buffer()
  clear_buffer(buf)
  state.rows = {}

  local lines = {}
  local function walk(nodes, depth, prefix)
    for index, node in ipairs(nodes or {}) do
      local path = prefix == "" and tostring(index) or (prefix .. "/" .. tostring(index))
      local has_children = node_has_children(node)
      local expanded = has_children and (state.expanded[path] ~= false)
      if depth == 0 and state.expanded[path] == nil then
        expanded = true
      end
      if has_children and state.expanded[path] == nil then
        state.expanded[path] = expanded
      end

      local marker
      if has_children then
        marker = expanded and "▼" or "▶"
      else
        marker = node.violation and "⚠" or "✓"
      end

      local label = node.label or ""
      local line = ("%s%s %s"):format(string.rep("  ", depth), marker, label)
      table.insert(lines, line)
      table.insert(state.rows, {
        path = path,
        parent_path = prefix ~= "" and prefix or nil,
        node = node,
        depth = depth,
        expanded = expanded,
        has_children = has_children,
        line = #lines,
      })

      if has_children and expanded then
        walk(node.children, depth + 1, path)
      end
    end
  end

  walk(state.nodes, 0, "")

  if #lines == 0 then
    table.insert(lines, "No callers found")
  end

  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false

  pcall(vim.fn.sign_unplace, "GorefactTree", { buffer = buf })
  for _, row in ipairs(state.rows) do
    if row.node and not row.has_children then
      local sign = row.node.violation and "GorefactWarn" or "GorefactOk"
      pcall(vim.fn.sign_place, row.line, "GorefactTree", sign, buf, { lnum = row.line, priority = 10 })
    end
  end

  local line = math.max(1, math.min(vim.api.nvim_win_get_cursor(state.win)[1], #lines))
  pcall(vim.api.nvim_win_set_cursor, state.win, { line, 1 })
  vim.bo[buf].modified = false
  update_detail_from_cursor()
end

reload = function()
  if not state.func_id then
    return
  end
  rpc.request("gorefact.tree", {
    id = state.func_id,
    group = state.group,
    violationsOnly = state.violations_only,
  }, function(result, err)
    if err then
      vim.notify(vim.inspect(err), vim.log.levels.ERROR, { title = "gorefact tree" })
      return
    end
    state.nodes = (result and result.nodes) or {}
    render()
  end)
end

update_detail_from_cursor = function()
  local entry = current_entry()
  if not entry then
    return
  end
  detail.request(entry.node.id)
end

local function toggle_current()
  local entry = current_entry()
  if not entry then
    return
  end
  if entry.has_children then
    state.expanded[entry.path] = not entry.expanded
    render()
    return
  end
  open_source(entry, false)
end

local function move_to_parent()
  local entry = current_entry()
  if not entry then
    return
  end
  if entry.has_children and entry.expanded then
    state.expanded[entry.path] = false
    render()
    return
  end
  if entry.parent_path then
    for _, row in ipairs(state.rows) do
      if row.path == entry.parent_path then
        vim.api.nvim_win_set_cursor(state.win, { row.line, 1 })
        update_detail_from_cursor()
        return
      end
    end
  end
end

local function cycle_group()
  local index = 1
  for i, mode in ipairs(modes) do
    if mode == state.group then
      index = i
      break
    end
  end
  state.group = modes[(index % #modes) + 1]
  reload()
end

local function toggle_violations()
  state.violations_only = not state.violations_only
  reload()
end

local function yank_current(kind)
  local entry = current_entry()
  if not entry then
    return
  end
  local node = entry.node or {}
  local value
  if kind == "file" then
    value = node.file and ("%s:%d"):format(node.file, node.line or 0) or ""
  elseif kind == "package" then
    value = node.pkg or node.package or ""
  else
    value = node.label or ""
  end
  if value ~= "" then
    vim.fn.setreg("+", value)
  end
end

local function close_windows()
  detail.close()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    pcall(vim.api.nvim_win_close, state.win, true)
  end
  state.win = nil
end

local function open_cursor_source(split)
  local entry = current_entry()
  if not entry then
    return
  end
  open_source(entry, split)
end

local function attach_keymaps(buf)
  local opts = { buffer = buf, silent = true, nowait = true }
  vim.keymap.set("n", "<CR>", toggle_current, opts)
  vim.keymap.set("n", "l", toggle_current, opts)
  vim.keymap.set("n", "h", move_to_parent, opts)
  vim.keymap.set("n", "g", cycle_group, opts)
  vim.keymap.set("n", "v", toggle_violations, opts)
  vim.keymap.set("n", "gf", function()
    open_cursor_source(false)
  end, opts)
  vim.keymap.set("n", "<C-v>", function()
    open_cursor_source(true)
  end, opts)
  vim.keymap.set("n", "yf", function()
    yank_current("file")
  end, opts)
  vim.keymap.set("n", "yy", function()
    yank_current("label")
  end, opts)
  vim.keymap.set("n", "yp", function()
    yank_current("package")
  end, opts)
  vim.keymap.set("n", "q", close_windows, opts)
end

local function attach_autocmds(buf)
  local group = vim.api.nvim_create_augroup("GorefactTree" .. buf, { clear = true })
  vim.api.nvim_create_autocmd({ "CursorMoved", "BufEnter" }, {
    group = group,
    buffer = buf,
    callback = update_detail_from_cursor,
  })
  vim.api.nvim_create_autocmd("BufWinLeave", {
    group = group,
    buffer = buf,
    callback = function()
      detail.close()
    end,
  })
end

function M.open_for_id(func_id, opts)
  opts = opts or {}
  state.func_id = func_id
  state.group = opts.group or state.group or "method"
  state.violations_only = opts.violationsOnly or false
  state.expanded = {}
  ensure_window()
  attach_keymaps(state.buf)
  attach_autocmds(state.buf)
  reload()
end

function M.open(opts)
  opts = opts or {}
  local buf = vim.api.nvim_get_current_buf()
  local file = vim.api.nvim_buf_get_name(buf)
  local row = vim.api.nvim_win_get_cursor(0)[1]

  rpc.request("gorefact.funcAtPos", { file = file, line = row }, function(func, err)
    if err then
      vim.notify(vim.inspect(err), vim.log.levels.ERROR, { title = "gorefact tree" })
      return
    end
    if not func or not func.id then
      vim.notify("gorefact: no function under cursor", vim.log.levels.WARN, { title = "gorefact tree" })
      return
    end
    M.open_for_id(func.id, opts)
  end)
end

function M.refresh()
  reload()
end

function M.status()
  local violations = 0
  for _, row in ipairs(state.rows or {}) do
    if row.node and row.node.violation then
      violations = violations + 1
    end
  end
  return {
    func_id = state.func_id,
    group = state.group,
    violations_only = state.violations_only,
    violations = violations,
  }
end

return M
