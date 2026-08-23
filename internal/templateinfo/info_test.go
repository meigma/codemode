package templateinfo

import "testing"

func TestSummary(t *testing.T) {
	t.Parallel()

	got := Summary()
	if got == "" {
		t.Fatal("Summary returned an empty string")
	}
}
