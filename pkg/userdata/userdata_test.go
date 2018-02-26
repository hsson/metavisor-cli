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
