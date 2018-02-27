package mv

import "testing"

func TestCleanup(t *testing.T) {
	didRun := false
	f := func() {
		didRun = true
	}
	stackedCleanups = nil
	QueueCleanup(f, true)
	Cleanup(true)
	if didRun {
		t.Error("Cleanup should only have run if failure happened")
	}
	QueueCleanup(f, true)
	Cleanup(false)
	if !didRun {
		t.Error("Cleanup never happened")
	}
	didAlwaysRun := false
	fAlways := func() {
		didAlwaysRun = true
	}
	QueueCleanup(fAlways, false)
	Cleanup(true)
	if !didAlwaysRun {
		t.Error("Cleanup function never ran")
	}
}
