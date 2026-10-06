import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { ensureHub, loadConfig, profileDir, resolveToolArg, saveConfig, updateProfileState } from './config.js';
import { formatStatus } from './format.js';
import { buildEnv, clearProfile, liveCheck, readCredentials } from './health.js';
import { serveMcp } from './mcp.js';
import {
  PACKAGE_ROOT,
  addProfile,
  clear,
  failover,
  removeProfile,
  setOrder,
  setup,
  statusReport,
  syncClaudeMcp,
  useProfile,
} from './ops.js';
import { launch, pinnedProfile, resolveBin, spawnTool, syncProfile } from './run.js';
import { SHELLS, detectShell, envLines, shellInit } from './shell.js';
import { statusline } from './statusline.js';
import { TOOLS, TOOL_IDS, binFor, getTool } from './tools.js';
import { UsageError, c, info, parseFlags, readJson, warn, which } from './util.js';

const VERSION = JSON.parse(fs.readFileSync(path.join(PACKAGE_ROOT, 'package.json'), 'utf8')).version;

const HELP = `aims ${VERSION} - several Claude Code / Codex accounts on one machine

Setup (once per account)
  aims add <tool> <name> [--existing] [--isolated]
                               create a profile; --existing adopts the login you already have
  aims login <tool> <name> [-- args]
                               log that profile in (browser); creates it if needed
  aims setup [--statusline] [--tools claude,codex] [--no-skill] [--no-mcp]
                               install the skill + MCP server (+ Claude status line)
  eval "$(aims shell-init)"    make plain \`claude\` / \`codex\` follow the active profile

Daily use
  aims                         status of all profiles (alias: aims ls, aims status)
  aims use [tool] <name>       switch the active profile (no tool = every tool with that name)
  aims claude [args]           run claude as the active profile (failover if it is limited)
  aims claude@work [args]      run as a specific profile        (same for codex)
  aims env [tool] [name]       print exports that pin THIS terminal: eval "$(aims env work)"
  aims env --reset             un-pin this terminal

When an account is limited or logged out
  aims failover <tool> [--resume] [--to name] [--minutes N]
                               mark the current profile limited, activate the next one;
                               --resume continues the last conversation on the new account
  aims clear <tool> [name]     forget a cooldown / needs-login mark
  aims status --live           verify logins and usage with the providers

More
  aims order <tool> <names...> failover order
  aims rm <tool> <name> [--purge]
  aims logout <tool> <name>
  aims sync [tool]             repair shared links, copy MCP servers into Claude profiles
  aims doctor [--fix]          diagnose setup problems
  aims mcp                     run the MCP server (stdio)
  aims statusline              Claude Code status line command

tools: ${TOOL_IDS.join(', ')}     config: ~/.aims (AIMS_HOME)`;

function out(s) {
  process.stdout.write(s.endsWith('\n') ? s : `${s}\n`);
}

function needTool(arg) {
  if (!arg || !TOOLS[arg]) throw new UsageError(`expected a tool (${TOOL_IDS.join(', ')}), got "${arg || ''}"`);
  return arg;
}

function parseToolAt(token) {
  const m = /^(claude|codex)(?:[@:](.+))?$/.exec(token || '');
  return m ? { id: m[1], profile: m[2] || null } : null;
}

function printReport(id, name, report) {
  for (const r of report) {
    if (r.action === 'conflict') warn(`${id}/${name}: "${r.name}" ${r.detail}`);
    else if (r.action === 'merged' || r.action === 'adopted') {
      info(`${id}/${name}: ${r.action} "${r.name}" into the shared folder${r.detail ? ` (${r.detail})` : ''}`);
    }
  }
}

function claudeAuthStatus(cfg, name) {
  const bin = which(binFor(TOOLS.claude));
  if (!bin) return null;
  const { env } = buildEnv(cfg, 'claude', name);
  const r = spawnSync(bin, ['auth', 'status', '--json'], { env, encoding: 'utf8', timeout: 30000 });
  try {
    return JSON.parse(r.stdout);
  } catch {
    return null;
  }
}

