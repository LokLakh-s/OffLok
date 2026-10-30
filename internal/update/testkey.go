//go:build offlok_testkey

package update

import "os"

// Test builds (-tags offlok_testkey, never released) also trust test/release/test-release.key.pem, which is public,
// and take their releases from $OFFLOK_UPDATE_BASE: the update tests serve made-up releases.
func init() {
	builtinKeys["test"] = "LoMxaWayxvxh8ck0DKOnDmEU+rOlocbh63mqRoVHLGg="
	if b := os.Getenv("OFFLOK_UPDATE_BASE"); b != "" {
		DefaultBase = b
	}
}
