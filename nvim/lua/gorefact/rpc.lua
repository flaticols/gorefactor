local M = {}

local state = {
  config = {
    binary = "gorefact",
    dir = ".",
    tests = false,
    filter_pkg = "",
    rules = "rules.toml",
    patterns = { "./..." },
    server_args = {},
  },
  job_id = nil,
  next_id = 1,
  pending = {},
  stdout_buffer = "",
  stderr_buffer = "",
  stopping = false,
  autocmd_registered = false,
}

local function shallow_copy(list)
  local out = {}
  for i, v in ipairs(list or {}) do
    out[i] = v
  end
  return out
end

local function merge_config(opts)
  local cfg = vim.tbl_deep_extend("force", {}, state.config, opts or {})
  cfg.patterns = shallow_copy(cfg.patterns)
  cfg.server_args = shallow_copy(cfg.server_args)
  return cfg
end

local function job_command()
  local cmd = { state.config.binary, "serve" }
  if state.config.rules and state.config.rules ~= "" then
    table.insert(cmd, "--rules")
    table.insert(cmd, state.config.rules)
  end
  if state.config.dir and state.config.dir ~= "" then
    table.insert(cmd, "--dir")
    table.insert(cmd, state.config.dir)
  end
  if state.config.filter_pkg and state.config.filter_pkg ~= "" then
    table.insert(cmd, "--filter-pkg")
    table.insert(cmd, state.config.filter_pkg)
  end
  if state.config.tests then
    table.insert(cmd, "--tests")
  end
  for _, arg in ipairs(state.config.server_args or {}) do
    table.insert(cmd, arg)
  end
  for _, pattern in ipairs(state.config.patterns or {}) do
    table.insert(cmd, pattern)
  end
  return cmd
end

local function notify_progress(msg)
  if vim.notify then
    vim.schedule(function()
      vim.notify(msg, vim.log.levels.INFO, { title = "gorefact" })
    end)
  end
end

local function handle_message(raw)
  raw = vim.trim(raw or "")
  if raw == "" then
    return
  end
  local ok, msg = pcall(vim.json.decode, raw)
  if not ok then
    notify_progress("gorefact: failed to decode rpc message")
    return
  end

  if msg.method and not msg.id then
    if msg.method == "gorefact.progress" and msg.params then
      local stage = msg.params.stage or "working"
      notify_progress("gorefact: " .. stage)
      return
    end
    if msg.method == "gorefact.ready" then
      notify_progress("gorefact: ready")
      return
    end
    return
  end

  local id = tostring(msg.id)
  local pending = state.pending[id]
  if not pending then
    return
  end
  state.pending[id] = nil

  if msg.error then
    pending(nil, msg.error)
    return
  end
  pending(msg.result, nil)
end

local function drain_buffer(buffer)
  local lines = {}
  local start = 1
  while true do
    local stop = buffer:find("\n", start, true)
    if not stop then
      break
    end
    table.insert(lines, buffer:sub(start, stop - 1))
    start = stop + 1
  end
  return lines, buffer:sub(start)
end

local function on_stdout(_, data, _)
  local chunk = table.concat(data or {}, "\n")
  if chunk == "" then
    return
  end
  state.stdout_buffer = state.stdout_buffer .. chunk
  local lines, remainder = drain_buffer(state.stdout_buffer)
  state.stdout_buffer = remainder
  for _, line in ipairs(lines) do
    handle_message(line)
  end
end

local function on_stderr(_, data, _)
  local chunk = table.concat(data or {}, "\n")
  if chunk == "" then
    return
  end
  state.stderr_buffer = state.stderr_buffer .. chunk
  local lines, remainder = drain_buffer(state.stderr_buffer)
  state.stderr_buffer = remainder
  for _, line in ipairs(lines) do
    if vim.trim(line) ~= "" then
      notify_progress("gorefact: " .. vim.trim(line))
    end
  end
end

local function on_exit(_, code, _)
  state.job_id = nil
  state.stdout_buffer = ""
  state.stderr_buffer = ""
  local pending = state.pending
  state.pending = {}
  if not state.stopping and next(pending) ~= nil then
    for _, cb in pairs(pending) do
      cb(nil, { code = code, message = "gorefact server exited" })
    end
  end
  if not state.stopping then
    vim.schedule(function()
      vim.notify(("gorefact server exited (code %s)"):format(code), vim.log.levels.ERROR, { title = "gorefact" })
      if vim.ui and vim.ui.select then
        vim.ui.select({ "restart", "ignore" }, { prompt = "gorefact server crashed" }, function(choice)
          if choice == "restart" then
            pcall(M.start)
          end
        end)
      end
    end)
  end
end

function M.setup(opts)
  state.config = merge_config(opts)
  if not state.autocmd_registered then
    state.autocmd_registered = true
    vim.api.nvim_create_autocmd("VimLeavePre", {
      callback = function()
        pcall(M.stop)
      end,
      desc = "Stop gorefact server",
    })
  end
end

function M.config()
  return state.config
end

function M.start()
  if state.job_id and vim.fn.jobwait({ state.job_id }, 0)[1] == -1 then
    return state.job_id
  end

  state.stopping = false
  local job_id = vim.fn.jobstart(job_command(), {
    stdout_buffered = false,
    stderr_buffered = false,
    on_stdout = on_stdout,
    on_stderr = on_stderr,
    on_exit = on_exit,
  })
  if job_id <= 0 then
    error("gorefact: failed to start server")
  end
  state.job_id = job_id
  return job_id
end

function M.stop()
  state.stopping = true
  if state.job_id then
    pcall(vim.fn.jobstop, state.job_id)
  end
  state.job_id = nil
end

function M.restart()
  M.stop()
  state.stopping = false
  return M.start()
end

function M.request(method, params, cb)
  local id = state.next_id
  state.next_id = state.next_id + 1
  state.pending[tostring(id)] = cb or function() end

  if not (state.job_id and vim.fn.jobwait({ state.job_id }, 0)[1] == -1) then
    M.start()
  end

  local payload = vim.json.encode({
    jsonrpc = "2.0",
    id = id,
    method = method,
    params = params or {},
  })
  vim.fn.chansend(state.job_id, payload .. "\n")
  return id
end

function M.notify(method, params)
  if not (state.job_id and vim.fn.jobwait({ state.job_id }, 0)[1] == -1) then
    M.start()
  end
  local payload = vim.json.encode({
    jsonrpc = "2.0",
    method = method,
    params = params or {},
  })
  vim.fn.chansend(state.job_id, payload .. "\n")
end

return M