function afterLogin(cfg, id, name) {
  clearProfile(id, name);
  let email = null;
  let plan = null;
  if (id === 'claude') {
    const s = claudeAuthStatus(cfg, name);
    if (s) {
      email = s.email || null;
      plan = s.subscriptionType || null;
    }
  }
  const cred = readCredentials(cfg, id, name);
  email = email || cred.email || null;
  plan = plan || cred.plan || null;
  if (email || plan) updateProfileState(id, name, (s) => ({ ...s, account: { email, plan } }));
  if (email) {
    for (const other of Object.keys(cfg.tools[id].profiles)) {
      if (other === name) continue;
      const oc = readCredentials(cfg, id, other);
      if (oc.email && oc.email.toLowerCase() === email.toLowerCase()) {
        warn(`"${name}" and "${other}" are both logged in as ${email}. Log in again with the other account (use a private browser window).`);
      }
    }
  }
  return { email, plan };
}

async function cmdLogin(argv, { logout = false } = {}) {
  const { positional, rest } = parseFlags(argv);
  const id = needTool(positional[0]);
  const name = positional[1];
  if (!name) throw new UsageError(`usage: aims ${logout ? 'logout' : 'login'} <tool> <name>`);
  let cfg = loadConfig();
  if (!cfg.tools[id].profiles[name]) {
    if (logout) throw new Error(`no ${id} profile "${name}"`);
    addProfile(id, name);
    info(`created ${id} profile "${name}"`);
    cfg = loadConfig();
  }
  printReport(id, name, syncProfile(cfg, id, name));
  const tool = getTool(id);
  const bin = resolveBin(id);
  const { env } = buildEnv(cfg, id, name);
  if (!logout) info(`logging in ${tool.title} profile "${name}". ${tool.loginTip}`);
  const code = await new Promise((resolve, reject) => {
    const child = spawnTool(bin, [...(logout ? tool.logoutArgs : tool.loginArgs), ...rest], { env, stdio: 'inherit' });
    child.on('error', reject);
    child.on('exit', (code) => resolve(code ?? 1));
  });
  if (code !== 0) return code;
  if (logout) {
    updateProfileState(id, name, (s) => ({ ...s, account: undefined }));
    return 0;
  }
  const acct = afterLogin(cfg, id, name);
  info(`${id}/${name} logged in${acct.email ? ` as ${acct.email}` : ''}${acct.plan ? ` (${acct.plan})` : ''}`);
  return 0;
}

async function cmdStatus(argv) {
  const { flags, positional } = parseFlags(argv, { live: 'bool', json: 'bool' });
  const ids = resolveToolArg(positional[0]);
  if (flags.live) {
    const cfg = loadConfig();
    for (const id of ids) {
      for (const name of cfg.tools[id].order) {
        if (!flags.json) info(`checking ${id}/${name}...`);
        const r = await liveCheck(cfg, id, name);
        if (!flags.json && r.status !== 'ok') warn(`${id}/${name}: ${r.status}${r.detail ? ` - ${r.detail}` : ''}`);
      }
    }
  }
  const report = statusReport({ ids });
  if (flags.json) return out(JSON.stringify(report, null, 2));
  out(formatStatus(report, { color: process.stdout.isTTY && !process.env.NO_COLOR }));
  const anyProfiles = report.some((t) => t.profiles.length);
  if (!anyProfiles) out(`\nStart with:  aims add claude personal --existing   (adopts your current login)\n             aims login claude work`);
}

