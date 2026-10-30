# Test release key

`test-release.key.pem` signs the releases OffLok's update tests make up. It is public on purpose and worth nothing:
only binaries built with `-tags offlok_testkey` trust it (`internal/update/testkey.go`), and no release is built
that way. Released binaries trust the release key alone (`packaging/release/`).
