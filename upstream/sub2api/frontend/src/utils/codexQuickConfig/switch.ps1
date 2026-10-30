$ErrorActionPreference = 'Stop'
function Decode-Text([string]$value) { return [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($value)) }
$messages = (Decode-Text '@@MESSAGES@@') | ConvertFrom-Json
$configPayload = '@@CONFIG@@'
$catalogPayload = '@@CATALOG@@'
$utf8 = [Text.UTF8Encoding]::new($false)
$stage = ''; $backup = ''; $lock = ''; $configChanged = $false; $catalogChanged = $false
$status = 1
try { [Console]::OutputEncoding = $utf8 } catch { }

function Check-Target([string]$path, [bool]$directory) {
  if (Test-Path -LiteralPath $path) {
    $item = Get-Item -LiteralPath $path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Refusing a symbolic link or junction: $path" }
    if ($item.PSIsContainer -ne $directory) { throw "Path has the wrong file type: $path" }
    if ($env:OS -eq 'Windows_NT') {
      # Repair only these exact targets, never recurse or reset unrelated ACLs.
      $identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
      $acl = Get-Acl -LiteralPath $path
      $rule = [Security.AccessControl.FileSystemAccessRule]::new($identity, [Security.AccessControl.FileSystemRights]::Modify, [Security.AccessControl.AccessControlType]::Allow)
      $acl.AddAccessRule($rule)
      try { Set-Acl -LiteralPath $path -AclObject $acl }
      catch { throw "Cannot repair permissions for the current user: $path. Ask the administrator to restore access. $($_.Exception.Message)" }
      if (!$directory -and $item.IsReadOnly) { $item.IsReadOnly = $false }
    }
  }
}
function Private-Directory([string]$path) {
  Check-Target $path $true
  [void][IO.Directory]::CreateDirectory($path)
  if ($env:OS -eq 'Windows_NT') {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = Get-Acl -LiteralPath $path
    $acl.SetAccessRuleProtection($true, $false)
    $rule = [Security.AccessControl.FileSystemAccessRule]::new($identity, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow')
    $acl.SetAccessRule($rule)
    Set-Acl -LiteralPath $path -AclObject $acl
  }
}
function Write-Text([string]$path, [string]$text) { [IO.File]::WriteAllText($path, $text, $utf8) }
function Transform-Toml([string]$text, [string]$mode) {
  $root = $true; $managed = $false; $multi = ''; $table = ''; $state = 'official'; $customUrl = $false
  $text = $text.TrimStart([char]0xFEFF).Replace("`r`n", "`n")
  if ($text.Length -gt 0 -and !$text.EndsWith("`n")) { $text += "`n" }
  $lines = [Collections.Generic.List[string]]::new()
  foreach ($raw in [regex]::Split($text, '(?<=\n)')) {
    if ($raw.Length -eq 0) { continue }
    $line = $raw.TrimStart(); $free = $multi.Length -eq 0
    if ($free -and $line.StartsWith('[')) {
      $root = $false
      $parent = $line -cmatch '^\[\s*(model_providers|"model_providers"|\x27model_providers\x27)\s*\]'
      $managed = $line -cmatch '^\[\s*(model_providers|"model_providers"|\x27model_providers\x27)\s*\.\s*(xingqiao|"xingqiao"|\x27xingqiao\x27)\s*(\]|\.)'
    }
    $key = (($line -split '=', 2)[0].Trim() -replace '["\x27]', '')
    if ($free -and $line.Contains('=') -and (($root -and ($key -ceq 'model_providers' -or $key -cmatch '^model_providers\.xingqiao(\.|$)')) -or ($parent -and $key -ceq 'xingqiao'))) { throw 'Unsupported inline provider definition. Original config is preserved.' }
    if ($free -and $root -and $key -ceq 'model_provider') {
      $value = (($line -split '=', 2)[1] -replace '\s*(#.*)?$', '').Trim()
      if ($value -notmatch '^["\x27][A-Za-z0-9_-]+["\x27]$') { throw 'Unsupported provider selector. Original config is preserved.' }
      $value = $value -replace '["\x27]', ''
      if ($value -ceq 'openai') { $state = 'official' } elseif ($value -ceq 'xingqiao') { $state = 'xingqiao' } else { $state = 'custom' }
    }
    if ($free -and $root -and $key -ceq 'openai_base_url') { $customUrl = $true }
    $selected = $free -and $root -and $line.Contains('=') -and $key -cin @('model_provider','model','review_model','model_catalog_json','openai_base_url')
    $q = ''
    for ($i = 0; $i -lt $raw.Length; $i++) {
      $c = [string]$raw[$i]; $triple = ''
      if ($i + 3 -le $raw.Length) { $triple = $raw.Substring($i, 3) }
      if ($multi.Length -gt 0) {
        if ($multi -eq '"""' -and $c -eq '\') { $i++; continue }
        if ($triple -eq $multi) { $multi = ''; $i += 2 }
      } elseif ($q.Length -gt 0) {
        if ($q -eq '"' -and $c -eq '\') { $i++; continue }
        if ($q -eq $c) { $q = '' }
      } else {
        if ($c -eq '#') { break }
        if ($triple -eq '"""' -or $triple -eq "'''") { $multi = $triple; $i += 2 }
        elseif ($c -eq '"' -or $c -eq "'") { $q = $c }
      }
    }
    if ($selected -and $multi.Length -gt 0) { throw 'Cannot safely edit a multiline connection selector. Original config is preserved.' }
    if ($free -and $raw.TrimEnd("`r", "`n") -eq '# Xingqiao managed provider') { continue }
    if (($mode -eq 'roots' -and $selected) -or ($mode -eq 'body' -and !$selected -and !$managed) -or ($mode -eq 'provider' -and $managed)) { $lines.Add($raw) }
  }
  if ($multi.Length -gt 0) { throw 'Unterminated multiline TOML string. Original config is preserved.' }
  if ($mode -eq 'state') { if ($customUrl) { return 'custom' }; return $state }
  return [string]::Join('', $lines)
}
function Publish-File([string]$source, [string]$destination) {
  if ([IO.File]::Exists($destination)) {
    [IO.File]::Replace($source, $destination, [System.Management.Automation.Language.NullString]::Value)
  } else { [IO.File]::Move($source, $destination) }
}

try {
  [Console]::WriteLine($messages.menu)
  do {
    $choice = Read-Host $messages.prompt
    if ($choice -eq '0' -or $null -eq $choice -or ([Console]::IsInputRedirected -and $choice -eq '')) { exit 0 }
  } while ($choice -notin @('1','2'))
  $configDir = $env:CODEX_HOME
  if ([string]::IsNullOrWhiteSpace($configDir)) { $configDir = [IO.Path]::Combine([Environment]::GetFolderPath('UserProfile'), '.codex') }
  $configDir = [IO.Path]::GetFullPath($configDir)
  Check-Target $configDir $true
  [void][IO.Directory]::CreateDirectory($configDir)
  $configFile = [IO.Path]::Combine($configDir, 'config.toml')
  $catalogFile = [IO.Path]::Combine($configDir, '.xingqiao', 'models.json')
  $stateDir = [IO.Path]::Combine($configDir, '.xingqiao')
  $backupDir = [IO.Path]::Combine($configDir, 'xingqiao-backups')
  $officialFile = [IO.Path]::Combine($stateDir, 'official.toml')
  Check-Target $configFile $false
  Check-Target $stateDir $true
  Check-Target $officialFile $false
  if ($choice -eq '1' -and $catalogPayload) { Check-Target $catalogFile $false }
  Private-Directory $stateDir
  Private-Directory $backupDir
  $lockPath = [IO.Path]::Combine($configDir, '.xingqiao-lock')
  # CreateNew is atomic and refuses an existing lock, including a link.
  try { $lock = [IO.File]::Open($lockPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None) }
  catch { throw "Another setup may be running. Confirm it has stopped before removing: $lockPath" }
  $stage = [IO.Path]::Combine($stateDir, ('tmp.' + [Guid]::NewGuid().ToString('N')))
  Private-Directory $stage
  $current = ''; if ([IO.File]::Exists($configFile)) { $current = [IO.File]::ReadAllText($configFile) }
  $body = Transform-Toml $current 'body'
  $currentRoots = Transform-Toml $current 'roots'
  $currentState = Transform-Toml $current 'state'
  $saveOfficial = $false
  if ($currentState -eq 'official') {
    if ($current.Length -gt 0) { $official = $current } else { $official = "model_provider = `"openai`"`n" }
    $saveOfficial = $true
  } elseif ([IO.File]::Exists($officialFile)) { $official = [IO.File]::ReadAllText($officialFile) }
  else { $official = "model_provider = `"openai`"`n" + $body; $saveOfficial = $true }

  if ($choice -eq '1') {
    $payload = Decode-Text $configPayload
    $new = Transform-Toml $payload 'roots'
    if ($catalogPayload) {
      [IO.File]::WriteAllBytes([IO.Path]::Combine($stage, 'catalog.json'), [Convert]::FromBase64String($catalogPayload))
      $catalogPath = $catalogFile.Replace('\', '\\').Replace('"', '\"')
      if ($catalogPath -match '[\r\n\t]') { throw 'Control characters in CODEX_HOME are not supported.' }
      $new += 'model_catalog_json = "' + $catalogPath + '"' + "`n"
    }
    $new += $body
    if ($new.Length -gt 0 -and !$new.EndsWith("`n")) { $new += "`n" }
    $new += "# Xingqiao managed provider`n" + (Transform-Toml $payload 'provider')
  } else {
    $officialBody = Transform-Toml $official 'body'
    if ($body -ceq $officialBody) { $new = $official }
    else { $new = (Transform-Toml $official 'roots') + $body }
  }
  $configTemp = [IO.Path]::Combine($stage, 'config.toml'); Write-Text $configTemp $new
  $backup = [IO.Path]::Combine($backupDir, ([DateTime]::Now.ToString('yyyyMMdd-HHmmss') + '.' + [Guid]::NewGuid().ToString('N')))
  Private-Directory $backup
  if ([IO.File]::Exists($configFile)) { [IO.File]::Copy($configFile, [IO.Path]::Combine($backup, 'config.toml')) }
  if ($choice -eq '1' -and $catalogPayload -and [IO.File]::Exists($catalogFile)) { [IO.File]::Copy($catalogFile, [IO.Path]::Combine($backup, 'xingqiao-models.json')) }
  if ($saveOfficial) {
    $officialTemp = [IO.Path]::Combine($stage, 'official.toml'); Write-Text $officialTemp $official
    Publish-File $officialTemp $officialFile
  }
  if ($choice -eq '1' -and $catalogPayload) {
    $catalogChanged = $true
    Publish-File ([IO.Path]::Combine($stage, 'catalog.json')) $catalogFile
  }
  $configChanged = $true; Publish-File $configTemp $configFile
  $configChanged = $false; $catalogChanged = $false
  [Console]::WriteLine($messages.success)
  [Console]::WriteLine($messages.backup + $backup)
  if ($choice -eq '2') { [Console]::WriteLine($messages.official) }
  $status = 0
} catch {
  $failure = $_.Exception.Message
  if ($configChanged) {
    try {
      $saved = [IO.Path]::Combine($backup, 'config.toml')
      if ([IO.File]::Exists($saved)) { [IO.File]::Copy($saved, $configFile, $true) } else { [IO.File]::Delete($configFile) }
    } catch { $failure += "`nConfig rollback failed. Backup: $backup. $($_.Exception.Message)" }
  }
  if ($catalogChanged) {
    try {
      $saved = [IO.Path]::Combine($backup, 'xingqiao-models.json')
      if ([IO.File]::Exists($saved)) { [IO.File]::Copy($saved, $catalogFile, $true) } else { [IO.File]::Delete($catalogFile) }
    } catch { $failure += "`nCatalog rollback failed. Backup: $backup. $($_.Exception.Message)" }
  }
  [Console]::WriteLine($messages.failure + "`n" + $failure)
  if ($backup) { [Console]::WriteLine($messages.backup + $backup) }
} finally {
  if ($stage -and [IO.Directory]::Exists($stage)) { try { [IO.Directory]::Delete($stage, $true) } catch { } }
  if ($lock -is [IO.FileStream]) { $lock.Dispose(); [IO.File]::Delete($lockPath) }
}
if (![Console]::IsInputRedirected) { [void](Read-Host $messages.close) }
exit $status