async function cmdDoctor(argv) {
  const { flags } = parseFlags(argv, { fix: 'bool' });
  const cfg = loadConfig();
  let problems = 0;
  const ok = (m) => out(`${c.green('ok')}    ${m}`);
  const bad = (m) => {
    problems++;
    out(`${c.yellow('warn')}  ${m}`);
  };
  const [major] = process.versions.node.split('.').map(Number);
  (major >= 18 ? ok : bad)(`node ${process.versions.node}`);
  for (const id of TOOL_IDS) {
    const tool = TOOLS[id];
    const bin = which(binFor(tool));
    if (!bin) {
      bad(`${tool.bin} not found on PATH`);
      continue;
    }
    const v = spawnSync(bin, ['--version'], { encoding: 'utf8', timeout: 20000 });
    ok(`${tool.bin} ${(v.stdout || '').trim().split('\n')[0]} (${bin})`);
    for (const k of tool.conflictingEnv) {
      if (process.env[k]) bad(`${k} is set in this shell; it overrides profile logins (aims drops it when launching ${id})`);
    }
    const t = cfg.tools[id];
    if (!Object.keys(t.profiles).length) {
      out(`      ${id}: no profiles`);
      continue;
    }
    ensureHub(cfg, id);
    const pin = pinnedProfile(id);
    if (process.env[tool.homeEnv]) out(`      ${tool.homeEnv}=${process.env[tool.homeEnv]}${pin ? ` (terminal pinned to "${pin}")` : ''}`);
    const emails = {};
    for (const name of t.order) {
      const report = syncProfile(cfg, id, name, { fix: flags.fix });
      const conflicts = report.filter((r) => r.action === 'conflict');
      printReport(id, name, report);
      const cred = readCredentials(cfg, id, name);
      const label = `${id}/${name}${cred.email ? ` <${cred.email}>` : ''}`;
      if (cred.present === false) bad(`${label}: not logged in -> aims login ${id} ${name}`);
      else if (conflicts.length) bad(`${label}: ${conflicts.length} shared item(s) not linked (see above)`);
      else ok(`${label}: ${t.profiles[name].existing ? 'default login' : t.profiles[name].isolated ? 'isolated' : 'shared links fine'}`);
      if (cred.email) (emails[cred.email.toLowerCase()] ||= []).push(name);
    }
    for (const [email, names] of Object.entries(emails)) {
      if (names.length > 1) bad(`${id}: ${names.join(' and ')} are the same account (${email})`);
    }
    if (id === 'claude') {
      const settings = readJson(path.join(t.hub, 'settings.json'), {}) || {};
      if (settings.env && settings.env.CLAUDE_CONFIG_DIR) bad('settings.json env sets CLAUDE_CONFIG_DIR; remove it, aims sets it per profile');
    }
  }
  saveConfig(cfg);
  out(problems ? `\n${problems} thing(s) to look at.` : '\nAll good.');
  return problems ? 1 : 0;
}

