# OffLok's Windows test, on a disposable Windows machine (GitHub's runner), as an administrator: installs
# dist\offlok-<version>-setup.exe silently, checks the service, the system DNS, Unbound's account (LocalService),
# the blocking and the fallback to a DNS-over-TLS resolver behind the firewall, uninstalls, and
# checks that every interface has its original DNS back. Unbound's usual port is held by another program first, as
# a range Hyper-V, WSL or Docker reserve at boot may hold it: OffLok must take a spare one.
. "$PSScriptRoot\lib.ps1"

$setup = Get-ChildItem dist\offlok-*-setup.exe | Select-Object -First 1
$before = Get-DnsSnapshot
"# DNS before: $($before -join '; ')"
# A range Windows reserved may already hold the port: then nothing can take it, which does as well.
try { $holder = [Net.Sockets.UdpClient]::new([Net.IPEndPoint]::new([Net.IPAddress]::Loopback, 53530)) } catch { $holder = $null }

Start-Process $setup.FullName -ArgumentList '/S' -Wait
$dir = Join-Path $env:ProgramFiles 'OffLok'
$offlok = Join-Path $dir 'offlok.exe'

Check 'the service runs' { (Get-Service OffLok).Status -eq 'Running' }
Check 'it starts at boot' { (Get-Service OffLok).StartType -eq 'Automatic' }
Check 'offlok is in the PATH' { ([Environment]::GetEnvironmentVariable('Path', 'Machine') -split ';') -contains $dir }
Check 'resolves by itself, or falls back on this network' {
    Wait-Until 60 { (& $offlok status) -match '^(On|Fallback):' }
}
$port = [int]((Get-Content "$env:ProgramData\OffLok\unbound\unbound.conf" | Select-String '^\s*port:\s*(\d+)').Matches[0].Groups[1].Value)
Check "Unbound's usual port taken, it takes a spare one ($port)" { $port -ne 53530 -and $port -gt 0 }
if ($holder) { $holder.Dispose() }
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
$sockets = @(Get-NetUDPEndpoint -LocalPort 53, $port -ErrorAction SilentlyContinue) +
    @(Get-NetTCPConnection -LocalPort 53, $port -State Listen -ErrorAction SilentlyContinue)
"# DNS sockets: $(($sockets | ForEach-Object { "$($_.LocalAddress):$($_.LocalPort) $((Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).Name)" }) -join '; ')"
Check 'OffLok answers DNS on the loopback only' {
    $mine = @($sockets | Where-Object { $_.OwningProcess -in $ours })
    $mine.Count -gt 0 -and @($mine | Where-Object { $_.LocalAddress -notin @('127.0.0.1', '::1') }).Count -eq 0
}
Check 'Unbound runs next to offlokd' { [bool](Get-Process unbound -ErrorAction SilentlyContinue) }
# A flaw in Unbound, which reads what any server on the internet sends, must not give SYSTEM away.
$owner = Get-CimInstance Win32_Process -Filter "Name = 'unbound.exe'" | Invoke-CimMethod -MethodName GetOwner
"# Unbound's account: $($owner.Domain)\$($owner.User)"
Check 'Unbound runs as LocalService, not SYSTEM' { $owner.Domain -eq 'NT AUTHORITY' -and $owner.User -eq 'LOCAL SERVICE' }
Check 'Unbound keeps the root trust anchor itself (it may write root.key)' {
    Wait-Until 60 { (Get-Content "$env:ProgramData\OffLok\unbound\keys\root.key" -Raw) -match 'autotrust trust anchor file' }
}

& $offlok block ads on | Out-Null
Check 'ads: the list is downloaded' { Wait-Until 90 { (& $offlok status) -match 'ads \(\d{5,} names\)' } }
# The list counts once downloaded; it blocks once compiled, a moment later.
Check 'ads: doubleclick.net is blocked' {
    Wait-Until 15 { (Resolve-DnsName doubleclick.net -Type A -Server 127.0.0.1 -DnsOnly).IPAddress -contains '0.0.0.0' }
}
Check 'check names the list' { (& $offlok check doubleclick.net) -match 'AdGuard DNS filter' }
& $offlok block ads off | Out-Null

# The root servers cut off by the firewall: the personal DNS-over-TLS resolver, whose certificate Unbound checks
# against Windows's store, as LocalService. The firewall's state comes back as it was.
Check 'a DNS-over-TLS resolver is set' { (& $offlok dot dns.quad9.net) -and $LASTEXITCODE -eq 0 }
$profiles = Get-NetFirewallProfile | Select-Object Name, Enabled
Set-NetFirewallProfile -All -Enabled True
foreach ($proto in 'UDP', 'TCP') {
    New-NetFirewallRule -DisplayName "OffLok test: no $proto DNS out" -Direction Outbound -Protocol $proto `
        -RemotePort 53 -Action Block | Out-Null
}
Check 'the root cut off: through the DNS-over-TLS resolver' {
    Wait-Until 120 { (& $offlok status) -match 'using your DNS-over-TLS resolver' }
}
"# $((& $offlok status) -join ' | ')"
Check 'resolves through it' { Wait-Until 30 { [bool](Resolve-DnsName example.org -Type A -Server 127.0.0.1 -DnsOnly) } }
Get-NetFirewallRule -DisplayName 'OffLok test: no * DNS out' | Remove-NetFirewallRule
foreach ($p in $profiles) { Set-NetFirewallProfile -Name $p.Name -Enabled $p.Enabled }
& $offlok dot none | Out-Null
Check 'the root back, no resolver set: as before' {
    Wait-Until 120 { $st = & $offlok status; ($st -match '^(On|Fallback):') -and -not ($st -match 'using your DNS-over-TLS') }
}

Start-Process (Join-Path $dir 'offlok-desktop.exe') -ArgumentList '--hidden'
Start-Sleep -Seconds 8
Check 'the tray app starts and stays up' { [bool](Get-Process offlok-desktop -ErrorAction SilentlyContinue) }
# Started hidden (the session's autostart), it is a tray icon alone: no web view, no WebView2 processes, until the
# window is first opened.
Check 'hidden, the tray app runs no web view' {
    $tray = @(Get-Process offlok-desktop -ErrorAction SilentlyContinue)[0].Id
    -not @(Get-CimInstance Win32_Process -Filter "Name = 'msedgewebview2.exe'" | Where-Object { $_.ParentProcessId -eq $tray })
}

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
