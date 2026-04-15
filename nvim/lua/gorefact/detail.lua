local rpc = require("gorefact.rpc")

local M = {}

local ns = vim.api.nvim_create_namespace("gorefact-detail")

local state = {
  buf = nil,
  win = nil,
  current = nil,
}

local function ensure_buffer()
  if state.buf and vim.api.nvim_buf_is_valid(state.buf) then
    return state.buf
  end

  state.buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_name(state.buf, "gorefact-detail")
  vim.bo[state.buf].buftype = "nofile"
  vim.bo[state.buf].bufhidden = "wipe"
  vim.bo[state.buf].swapfile = false
  vim.bo[state.buf].modifiable = false
  vim.bo[state.buf].filetype = "markdown"
  return state.buf
end

local function ensure_window()
  local buf = ensure_buffer()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_set_buf(state.win, buf)
    if M.attach then
      M.attach(buf)
    end
    return state.win
  end

  vim.cmd("vsplit")
  state.win = vim.api.nvim_get_current_win()
  vim.api.nvim_win_set_buf(state.win, buf)
  vim.wo[state.win].wrap = false
  vim.wo[state.win].number = false
  vim.wo[state.win].relativenumber = false
  vim.wo[state.win].signcolumn = "no"
  vim.wo[state.win].cursorline = true
  if M.attach then
    M.attach(buf)
  end
  return state.win
end

local function set_lines(lines)
  local buf = ensure_buffer()
  vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
end

local function source_from_buffer()
  local buf = ensure_buffer()
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  for _, line in ipairs(lines) do
    local file, lnum = line:match("^File:%s+(.+):(%d+)$")
    if file and lnum then
      return file, tonumber(lnum)
    end
  end
  return nil, nil
end

local function open_source(split)
  local file, lnum = source_from_buffer()
  if not file then
    return
  end
  local cmd = split and "vsplit " or "edit "
  vim.cmd(cmd .. vim.fn.fnameescape(file))
  if lnum then
    vim.api.nvim_win_set_cursor(0, { lnum, 1 })
  end
end

local function add_virt_line(line, text, hl_group)
  local buf = ensure_buffer()
  vim.api.nvim_buf_set_extmark(buf, ns, line, 0, {
    virt_text = { { text, hl_group or "DiagnosticWarn" } },
    virt_text_pos = "eol",
  })
end

local function render(detail)
  state.current = detail or {}

  local lines = {
    "# gorefact detail",
    "",
    ("Package: `%s`"):format(detail.pkg or "?"),
    ("Function: `%s`"):format(detail.func or "?"),
    ("File: `%s:%s`"):format(detail.file or "?", detail.line or 0),
    "",
    "Signature:",
    "```go",
    detail.signature or "",
    "```",
    "",
    ("Calls: %d"):format(detail.callCount or 0),
  }

  if detail.callLines and #detail.callLines > 0 then
    table.insert(lines, ("Call lines: %s"):format(table.concat(detail.callLines, ", ")))
  end

  local violation_start = #lines + 1
  if detail.violations and #detail.violations > 0 then
    table.insert(lines, "")
    table.insert(lines, "Violations:")
    for _, violation in ipairs(detail.violations) do
      table.insert(lines, ("- `%s` -> `%s`"):format(violation.from or "?", violation.to or "?"))
      if violation.reason and violation.reason ~= "" then
        table.insert(lines, ("  %s"):format(violation.reason))
      end
    end
  end

  set_lines(lines)

  if detail.violations and #detail.violations > 0 then
    local idx = violation_start
    add_virt_line(idx - 1, " violations", "Title")
    for _, violation in ipairs(detail.violations) do
      add_virt_line(idx, " ⚠", "DiagnosticWarn")
      idx = idx + 1
      if violation.reason and violation.reason ~= "" then
        idx = idx + 1
      end
    end
  end
end

function M.show(detail)
  ensure_window()
  render(detail or {})
end

function M.request(node_id)
  if not node_id then
    return
  end
  ensure_window()
  rpc.request("gorefact.detail", { nodeID = node_id }, function(result, err)
    if err then
      vim.notify(vim.inspect(err), vim.log.levels.ERROR, { title = "gorefact detail" })
      return
    end
    render(result or {})
  end)
end

function M.open_source()
  open_source(false)
end

function M.open_source_split()
  open_source(true)
end

function M.close()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    pcall(vim.api.nvim_win_close, state.win, true)
  end
  state.win = nil
end

function M.attach(buf)
  local opts = { buffer = buf, silent = true, nowait = true }
  vim.keymap.set("n", "q", function()
    M.close()
  end, opts)
  vim.keymap.set("n", "gf", function()
    M.open_source()
  end, opts)
  vim.keymap.set("n", "<C-v>", function()
    M.open_source_split()
  end, opts)
end

return M
