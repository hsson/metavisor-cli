//    Copyright 2018 Immutable Systems, Inc.
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

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
