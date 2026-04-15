local rpc = require("gorefact.rpc")
local tree = require("gorefact.tree")

local M = {}

local ns = vim.api.nvim_create_namespace("gorefact-search")

local state = {
  buf = nil,
  win = nil,
  timer = nil,
  query = "",
  results = {},
  selected = 1,
  active = false,
}

local function ensure_timer()
  if state.timer then
    return state.timer
  end
  state.timer = vim.loop.new_timer()
  return state.timer
end

local function stop_timer()
  if state.timer then
    state.timer:stop()
  end
end

local function close_window()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    pcall(vim.api.nvim_win_close, state.win, true)
  end
  state.win = nil
  state.buf = nil
  state.active = false
  stop_timer()
end

local function ensure_buffer()
  if state.buf and vim.api.nvim_buf_is_valid(state.buf) then
    return state.buf
  end

  state.buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_name(state.buf, "gorefact-search")
  vim.bo[state.buf].buftype = "nofile"
  vim.bo[state.buf].bufhidden = "wipe"
  vim.bo[state.buf].swapfile = false
  vim.bo[state.buf].modifiable = true
  vim.bo[state.buf].filetype = "gorefact-search"
  return state.buf
end

local function float_dimensions()
  local width = math.min(90, math.max(40, math.floor(vim.o.columns * 0.72)))
  local height = math.min(14, math.max(8, math.floor(vim.o.lines * 0.45)))
  return width, height
end

local function ensure_window()
  local buf = ensure_buffer()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_set_buf(state.win, buf)
    return state.win
  end

  local width, height = float_dimensions()
  local row = math.floor((vim.o.lines - height) / 3)
  local col = math.floor((vim.o.columns - width) / 2)
  state.win = vim.api.nvim_open_win(buf, true, {
    relative = "editor",
    row = row,
    col = col,
    width = width,
    height = height,
    style = "minimal",
    border = "rounded",
    title = " gorefact search ",
    title_pos = "center",
  })
  vim.wo[state.win].wrap = false
  vim.wo[state.win].cursorline = true
  vim.wo[state.win].winhl = "Normal:Normal,FloatBorder:FloatBorder,CursorLine:Visual"
  return state.win
end

local function query_line()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return ""
  end
  local line = vim.api.nvim_buf_get_lines(state.buf, 0, 1, false)[1] or ""
  return vim.trim(line)
end

local function clear_highlights(buf)
  pcall(vim.api.nvim_buf_clear_namespace, buf, ns, 0, -1)
end

local function highlight_selected(buf)
  if not state.results or #state.results == 0 then
    return
  end
  local line = state.selected + 2
  vim.api.nvim_buf_add_highlight(buf, ns, "Visual", line - 1, 0, -1)
end

local function render()
  local buf = ensure_buffer()
  local query = query_line()
  state.query = query
  clear_highlights(buf)

  local lines = { query, "", "Results:" }
  for _, item in ipairs(state.results) do
    local suffix = item.kind or "item"
    if item.kind == "struct" then
      suffix = ("%d methods"):format(item.methodCount or 0)
    elseif item.callerCount and item.callerCount > 0 then
      suffix = ("%d callers"):format(item.callerCount)
    end
    table.insert(lines, ("  %s  [%s]"):format(item.name or "?", suffix))
  end

  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false

  highlight_selected(buf)

  local line_count = math.max(1, #lines)
  local target_line = math.min(math.max(1, state.selected + 2), line_count)
  pcall(vim.api.nvim_win_set_cursor, state.win, { target_line, 1 })
  vim.bo[buf].modifiable = false
end

local function open_selected()
  local item = state.results[state.selected]
  if not item then
    return
  end
  close_window()
  tree.open_for_id(item.id, { group = "method" })
end

local function move_selection(delta)
  if #state.results == 0 then
    return
  end
  state.selected = math.max(1, math.min(#state.results, state.selected + delta))
  render()
end

local function refresh()
  if not state.active then
    return
  end
  local query = query_line()
  state.query = query
  if query == "" then
    state.results = {}
    state.selected = 1
    render()
    return
  end

  rpc.request("gorefact.search", { query = query }, function(result, err)
    if err then
      vim.notify(vim.inspect(err), vim.log.levels.ERROR, { title = "gorefact search" })
      return
    end
    state.results = result or {}
    state.selected = 1
    render()
  end)
end

local function schedule_refresh()
  stop_timer()
  ensure_timer():start(150, 0, vim.schedule_wrap(refresh))
end

local function attach_buffer_mappings(buf)
  local opts = { buffer = buf, silent = true, nowait = true }
  vim.keymap.set("i", "<Esc>", function()
    close_window()
  end, opts)
  vim.keymap.set("n", "q", function()
    close_window()
  end, opts)
  vim.keymap.set("i", "<CR>", function()
    open_selected()
  end, opts)
  vim.keymap.set("n", "<CR>", function()
    open_selected()
  end, opts)
  vim.keymap.set("n", "j", function()
    move_selection(1)
  end, opts)
  vim.keymap.set("n", "k", function()
    move_selection(-1)
  end, opts)
  vim.keymap.set("i", "<C-n>", function()
    move_selection(1)
  end, opts)
  vim.keymap.set("i", "<C-p>", function()
    move_selection(-1)
  end, opts)
end

local function attach_autocmds(buf)
  local group = vim.api.nvim_create_augroup("GorefactSearch" .. buf, { clear = true })
  vim.api.nvim_create_autocmd({ "TextChangedI", "TextChanged" }, {
    group = group,
    buffer = buf,
    callback = schedule_refresh,
  })
  vim.api.nvim_create_autocmd("BufLeave", {
    group = group,
    buffer = buf,
    callback = function()
      close_window()
    end,
  })
end

function M.open(query)
  ensure_window()
  local buf = ensure_buffer()
  state.active = true
  state.query = query or ""
  state.results = {}
  state.selected = 1

  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { state.query, "", "Results:" })
  vim.bo[buf].modifiable = false
  vim.api.nvim_win_set_cursor(state.win, { 1, #state.query + 1 })
  attach_buffer_mappings(buf)
  attach_autocmds(buf)
  vim.cmd("startinsert")
  if state.query ~= "" then
    schedule_refresh()
  end
end

function M.close()
  close_window()
end

return M
