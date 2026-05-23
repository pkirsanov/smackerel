package notification

import "testing"

func TestCoreNotificationPackageHasNoNtfySpecificProductionDependency(t *testing.T) {
	assertCorePackageHasNoFutureAdapterDependency(t)
}
