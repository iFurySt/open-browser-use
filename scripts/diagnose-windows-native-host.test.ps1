$ErrorActionPreference = 'Stop'
. "$PSScriptRoot/diagnose-windows-native-host.ps1"
function Assert($condition, [string]$message) {
    if (-not $condition) { throw $message }
}

# Exercise actual Unicode registry values and missing keys on Windows, without
# touching the real Chrome registration. Fixtures live under a unique test key.
if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
    $fixtureKey = 'Software\OpenBrowserUseTests\' + [guid]::NewGuid().ToString('N')
    try {
        foreach ($view in @('Registry32', 'Registry64')) {
            $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser, [Microsoft.Win32.RegistryView]::$view)
            try {
                $key = $base.CreateSubKey($fixtureKey)
                try {
                    $key.SetValue('', 'C:\测试 用户\manifest.json', [Microsoft.Win32.RegistryValueKind]::String)
                    $key.SetValue('Disabled', 0, [Microsoft.Win32.RegistryValueKind]::DWord)
                } finally { $key.Dispose() }
                $read = Read-NativeRegistryValue CurrentUser $view $fixtureKey
                Assert ($read.State -eq 'present' -and $read.Value -ceq 'C:\测试 用户\manifest.json') 'Unicode registry path did not round-trip.'
                $read = Read-NativeRegistryValue CurrentUser $view $fixtureKey Disabled
                Assert ($read.Kind -eq 'DWord' -and $read.Value -eq 0) 'DWORD policy did not round-trip.'
                $read = Read-NativeRegistryValue CurrentUser $view "$fixtureKey\missing"
                Assert ($read.State -eq 'missing') 'Missing registry key was not reported.'
            } finally { $base.Dispose() }
        }
    } finally {
        foreach ($view in @('Registry32', 'Registry64')) {
            $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser, [Microsoft.Win32.RegistryView]::$view)
            try { $base.DeleteSubKeyTree($fixtureKey, $false) } finally { $base.Dispose() }
        }
    }
}

$script:values = @{}
function Read-NativeRegistryValue {
    param([string]$Hive, [string]$View, [string]$Path, [string]$Name = '')
    $key = "$Hive|$View|$Path|$Name"
    if ($script:values.ContainsKey($key)) { return $script:values[$key] }
    return [pscustomobject]@{ State = 'missing'; Value = $null; Kind = $null }
}
function Set-FixtureValue($hive, $view, $path, $name, $value, $kind = 'String') {
    $script:values["$hive|$view|$path|$name"] = [pscustomobject]@{ State = 'present'; Value = $value; Kind = $kind }
}
$extension = 'bgjoihaepiejlfjinojjfgokghnodnhd'
$hostName = 'com.ifuryst.open_browser_use.extension'
$hostKey = "Software\Google\Chrome\NativeMessagingHosts\$hostName"
$temp = Join-Path ([System.IO.Path]::GetTempPath()) ('obu-diagnosis-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $manifestPath = Join-Path $temp 'manifest.json'
    $exe = Join-Path $temp 'host.exe'
    Set-Content -LiteralPath $exe -Value 'fixture; never executed'
    $manifest = @{ name = $hostName; type = 'stdio'; description = 'test'; path = 'host.exe'; allowed_origins = @("chrome-extension://$extension/") }
    $manifest | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding UTF8

    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].Problems.Count -gt 0) 'Missing registration should be diagnosed.'

    Set-FixtureValue CurrentUser Registry64 $hostKey '' $manifestPath
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].Problems.Count -eq 0) 'Valid registration and relative executable should pass static checks.'
    Assert ($report.SelectionByPolicy[1].Problems.Count -gt 0) 'Disabling user-level hosts must ignore HKCU.'

    Set-FixtureValue CurrentUser Registry32 $hostKey '' (Join-Path $temp 'missing.json')
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].SelectedRegistration.View -eq 'Registry32') '32-bit registration must be selected first.'
    Assert ($report.SelectionByPolicy[0].Problems[0] -like '*missing manifest*') 'A broken earlier registration must not fall back to a good later one.'

    Set-FixtureValue CurrentUser Registry32 $hostKey '' $manifestPath
    Set-FixtureValue LocalMachine Registry64 $hostKey '' $manifestPath
    Set-FixtureValue LocalMachine Registry64 'Software\Policies\Google\Chrome' NativeMessagingUserLevelHosts 0 DWord
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.Findings -match 'NativeMessagingUserLevelHosts=0') 'Disabled user-level policy should be explained.'
    Assert ($report.SelectionByPolicy[1].SelectedRegistration.Hive -eq 'LocalMachine') 'Machine registration must be selected when user-level hosts are disabled.'

    $manifest.allowed_origins = "chrome-extension://$extension/"
    $manifest | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding UTF8
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].Problems -match 'allowed_origins') 'An origin string instead of an array is invalid.'

    $manifest.allowed_origins = @('chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/')
    $manifest | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding UTF8
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].Problems -match 'allowed_origins') 'Wrong extension origin should be diagnosed.'

    $manifest.allowed_origins = @("chrome-extension://$extension/")
    $manifest.path = 'missing.exe'
    $manifest.name = 'wrong.name'
    $manifest.type = 'invalid'
    $manifest | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding UTF8
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].Problems.Count -eq 3) 'Missing executable and invalid name/type should all be reported.'

    Set-Content -LiteralPath $manifestPath -Value 'invalid json'
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].Problems -match 'parse') 'Invalid JSON should be diagnosed.'

    Set-FixtureValue CurrentUser Registry32 $hostKey '' ('"' + $manifestPath + '"')
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.SelectionByPolicy[0].Problems -match 'surrounding quotes') 'Quoted registry paths should be diagnosed.'

    $script:values["CurrentUser|Registry32|$hostKey|"] = [pscustomobject]@{ State = 'error'; Value = $null; Kind = $null; Error = 'fixture access denied' }
    $report = Get-NativeHostDiagnosis $extension
    Assert ($report.Findings -match 'provisional') 'Read errors must not be silently treated as missing entries.'
    Write-Output 'Windows native host diagnostic tests passed.'
} finally {
    Remove-Item -LiteralPath $temp -Recurse -Force
}
