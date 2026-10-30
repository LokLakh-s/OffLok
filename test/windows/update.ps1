# OffLok's update on Windows, from end to end, on a disposable machine (GitHub's runner), as an administrator.
# Test release A (a test build, which trusts test\release\'s key and reads OFFLOK_UPDATE_BASE) is installed over a
# ProgramData\OffLok that someone prepared; it finds release B on a local server, checks its signature and hash
# and runs its installer as SYSTEM; the service, the command and the tray app come back as B. Then B is
# uninstalled. The builds come from test/update-builds.sh windows, in dist\update-test.
. "$PSScriptRoot\lib.ps1"

$u = 'dist\update-test'
$A = (Get-Content "$u\a.version").Trim()
$B = (Get-Content "$u\b.version").Trim()
$dir = Join-Path $env:ProgramFiles 'OffLok'
$offlok = Join-Path $dir 'offlok.exe'
$state = Join-Path $env:ProgramData 'OffLok'
$before = Get-DnsSnapshot

# Before OffLok: a folder in its place owned by someone else (as a user could make one: ProgramData lets every user
# create folders), and a junction to a folder elsewhere, whose content must survive.
$target = Join-Path $env:RUNNER_TEMP 'offlok-junction-target'
New-Item -ItemType Directory -Force $state, "$state\planted", $target | Out-Null
Set-Content "$target\keep.txt" 'not OffLok''s'
icacls "$state\planted" /setowner '*S-1-5-11' | Out-Null # Authenticated Users
cmd /c mklink /J "$state\lists" $target | Out-Null
"# planted: owned by $((Get-Acl "$state\planted").Owner); lists: $((Get-Item "$state\lists").Attributes)"

Start-Process "$u\offlok-$A-setup.exe" -ArgumentList '/S' -Wait
Check 'A resolves' { Wait-Until 60 { (& $offlok status) -match '^(On|Fallback):' } }
Check 'its programs carry the product name and version' {
    @('offlokd.exe', 'offlok.exe', 'offlok-desktop.exe' | ForEach-Object {
        $v = (Get-Item (Join-Path $dir $_)).VersionInfo
        $v.ProductName -eq 'OffLok' -and $v.ProductVersion -eq $A
    }) -notcontains $false
}
Check 'what someone else owned in ProgramData\OffLok is gone' { -not (Test-Path "$state\planted") }
Check 'the junction is gone, not what it pointed at' {
    (Test-Path "$target\keep.txt") -and -not ((Get-Item "$state\lists" -ErrorAction SilentlyContinue).Attributes -band [IO.FileAttributes]::ReparsePoint)
}
Check 'ProgramData\OffLok is protected: users may only read it' {
    $acl = Get-Acl $state
    # Write data, append, write attributes and extended attributes, delete, delete children, permissions, owner.
    $writes = 0x2 -bor 0x4 -bor 0x10 -bor 0x40 -bor 0x100 -bor 0x10000 -bor 0x40000 -bor 0x80000
    $others = @($acl.Access | Where-Object {
        $sid = $_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        $sid -notin @('S-1-5-18', 'S-1-5-32-544') -and ([int]$_.FileSystemRights -band $writes)
    })
    $acl.AreAccessRulesProtected -and $others.Count -eq 0
}

# The release server: B's installer and its SHA256SUMS, signed with the test key.
$web = Join-Path $env:RUNNER_TEMP 'web'
$latest = "$web\releases\latest\download"
$release = "$web\releases\download\v$B"
New-Item -ItemType Directory -Force $latest, $release | Out-Null
Copy-Item "$u\offlok-$B-setup.exe" $release
$hash = (Get-FileHash "$release\offlok-$B-setup.exe" -Algorithm SHA256).Hash.ToLower()
[IO.File]::WriteAllText("$latest\SHA256SUMS", "$hash  offlok-$B-setup.exe`n")
go run ./test/release/sign test/release/test-release.key.pem "$latest\SHA256SUMS"
$server = Start-Process python -ArgumentList '-m', 'http.server', '8088', '--bind', '127.0.0.1', '--directory', $web -PassThru -WindowStyle Hidden
# The service takes its releases from there: test builds alone read OFFLOK_UPDATE_BASE.
Set-ItemProperty HKLM:\SYSTEM\CurrentControlSet\Services\OffLok -Name Environment -Type MultiString -Value 'OFFLOK_UPDATE_BASE=http://127.0.0.1:8088/releases'
Restart-Service OffLok
Wait-Until 60 { (& $offlok status) -match '^(On|Fallback):' } | Out-Null

Start-Process (Join-Path $dir 'offlok-desktop.exe') -ArgumentList '--hidden'
Check 'the tray app runs' { Wait-Until 20 { [bool](Get-Process offlok-desktop -ErrorAction SilentlyContinue) } }
$updating = Get-Date
Check 'B is offered' { (& $offlok update) -match "OffLok $B" }
# The status's System line names offlokd's version; other lines name the update's.
Check 'the service comes back as B' { Wait-Until 240 { (& $offlok status) -match "OffLok $B, Unbound" } }
Check 'B resolves' { Wait-Until 60 { (& $offlok status) -match '^(On|Fallback):' } }
Check 'the command is B' { (& $offlok version) -eq $B }
Check 'the tray app started again, from B' {
    Wait-Until 60 {
        $p = @(Get-Process offlok-desktop -ErrorAction SilentlyContinue)
        $p.Count -eq 1 -and $p[0].StartTime -gt $updating
    }
}
Check 'programs resolve through the system' { [bool](Resolve-DnsName nlnetlabs.nl -Type A -DnsOnly) }
"# $((& $offlok status) -join ' | ')"

Stop-Process $server
Start-Process (Join-Path $dir 'uninstall.exe') -ArgumentList '/S'
Check 'uninstall removes the service' { Wait-Until 90 { -not (Get-Service OffLok -ErrorAction SilentlyContinue) } }
Check 'uninstall removes every file, the set-aside ones too' { Wait-Until 30 { -not (Test-Path $dir) } }
Check 'every interface has its original DNS back' { -not (Compare-Object $before (Get-DnsSnapshot)) }

Summary
