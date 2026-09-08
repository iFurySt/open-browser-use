# Read-only diagnostics. Dot-source this file to use the functions in tests.
[CmdletBinding()]
param(
    [ValidatePattern('^[a-p]{32}$')]
    [string]$ExtensionId = 'bgjoihaepiejlfjinojjfgokghnodnhd'
)

function Read-NativeRegistryValue {
    param([string]$Hive, [string]$View, [string]$Path, [string]$Name = '')
    $base = $null
    $key = $null
    try {
        $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey(
            [Microsoft.Win32.RegistryHive]::$Hive,
            [Microsoft.Win32.RegistryView]::$View)
        $key = $base.OpenSubKey($Path, $false)
        if ($null -eq $key -or $key.GetValueNames() -notcontains $Name) {
            return [pscustomobject]@{ State = 'missing'; Value = $null; Kind = $null }
        }
        return [pscustomobject]@{
            State = 'present'
            Value = $key.GetValue($Name, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            Kind = $key.GetValueKind($Name).ToString()
        }
    } catch {
        return [pscustomobject]@{ State = 'error'; Value = $null; Kind = $null; Error = $_.Exception.Message }
    } finally {
        if ($null -ne $key) { $key.Dispose() }
        if ($null -ne $base) { $base.Dispose() }
    }
}

function Get-NativeHostDiagnosis {
    param([string]$ExtensionId)
    $hostName = 'com.ifuryst.open_browser_use.extension'
    $policyPath = 'Software\Policies\Google\Chrome'
    $hostPath = "Software\Google\Chrome\NativeMessagingHosts\$hostName"
    $findings = [System.Collections.Generic.List[string]]::new()
    $policies = @()
    # These are registry observations. chrome://policy is authoritative for
    # effective policy, including cloud policies and precedence rules.
    foreach ($hive in @('LocalMachine', 'CurrentUser')) {
        foreach ($view in @('Registry32', 'Registry64')) {
            $name = 'NativeMessagingUserLevelHosts'
            $value = Read-NativeRegistryValue $hive $view $policyPath $name
            $policies += [pscustomobject]@{ Hive = $hive; View = $view; Name = $name; Result = $value }
            if ($value.State -eq 'present' -and $value.Kind -eq 'DWord' -and $value.Value -eq 0) {
                $findings.Add('NativeMessagingUserLevelHosts=0 was found. If effective in chrome://policy, Chrome ignores the HKCU registration written by setup. Ask the administrator to allow user-level hosts or deploy an approved machine-level host.')
            }
        }
    }
    $registrations = @()
    foreach ($hive in @('CurrentUser', 'LocalMachine')) {
        foreach ($view in @('Registry32', 'Registry64')) {
            $value = Read-NativeRegistryValue $hive $view $hostPath
            $registrations += [pscustomobject]@{ Hive = $hive; View = $view; Result = $value }
        }
    }
    # Show both policy scenarios instead of guessing effective Chrome policy.
    $selections = @()
    foreach ($allowUser in @($true, $false)) {
        $eligible = @($registrations | Where-Object {
            ($allowUser -or $_.Hive -eq 'LocalMachine') -and
            $_.Result.State -eq 'present' -and $_.Result.Kind -in @('String', 'ExpandString')
        })
        $selected = $eligible | Select-Object -First 1
        $checks = [System.Collections.Generic.List[string]]::new()
        if ($null -eq $selected) {
            $checks.Add('No readable string registration found in this policy scenario.')
        } else {
            $path = [string]$selected.Result.Value
            if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
                $absolute = $path -match '^(?:[A-Za-z]:[\\/]|\\\\[^\\]+\\[^\\]+)'
            } else {
                $absolute = [System.IO.Path]::IsPathRooted($path)
            }
            if (-not $absolute -or $path.Contains('"') -or $path.IndexOfAny([System.IO.Path]::GetInvalidPathChars()) -ge 0) {
                $checks.Add('Registered manifest path must be absolute and must not include surrounding quotes.')
            } elseif (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
                $checks.Add('The selected registration points to a missing manifest; a later registration does not repair this earlier match.')
            } else {
                try {
                    $manifest = Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop
                    if ($manifest.name -cne $hostName) { $checks.Add('Manifest name does not match the registered host name.') }
                    if ($manifest.type -cne 'stdio') { $checks.Add('Manifest type must be stdio.') }
                    if ($manifest.description -isnot [string]) { $checks.Add('Manifest description must be a string.') }
                    $origin = "chrome-extension://$ExtensionId/"
                    if ($manifest.allowed_origins -isnot [array] -or @($manifest.allowed_origins) -cnotcontains $origin) { $checks.Add('Manifest allowed_origins does not contain the requested extension origin.') }
                    if ($manifest.path -isnot [string] -or [string]::IsNullOrWhiteSpace($manifest.path)) {
                        $checks.Add('Manifest executable path is missing.')
                    } else {
                        $exe = [string]$manifest.path
                        # Chrome permits paths relative to the manifest on Windows.
                        if (-not [System.IO.Path]::IsPathRooted($exe)) { $exe = Join-Path (Split-Path -Parent $path) $exe }
                        if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { $checks.Add('Manifest executable does not exist at its resolved path.') }
                    }
                } catch {
                    $checks.Add("Cannot read or parse the selected manifest: $($_.Exception.Message)")
                }
            }
        }
        $selections += [pscustomobject]@{ AllowUserLevelHosts = $allowUser; SelectedRegistration = $selected; Problems = @($checks.ToArray()) }
    }
    if (@(($registrations + $policies) | Where-Object { $_.Result.State -eq 'error' }).Count -gt 0) {
        $findings.Add('Some registry reads failed. Selection is provisional; inspect the errors before drawing conclusions.')
    }
    $findings.Add('Check chrome://policy for effective NativeMessagingUserLevelHosts, NativeMessagingBlocklist and NativeMessagingAllowlist; registry observations alone do not include cloud policies.')
    $findings.Add('Successful static checks do not prove Chrome can launch the host. Reconnect the extension and collect the corresponding Chrome native-messaging error; manual host startup tests a different path.')
    return [pscustomobject]@{
        HostName = $hostName
        ExtensionId = $ExtensionId
        RegistrationsInLookupOrder = $registrations
        PolicyObservations = $policies
        SelectionByPolicy = $selections
        Findings = @($findings.ToArray() | Select-Object -Unique)
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw 'This diagnostic command requires Windows.'
    }
    Get-NativeHostDiagnosis -ExtensionId $ExtensionId | ConvertTo-Json -Depth 10
}
