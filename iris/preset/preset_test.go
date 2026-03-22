package preset

import "testing"

func TestPackageBuilds(t *testing.T) {
	t.Parallel()

	_ = ClientDefaults(nil)
}
