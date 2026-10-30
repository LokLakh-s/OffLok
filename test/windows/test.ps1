# OffLok's Windows test, on a disposable Windows machine (GitHub's runner), as an administrator: installs
# dist\offlok-<version>-setup.exe silently, checks the service, the system DNS and the blocking, uninstalls, and
# checks that every interface has its original DNS back.
. "$PSScriptRoot\lib.ps1"

$setup = Get-ChildItem dist\offlok-*-setup.exe | Select-Object -First 1
$before = Get-DnsSnapshot
"# DNS before: $($before -join '; ')"

Start-Process $setup.FullName -ArgumentList '/S' -Wait
$dir = Join-Path $env:ProgramFiles 'OffLok'
$offlok = Join-Path $dir 'offlok.exe'

Check 'the service runs' { (Get-Service OffLok).Status -eq 'Running' }
Check 'it starts at boot' { (Get-Service OffLok).StartType -eq 'Automatic' }
Check 'offlok is in the PATH' { ([Environment]::GetEnvironmentVariable('Path', 'Machine') -split ';') -contains $dir }
Check 'resolves by itself, or falls back on this network' {
    Wait-Until 60 { (& $offlok status) -match '^(On|Fallback):' }
}
"# $((& $offlok status) -join ' | ')"
"# DNS while on: $((Get-DnsSnapshot) -join '; ')"
Check 'the interfaces point at OffLok' {
    $v4 = @(Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notlike 'Loopback*' -and $_.ServerAddresses.Count -gt 0 })
    $v4.Count -gt 0 -and @($v4 | Where-Object { $_.ServerAddresses -notcontains '127.0.0.1' }).Count -eq 0
}
# An interface's IPv6 DNS left to the network would let Windows go around OffLok.
Check 'their IPv6 DNS too' {
    $set = @(Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object { $_.InterfaceAlias -notlike 'Loopback*' -and $_.ServerAddresses -contains '127.0.0.1' }).InterfaceIndex
    $v6 = @(Get-DnsClientServerAddress -AddressFamily IPv6 | Where-Object { $_.InterfaceIndex -in $set })
    $v6.Count -gt 0 -and @($v6 | Where-Object { ($_.ServerAddresses -join ',') -ne '::1' }).Count -eq 0
}
Check 'programs resolve through the system' { [bool](Resolve-DnsName nlnetlabs.nl -Type A -DnsOnly) }
Check 'a broken DNSSEC signature is refused' {
    try { Resolve-DnsName dnssec-failed.org -Type A -Server 127.0.0.1 -DnsOnly | Out-Null; $false } catch { $true }
}
# Every socket on the DNS ports, with its process: another program's (the runner's own) is shown, not counted.
$ours = @(Get-Process offlokd, unbound -ErrorAction SilentlyContinue).Id
$sockets = @(Get-NetUDPEndpoint -LocalPort 53, 53530 -ErrorAction SilentlyContinue) +
    @(Get-NetTCPConnection -LocalPort 53, 53530 -State Listen -ErrorAction SilentlyContinue)
"# DNS sockets: $(($sockets | ForEach-Object { "$($_.LocalAddress):$($_.LocalPort) $((Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).Name)" }) -join '; ')"
Check 'OffLok answers DNS on the loopback only' {
    $mine = @($sockets | Where-Object { $_.OwningProcess -in $ours })
    $mine.Count -gt 0 -and @($mine | Where-Object { $_.LocalAddress -notin @('127.0.0.1', '::1') }).Count -eq 0
}
Check 'Unbound runs next to offlokd' { [bool](Get-Process unbound -ErrorAction SilentlyContinue) }

& $offlok block ads on | Out-Null
Check 'ads: the list is downloaded' { Wait-Until 90 { (& $offlok status) -match 'ads \(\d{5,} names\)' } }
Check 'ads: doubleclick.net is blocked' {
    (Resolve-DnsName doubleclick.net -Type A -Server 127.0.0.1 -DnsOnly).IPAddress -contains '0.0.0.0'
}
Check 'check names the list' { (& $offlok check doubleclick.net) -match 'AdGuard DNS filter' }
& $offlok block ads off | Out-Null

Start-Process (Join-Path $dir 'offlok-desktop.exe') -ArgumentList '--hidden'
Start-Sleep -Seconds 8
Check 'the tray app starts and stays up' { [bool](Get-Process offlok-desktop -ErrorAction SilentlyContinue) }

# NSIS's uninstaller relaunches itself from %TEMP%: wait for its work instead of for the process.
Start-Process (Join-Path $dir 'uninstall.exe') -ArgumentList '/S'
Check 'uninstall removes the service' { Wait-Until 90 { -not (Get-Service OffLok -ErrorAction SilentlyContinue) } }
Check 'uninstall removes the files' { Wait-Until 30 { -not (Test-Path (Join-Path $dir 'offlokd.exe')) } }
Check 'unbound is gone' { -not (Get-Process unbound -ErrorAction SilentlyContinue) }
Check 'offlok is out of the PATH' { ([Environment]::GetEnvironmentVariable('Path', 'Machine') -split ';') -notcontains $dir }
$after = Get-DnsSnapshot
"# DNS after: $($after -join '; ')"
Check 'every interface has its original DNS back' { -not (Compare-Object $before $after) }

Summary
