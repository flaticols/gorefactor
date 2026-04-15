local M = {}

local function health()
  return vim.health or require("health")
end

local function system_ok(cmd, cwd)
  if vim.system then
    local result = vim.system(cmd, { text = true, cwd = cwd }):wait()
    return result.code == 0, vim.trim((result.stdout or "") .. "\n" .. (result.stderr or ""))
  end

  local prev = vim.fn.getcwd()
  if cwd and cwd ~= "" then
    vim.cmd("lcd " .. vim.fn.fnameescape(cwd))
  end
  local output = vim.fn.system(cmd)
  local ok = vim.v.shell_error == 0
  if cwd and cwd ~= "" then
    vim.cmd("lcd " .. vim.fn.fnameescape(prev))
  end
  return ok, vim.trim(output or "")
end

function M.check()
  local h = health()
  local cfg = require("gorefact").config()

  h.start("gorefact")

  if vim.fn.executable(cfg.binary or "gorefact") == 1 then
    h.ok(("binary found: %s"):format(cfg.binary or "gorefact"))
    local ok, out = system_ok({ cfg.binary or "gorefact", "version" }, cfg.dir)
    if ok then
      h.ok(("binary version: %s"):format(out))
    else
      h.warn(("failed to read gorefact version: %s"):format(out))
    end
  else
    h.error(("binary not found: %s"):format(cfg.binary or "gorefact"))
  end

  if cfg.rules and cfg.rules ~= "" then
    if vim.fn.filereadable(cfg.rules) == 1 then
      h.ok(("rules file found: %s"):format(cfg.rules))
      if vim.fn.executable(cfg.binary or "gorefact") == 1 then
        local ok, out = system_ok({ cfg.binary or "gorefact", "validate-rules", "--rules", cfg.rules }, cfg.dir)
        if ok then
          h.ok(("rules syntax valid: %s"):format(out))
        else
          h.error(("rules syntax invalid: %s"):format(out))
        end
      end
    else
      h.warn(("rules file missing: %s"):format(cfg.rules))
    end
  else
    h.warn("no rules file configured")
  end

  if vim.fn.executable("go") == 1 then
    local ok, out = system_ok({ "go", "version" }, cfg.dir)
    if ok then
      h.ok(("go toolchain: %s"):format(out))
    else
      h.warn(("failed to read go version: %s"):format(out))
    end
  else
    h.warn("go binary not found")
  end
end

return M