export async function main(argv) {
  const [cmd, ...args] = argv;
  try {
    // `aims claude ...`, `aims claude@work ...`
    const direct = parseToolAt(cmd);
    if (direct) {
      const r = await launch({ id: direct.id, profile: direct.profile, args });
      return r.code;
    }
    switch (cmd) {
      case undefined:
      case 'ls':
      case 'list':
      case 'status':
        return await cmdStatus(args);
      case 'run': {
        const target = parseToolAt(args[0]);
        if (!target) throw new UsageError('usage: aims run <tool>[@profile] [args...]');
        const r = await launch({ id: target.id, profile: target.profile, args: args.slice(1) });
        return r.code;
      }
      case 'add': {
        const { flags, positional } = parseFlags(args, { existing: 'bool', isolated: 'bool', dir: 'string', login: 'bool' });
        const id = needTool(positional[0]);
        const name = positional[1];
        if (!name) throw new UsageError('usage: aims add <tool> <name> [--existing] [--isolated]');
        const r = addProfile(id, name, flags);
        printReport(id, name, r.report);
        info(`added ${id} profile "${name}"${flags.existing ? ' (uses your existing login)' : ''} -> ${r.dir}${r.active ? ' [active]' : ''}`);
        if (flags.login) return await cmdLogin([id, name]);
        if (!flags.existing) info(`next: aims login ${id} ${name}`);
        else afterLogin(loadConfig(), id, name);
        return 0;
      }
      case 'rm':
      case 'remove': {
        const { flags, positional } = parseFlags(args, { purge: 'bool' });
        const id = needTool(positional[0]);
        if (!positional[1]) throw new UsageError('usage: aims rm <tool> <name> [--purge]');
        const r = removeProfile(id, positional[1], flags);
        info(`removed ${id} profile "${positional[1]}"${r.purged ? ` and deleted ${r.dir}` : ''}`);
        return 0;
      }
      case 'use': {
        const { positional } = parseFlags(args);
        const [a, b] = positional;
        if (!a) throw new UsageError('usage: aims use [tool] <name>');
        const ids = b ? useProfile(needTool(a), b) : useProfile(null, a);
        const name = b || a;
        info(`active ${ids.join(' + ')} profile: ${name}`);
        for (const id of ids) {
          const pin = pinnedProfile(id);
          if (pin && pin !== name) warn(`this terminal is pinned to ${id} "${pin}" (aims env --reset to follow the active profile)`);
        }
        return 0;
      }
      case 'order': {
        const id = needTool(args[0]);
        info(`${id} failover order: ${setOrder(id, args.slice(1)).join(' -> ')}`);
        return 0;
      }
      case 'env': {
        const { flags, positional } = parseFlags(args, { shell: 'string', reset: 'bool' });
        const shell = flags.shell || detectShell();
        if (!SHELLS.includes(shell)) throw new UsageError(`--shell must be one of ${SHELLS.join(', ')}`);
        let [a, b] = positional;
        let tool = null;
        if (a && TOOLS[a]) {
          tool = a;
          a = b;
        }
        out(envLines(shell, { tool, profile: a || null, reset: flags.reset }));
        return 0;
      }
      case 'shell-init': {
        const shell = args[0] || detectShell();
        if (!SHELLS.includes(shell)) throw new UsageError(`shell must be one of ${SHELLS.join(', ')}`);
        out(shellInit(shell));
        return 0;
      }
      case 'login':
        return await cmdLogin(args);
      case 'logout':
        return await cmdLogin(args, { logout: true });
      case 'failover': {
        const { flags, positional } = parseFlags(args, {
          to: 'string',
          from: 'string',
          minutes: 'string',
          reason: 'string',
          resume: 'bool',
        });
        const id = needTool(positional[0]);
        const r = failover(id, {
          from: flags.from,
          to: flags.to,
          minutes: flags.minutes ? Number(flags.minutes) : undefined,
          reason: flags.reason,
        });
        info(`${id}: "${r.from}" cooling down${r.until ? ` until ${new Date(r.until).toLocaleString()}` : ''} -> active: "${r.to}"`);
        if (r.pinned && r.pinned !== r.to) info(`(this terminal was pinned to "${r.pinned}"; aims skips it while it cools down)`);
        if (flags.resume) {
          const res = await launch({ id, profile: r.to, args: getTool(id).resumeArgs });
          return res.code;
        }
        info(`continue the same conversation: aims ${id} ${getTool(id).resumeArgs.join(' ')}`);
        return 0;
      }
      case 'clear': {
        const id = needTool(args[0]);
        info(`cleared ${id}: ${clear(id, args[1]).join(', ') || '(none)'}`);
        return 0;
      }
      case 'sync': {
        const cfg = loadConfig();
        for (const id of resolveToolArg(args[0])) {
          for (const name of cfg.tools[id].order) printReport(id, name, syncProfile(cfg, id, name));
        }
        if (resolveToolArg(args[0]).includes('claude')) {
          for (const line of syncClaudeMcp(cfg)) info(`copied MCP servers -> ${line}`);
        }
        saveConfig(cfg);
        info('shared links are up to date');
        return 0;
      }
      case 'doctor':
        return await cmdDoctor(args);
      case 'setup': {
        const { flags } = parseFlags(args, {
          tools: 'string',
          statusline: 'bool',
          'no-skill': 'bool',
          'no-mcp': 'bool',
        });
        const ids = flags.tools ? flags.tools.split(',').map((s) => needTool(s.trim())) : TOOL_IDS;
        for (const m of setup({ ids, skill: !flags['no-skill'], mcp: !flags['no-mcp'], statusline: flags.statusline })) info(m);
        info('restart running claude/codex sessions to pick these up');
        return 0;
      }
      case 'mcp':
        return await serveMcp();
      case 'statusline':
        await statusline();
        return 0;
      case 'dir': {
        const id = needTool(args[0]);
        const cfg = loadConfig();
        out(profileDir(cfg, id, args[1] || cfg.tools[id].active));
        return 0;
      }
      case 'help':
      case '-h':
      case '--help':
        out(HELP);
        return 0;
      case 'version':
      case '-v':
      case '--version':
        out(VERSION);
        return 0;
      default:
        throw new UsageError(`unknown command "${cmd}" (see: aims help)`);
    }
  } catch (err) {
    if (err instanceof UsageError) {
      process.stderr.write(`aims: ${err.message}\n`);
      return 2;
    }
    throw err;
  }
}

