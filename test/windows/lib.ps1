# Helpers shared by OffLok's Windows tests (dot-sourced, never run).
$ErrorActionPreference = 'Stop'
$script:pass = 0
$script:fail = 0

function Check([string]$what, [scriptblock]$test) {
    try { $ok = & $test } catch { $ok = $false; $what += " ($_)" }
    if ($ok) { "ok   - $what"; $script:pass++ } else { "FAIL - $what"; $script:fail++ }
}

function Wait-Until([int]$seconds, [scriptblock]$test) {
    for ($i = 0; $i -lt $seconds; $i++) {
        try { if (& $test) { return $true } } catch {}
        Start-Sleep -Seconds 1
    }
    return $false
}

# Every interface's DNS servers, as Windows reports them: what an uninstall must give back.
function Get-DnsSnapshot {
    Get-DnsClientServerAddress | Where-Object { $_.InterfaceAlias -notlike 'Loopback*' } |
        ForEach-Object { "$($_.InterfaceIndex)/$($_.AddressFamily)=$($_.ServerAddresses -join ',')" } | Sort-Object
}

function Summary {
    "# $script:pass passed, $script:fail failed"
    if ($script:fail -gt 0) { exit 1 }
}
