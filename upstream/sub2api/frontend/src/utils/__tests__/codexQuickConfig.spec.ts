import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'
import { mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, realpathSync, symlinkSync, writeFileSync, chmodSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { Language, Parser } from 'web-tree-sitter'
import { buildCodexQuickConfigToml, buildMacLinuxCodexQuickConfigScript, buildMacCodexQuickConfigScript, buildWindowsCmdCodexQuickConfigScript, normalizeCodexBaseUrl, isCodexQuickConfigPlatform } from '../codexQuickConfig'

const roots: string[] = []
const input = { apiKey: 'sk-secret-value', baseUrl: 'https://example.com', platform: 'openai' as const }
const require = createRequire(import.meta.url)
const nativeUnix = process.platform !== 'win32'
const powershell = process.env.SUB2API_TEST_POWERSHELL || (process.platform === 'win32' ? 'powershell.exe' : '')
function runner() {
  const dir = mkdtempSync(join(tmpdir(), 'codex-switch-')); roots.push(dir)
  const configDir = join(dir, '中文 空格 & ! 目录'); mkdirSync(configDir)
  const path = join(dir, 'setup.command')
  return { dir, configDir, run(script: string, choice = '1\n') {
    writeFileSync(path, script)
    return spawnSync('/bin/bash', [path], { input: choice, encoding: 'utf8', timeout: 10000,
      env: { ...process.env, HOME: dir, CODEX_HOME: configDir } })
  } }
}
function psRunner() {
  const dir = mkdtempSync(join(tmpdir(), 'codex-switch-ps-')); roots.push(dir)
  const configDir = join(dir, '中文 空格 & ! 目录'); mkdirSync(configDir)
  return { dir, configDir, run(script: string, choice = '1\n') {
    const path = join(dir, '中文 空格 & (setup) !.cmd')
    writeFileSync(path, script)
    const command = script.match(/-EncodedCommand ([A-Za-z0-9+/=]+)/)?.[1]
    if (!command) throw new Error('Missing PowerShell bootstrap')
    return spawnSync(powershell, ['-NoProfile', '-EncodedCommand', command], { input: choice, encoding: 'utf8', timeout: 10000,
      env: { ...process.env, HOME: dir, CODEX_HOME: configDir, SUB2API_CODEX_SCRIPT: path } })
  } }
}
afterEach(() => { for (const dir of roots.splice(0)) { try { chmodSync(dir, 0o700); rmSync(dir, { recursive: true, force: true }) } catch { /* retain failed fixture */ } } })

const original = '# personal settings\nmodel = "official-model"\nmodel_provider = "openai"\n[features]\ngoals = false\n[mcp_servers.example]\ncommand = "my-tool"\n'
for (const [label, build] of [['macOS', buildMacCodexQuickConfigScript], ['Linux', buildMacLinuxCodexQuickConfigScript]] as const) {
  describe.runIf(nativeUnix)(`${label} offline switcher`, () => {
    it('waits for a choice and exits without changing files', () => {
      const r = runner(); writeFileSync(join(r.configDir, 'config.toml'), original)
      const result = r.run(build(input), '0\n')
      expect(result.status, result.stderr).toBe(0)
      expect(result.stdout).toContain('1. 连接星桥AI')
      expect(result.stdout).toContain('2. 切回官方 Codex')
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(original)
    })
    it('connects offline, preserves personal settings and restores official settings across repeats', () => {
      const r = runner(); writeFileSync(join(r.configDir, 'config.toml'), original)
      writeFileSync(join(r.configDir, 'auth.json'), '{"auth":"keep"}')
      const script = build({ ...input, modelCatalogContent: '{"models":[{"slug":"custom-model"}]}' })
      for (let i = 0; i < 2; i++) {
        const result = r.run(script)
        expect(result.status, result.stdout + result.stderr).toBe(0)
        const config = readFileSync(join(r.configDir, 'config.toml'), 'utf8')
        expect(config).toContain('model_provider = "xingqiao"')
        expect(config.match(/\[model_providers.xingqiao\]/g)).toHaveLength(1)
        expect(config).toContain('[mcp_servers.example]\ncommand = "my-tool"')
        expect(config).toContain('goals = false')
        expect(config).toContain('model_catalog_json = "' + realpathSync(r.configDir) + '/.xingqiao/models.json"')
        expect(statSync(join(r.configDir, 'config.toml')).mode & 0o777).toBe(0o600)
      }
      const result = r.run(script, '2\n')
      expect(result.status, result.stdout + result.stderr).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(original)
      expect(readFileSync(join(r.configDir, 'auth.json'), 'utf8')).toBe('{"auth":"keep"}')
      expect(readdirSync(join(r.configDir, 'xingqiao-backups')).length).toBeGreaterThanOrEqual(3)
    })
    it('configures without a catalog and can return a fresh installation to official defaults', () => {
      const r = runner(); const script = build(input)
      expect(r.run(script).status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).not.toContain('model_catalog_json')
      expect(r.run(script, '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).not.toContain('sk-secret-value')
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toContain('model_provider = "openai"')
    })
    it('preserves edits made while connected and refreshes the official snapshot after official edits', () => {
      const r = runner(); const script = build(input)
      writeFileSync(join(r.configDir, 'config.toml'), original)
      expect(r.run(script).status).toBe(0)
      const connected = readFileSync(join(r.configDir, 'config.toml'), 'utf8').replace('goals = false', 'goals = true')
      writeFileSync(join(r.configDir, 'config.toml'), connected)
      expect(r.run(script, '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toContain('goals = true')
      const latest = original.replace('official-model', 'new-official-model').replace('goals = false', 'goals = true')
      writeFileSync(join(r.configDir, 'config.toml'), latest)
      expect(r.run(script).status).toBe(0)
      expect(r.run(script, '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(latest)
    })
    it('returns an existing third-party provider to official defaults', () => {
      const r = runner(); const script = build(input)
      writeFileSync(join(r.configDir, 'config.toml'), original.replace('"openai"', '"another-relay"') + '[model_providers.another-relay]\nbase_url = "https://relay.invalid/v1"\n')
      expect(r.run(script, '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toContain('model_provider = "openai"')
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).not.toContain('model = "official-model"')
    })
    it('preserves BOM, CRLF and missing-final-newline settings through a round trip', () => {
      const r = runner(); const script = build(input)
      const config = '\uFEFF' + original.trimEnd().replace(/\n/g, '\r\n')
      writeFileSync(join(r.configDir, 'config.toml'), config)
      expect(r.run(script).status).toBe(0)
      const connected = readFileSync(join(r.configDir, 'config.toml'), 'utf8')
      expect(connected).not.toContain('\uFEFF')
      expect(connected.match(/^model_provider\s*=/gm)).toHaveLength(1)
      expect(r.run(script, '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(config)
    })
    it('refuses inline provider conflicts without changing the original file', () => {
      for (const config of ['model_providers.xingqiao = { name = "existing" }\n', 'model_providers = { other = { name = "existing" } }\n', '[model_providers]\nxingqiao = { name = "existing" }\n']) {
        const r = runner(); writeFileSync(join(r.configDir, 'config.toml'), config)
        expect(r.run(build(input)).status).not.toBe(0)
        expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(config)
      }
    })
    it('restores both config and catalog when publishing config fails after catalog publication', () => {
      const r = runner(); writeFileSync(join(r.configDir, 'config.toml'), original)
      mkdirSync(join(r.configDir, '.xingqiao')); writeFileSync(join(r.configDir, '.xingqiao/models.json'), 'original catalog')
      const script = build({ ...input, modelCatalogContent: '{"models":[]}' })
      // Fail only the config rename; exercise the real EXIT rollback and real catalog write.
      const shim = join(r.dir, 'bin'); mkdirSync(shim)
      writeFileSync(join(shim, 'mv'), '#!/bin/bash\ncase "$*" in *new.toml*) exit 73 ;; esac\nexec /bin/mv "$@"\n', { mode: 0o700 })
      const result = r.run(script.replace('export PATH="${PATH:-}:/usr/bin:/bin"', 'export PATH="' + shim + ':${PATH:-}:/usr/bin:/bin"'))
      expect(result.status).toBe(73)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(original)
      expect(readFileSync(join(r.configDir, '.xingqiao/models.json'), 'utf8')).toBe('original catalog')
      expect(readdirSync(r.configDir)).not.toContain('.xingqiao-lock')
    })
    it('keeps the official catalog untouched when connecting with a separate Xingqiao catalog', () => {
      const r = runner(); const catalogPath = realpathSync(r.configDir) + '/codex-models.json'
      const config = 'model_catalog_json = "' + catalogPath + '"\n' + original
      writeFileSync(join(r.configDir, 'config.toml'), config)
      writeFileSync(join(r.configDir, 'codex-models.json'), 'personal catalog')
      const script = build({ ...input, modelCatalogContent: '{"models":[]}' })
      expect(r.run(script).status).toBe(0)
      expect(readFileSync(join(r.configDir, 'codex-models.json'), 'utf8')).toBe('personal catalog')
      expect(r.run(script, '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(config)
      expect(readFileSync(join(r.configDir, 'codex-models.json'), 'utf8')).toBe('personal catalog')
    })
    it('preserves quoted provider names containing a literal dot after personal edits', () => {
      const r = runner(); const script = build(input)
      const provider = '[model_providers."xingqiao.personal"]\nbase_url = "https://personal.invalid/v1"\n'
      writeFileSync(join(r.configDir, 'config.toml'), original + provider)
      expect(r.run(script).status).toBe(0)
      const connected = readFileSync(join(r.configDir, 'config.toml'), 'utf8')
      expect(connected).toContain(provider)
      writeFileSync(join(r.configDir, 'config.toml'), connected.replace('goals = false', 'goals = true'))
      expect(r.run(script, '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toContain(provider)
    })
    it('preserves multiline TOML strings that contain apparent selectors and tables', () => {
      const r = runner()
      const config = 'developer_instructions = """\nmodel = "keep-me"\n[model_providers.xingqiao]\n"""\n' + original
      writeFileSync(join(r.configDir, 'config.toml'), config)
      expect(r.run(build(input)).status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toContain('developer_instructions = """\nmodel = "keep-me"\n[model_providers.xingqiao]\n"""')
      expect(r.run(build(input), '2\n').status).toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(config)
    })
    it('repairs owner permissions automatically without changing unrelated files', () => {
      const r = runner(); writeFileSync(join(r.configDir, 'config.toml'), original, { mode: 0o400 })
      writeFileSync(join(r.configDir, 'untouched'), 'keep', { mode: 0o640 }); chmodSync(r.configDir, 0o500)
      const result = r.run(build(input))
      expect(result.status, result.stdout + result.stderr).toBe(0)
      expect(statSync(r.configDir).mode & 0o700).toBe(0o700)
      expect(statSync(join(r.configDir, 'untouched')).mode & 0o777).toBe(0o640)
    })
    it('rejects symlinks and directory collisions without replacing the original config', () => {
      for (const target of ['config.toml', '.xingqiao/models.json']) {
        const r = runner(); if (target !== 'config.toml') writeFileSync(join(r.configDir, 'config.toml'), original)
        mkdirSync(join(r.configDir, target), { recursive: true })
        const result = r.run(build({ ...input, modelCatalogContent: '{"models":[]}' }))
        expect(result.status).not.toBe(0)
        if (target !== 'config.toml') expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(original)
      }
      const r = runner(); const outside = join(r.dir, 'outside'); writeFileSync(outside, original)
      symlinkSync(outside, join(r.configDir, 'config.toml'))
      expect(r.run(build(input)).status).not.toBe(0)
      expect(readFileSync(outside, 'utf8')).toBe(original)
    })
    it('leaves existing files intact when embedded catalog decoding fails', () => {
      const r = runner(); writeFileSync(join(r.configDir, 'config.toml'), original)
      mkdirSync(join(r.configDir, '.xingqiao')); writeFileSync(join(r.configDir, '.xingqiao/models.json'), 'old catalog')
      const script = build({ ...input, modelCatalogContent: '{"models":[]}' }).replace(/CATALOG_PAYLOAD='[^']*'/, "CATALOG_PAYLOAD='@@@'")
      expect(r.run(script).status).not.toBe(0)
      expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(original)
      expect(readFileSync(join(r.configDir, '.xingqiao/models.json'), 'utf8')).toBe('old catalog')
      expect(readdirSync(r.configDir).some(name => name.startsWith('.xingqiao-tmp'))).toBe(false)
    })
    it('does not perform network checks and handles invalid menu input', () => {
      const r = runner(); const script = build(input)
      expect(script).not.toMatch(/\b(curl|wget|Invoke-WebRequest|Invoke-RestMethod)\b/)
      const result = r.run(script, 'bad\n0\n')
      expect(result.status).toBe(0)
      expect(readdirSync(r.configDir)).not.toContain('config.toml')
    })
  })
}

describe('config builders', () => {
  it('normalizes URL and escapes untrusted TOML input', () => {
    expect(normalizeCodexBaseUrl(' https://example.com/// ')).toBe('https://example.com/v1')
    const config = buildCodexQuickConfigToml({ ...input, apiKey: 'sk-"\nvalue' })
    expect(config).toContain('experimental_bearer_token = "sk-\\"\\u000avalue"')
    expect(isCodexQuickConfigPlatform('composite')).toBe(true)
    expect(isCodexQuickConfigPlatform(null)).toBe(false)
  })
  it('parses the CMD bootstrap and clears inherited ERRORLEVEL before reading the process status', async () => {
    const script = buildWindowsCmdCodexQuickConfigScript(input)
    await Parser.init({ locateFile: () => require.resolve('web-tree-sitter/tree-sitter.wasm') })
    const language = await Language.load(require.resolve('tree-sitter-batch/tree-sitter-batch.wasm'))
    const parser = new Parser().setLanguage(language)
    const tree = parser.parse(script.slice(0, script.lastIndexOf('# SUB2API_POWERSHELL')))
    expect(tree?.rootNode.hasError).toBe(false)
    tree?.delete(); parser.delete()
    expect(script.indexOf('set "ERRORLEVEL="')).toBeGreaterThan(0)
    expect(script.indexOf('set "ERRORLEVEL="')).toBeLessThan(script.indexOf('-EncodedCommand'))
  })
  it('generates an ASCII CRLF CMD with no permanent execution-policy change', () => {
    const script = buildWindowsCmdCodexQuickConfigScript(input)
    expect(script.startsWith('@echo off\r\n')).toBe(true)
    expect([...script].every(char => char.charCodeAt(0) <= 127)).toBe(true)
    expect(script).not.toContain('-NonInteractive')
    expect(script).not.toMatch(/Set-ExecutionPolicy|Invoke-WebRequest|Invoke-RestMethod/)
  })
})

describe.runIf(Boolean(powershell))('PowerShell offline switcher', () => {
  it('preserves case-sensitive personal edits and quoted provider names on official restore', () => {
    const r = psRunner(); const script = buildWindowsCmdCodexQuickConfigScript(input)
    const config = original + '[model_providers."xingqiao.personal"]\nname = "abc"\n'
    writeFileSync(join(r.configDir, 'config.toml'), config)
    expect(r.run(script).status).toBe(0)
    const connected = readFileSync(join(r.configDir, 'config.toml'), 'utf8')
    expect(connected).toContain('[model_providers."xingqiao.personal"]')
    writeFileSync(join(r.configDir, 'config.toml'), connected.replace('"abc"', '"ABC"'))
    expect(r.run(script, '2\n').status).toBe(0)
    expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toContain('name = "ABC"')
  })
  it('keeps the official catalog untouched and refreshes subsequent official settings', () => {
    const r = psRunner(); const script = buildWindowsCmdCodexQuickConfigScript({ ...input, modelCatalogContent: '{"models":[]}' })
    writeFileSync(join(r.configDir, 'config.toml'), original)
    writeFileSync(join(r.configDir, 'codex-models.json'), 'personal catalog')
    expect(r.run(script).status).toBe(0)
    expect(readFileSync(join(r.configDir, '.xingqiao/models.json'), 'utf8')).toBe('{"models":[]}')
    expect(r.run(script, '2\n').status).toBe(0)
    const latest = original.replace('official-model', 'new-official-model')
    writeFileSync(join(r.configDir, 'config.toml'), latest)
    expect(r.run(script).status).toBe(0)
    expect(r.run(script, '2\n').status).toBe(0)
    expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(latest)
    expect(readFileSync(join(r.configDir, 'codex-models.json'), 'utf8')).toBe('personal catalog')
  })
  it('exits on EOF and rejects directory conflicts without changing the config', () => {
    const r = psRunner(); const script = buildWindowsCmdCodexQuickConfigScript(input)
    expect(r.run(script, '').status).toBe(0)
    expect(readdirSync(r.configDir)).toEqual([])
    mkdirSync(join(r.configDir, 'config.toml'))
    expect(r.run(script).status).toBe(1)
    expect(statSync(join(r.configDir, 'config.toml')).isDirectory()).toBe(true)
  })
  it('rolls back the catalog when config publication fails', () => {
    const r = psRunner(); writeFileSync(join(r.configDir, 'config.toml'), original)
    mkdirSync(join(r.configDir, '.xingqiao')); writeFileSync(join(r.configDir, '.xingqiao/models.json'), 'old catalog')
    const script = buildWindowsCmdCodexQuickConfigScript({ ...input, modelCatalogContent: '{"models":[]}' }).replace('$configChanged = $true; Publish-File $configTemp $configFile', '$configChanged = $true; throw "Injected config publish failure"')
    expect(r.run(script).status).toBe(1)
    expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(original)
    expect(readFileSync(join(r.configDir, '.xingqiao/models.json'), 'utf8')).toBe('old catalog')
    expect(readdirSync(join(r.configDir, '.xingqiao')).some(name => name.startsWith('tmp.'))).toBe(false)
  })
  it('connects and restores official config without touching login data', () => {
    const r = psRunner(); writeFileSync(join(r.configDir, 'config.toml'), original)
    const script = buildWindowsCmdCodexQuickConfigScript({ ...input, modelCatalogContent: '{"models":[]}' })
    const result = r.run(script)
    expect(result.status, result.stdout + result.stderr).toBe(0)
    expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toContain('model_provider = "xingqiao"')
    expect(r.run(script, '2\n').status).toBe(0)
    expect(readFileSync(join(r.configDir, 'config.toml'), 'utf8')).toBe(original)
  })
})
