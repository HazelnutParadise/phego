# PHP 8.5.12 behaviour for Windows device names and directories, for
# comparison with Phego's Root mounts (W-20). The cases match
# TestWindowsSpecialFileReport in internal/policy.
# Usage: probe-windows.ps1 -Php <php.exe> -Work <empty directory> -Probe <probe.php>
# Each case prints PHP's output and exit status; "exit=timeout" means it was
# still waiting after 3 seconds.
param(
    [Parameter(Mandatory)] [string] $Php,
    [Parameter(Mandatory)] [string] $Work,
    [Parameter(Mandatory)] [string] $Probe
)
$ErrorActionPreference = 'Stop'

function Run([string] $Name, [string[]] $Arguments) {
    "== $Name"
    $psi = [System.Diagnostics.ProcessStartInfo]::new($Php)
    foreach ($a in @('-n', '-d', 'display_errors=stderr', '-d', 'error_reporting=-1') + $Arguments) {
        $psi.ArgumentList.Add($a)
    }
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.RedirectStandardInput = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    $p.StandardInput.Close()
    $out = $p.StandardOutput.ReadToEndAsync()
    $err = $p.StandardError.ReadToEndAsync()
    if (-not $p.WaitForExit(3000)) {
        $p.Kill($true)
        $p.WaitForExit()
        $err.Result.TrimEnd()
        $out.Result.TrimEnd()
        'exit=timeout'
        return
    }
    $err.Result.TrimEnd()
    $out.Result.TrimEnd()
    "exit=$($p.ExitCode)"
}

"== system: $([System.Environment]::OSVersion.VersionString) $env:PROCESSOR_ARCHITECTURE"
Run 'version' @('-r', 'echo PHP_VERSION, " ", PHP_OS, "\n";')
New-Item -ItemType Directory -Path (Join-Path $Work 'd') | Out-Null
New-Item -ItemType Directory -Path (Join-Path $Work 'sub') | Out-Null
Run 'fopen NUL r' @($Probe, 'r', 'NUL', 'read')
Run 'fopen NUL w' @($Probe, 'w', 'NUL', 'write')
Run 'file_get_contents NUL' @($Probe, 'fgc', 'NUL')
Run 'include NUL' @($Probe, 'include', 'NUL')
Run 'php NUL' @('NUL')
foreach ($name in 'NUL', 'nul', 'NUL.txt', 'sub\NUL', 'CON', 'COM1', 'CONIN$', 'CONOUT$', 'LPT1', 'NUL\x') {
    Run "fopen <work>\$name r" @($Probe, 'r', (Join-Path $Work $name), 'read')
}
foreach ($mode in 'w', 'r+', 'a', 'x') {
    Run "fopen <work>\NUL $mode" @($Probe, $mode, (Join-Path $Work 'NUL'), 'write')
}
Run 'fopen <work>\sub\CON w' @($Probe, 'w', (Join-Path $Work 'sub\CON'), 'write')
Run 'include <work>\NUL' @($Probe, 'include', (Join-Path $Work 'NUL'))
Run 'php <work>\NUL' @((Join-Path $Work 'NUL'))
foreach ($mode in 'r', 'w', 'r+', 'a') {
    Run "fopen dir $mode" @($Probe, $mode, (Join-Path $Work 'd'))
}
Run 'php dir' @((Join-Path $Work 'd'))
foreach ($mode in 'r', 'w') {
    Run "fopen <work> $mode" @($Probe, $mode, $Work)
}
# Alternate data streams: Root mounts refuse any name with a colon.
Run 'fopen <work>\ads:s r' @($Probe, 'r', (Join-Path $Work 'ads:s'), 'read')
Run 'fopen <work>\ads:s w' @($Probe, 'w', (Join-Path $Work 'ads:s'), 'write')
Run 'fopen missing named pipe r' @($Probe, 'r', '\\.\pipe\phego-missing')
