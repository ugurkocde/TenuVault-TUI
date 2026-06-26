package restore

import (
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

func TestPlanPassthrough(t *testing.T) {
	items := []Item{{Category: "DeviceConfigurations", Name: "a"}}
	if got := Plan(store.Backup{}, items); len(got) != 1 {
		t.Fatalf("Plan returned %d items, want 1", len(got))
	}
}
