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

package userdata

import "testing"

func TestEmptyContainer(t *testing.T) {
	c := New()
	expected := `From nobody Tue Dec  3 19:00:57 2013
Content-Type: multipart/mixed; boundary="--===============HI-20131203==--"
MIME-Version: 1.0

----===============HI-20131203==----
`
	text := c.ToMIMEText()
	if text != expected {
		t.Errorf("unexpected text.\nGot:\n%s\nExpected:\n%s", text, expected)
	}
}

func TestSimpleText(t *testing.T) {
	c := New()
	cType := "test/some-type"
	cVal := "This is some userdata"
	c.AddPart(cType, cVal)

	expected := `From nobody Tue Dec  3 19:00:57 2013
Content-Type: multipart/mixed; boundary="--===============HI-20131203==--"
MIME-Version: 1.0

----===============HI-20131203==--
Content-Type: test/some-type; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit

This is some userdata
----===============HI-20131203==----
`

	if s := c.ToMIMEText(); s != expected {
		t.Errorf("unexpected text.\nGot:\n%s\nExpected:\n%s", s, expected)
	}
}
