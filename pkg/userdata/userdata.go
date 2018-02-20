package userdata

import (
	"bytes"
	"fmt"
)

const mimeHeader = `From nobody Tue Dec  3 19:00:57 2013
Content-Type: multipart/mixed; boundary="--===============HI-20131203==--"
MIME-Version: 1.0

`

const mimeFooter = `----===============HI-20131203==----
`

const partTemplate = `----===============HI-20131203==--
Content-Type: %s; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit

%s
`

// Container holds userdata that is passed to the MV
type Container interface {
	// AddPart will add a part to the userdata container
	AddPart(contentType, contentValue string)
	// ToMIMEText will generate MIME text from the container
	ToMIMEText() string
}

type container struct {
	parts []containerPart
}

type containerPart struct {
	contentType, contentValue string
}

// New will return an initialized Container
func New() Container {
	return &container{
		parts: []containerPart{},
	}
}

func (c *container) AddPart(contentType, contentValue string) {
	if c.parts == nil {
		c.parts = []containerPart{}
	}
	c.parts = append(c.parts, containerPart{
		contentType:  contentType,
		contentValue: contentValue,
	})
}

// I'm not sure if this is 100% correct MIME, but it matches
// the old brkt-cli at least
func (c *container) ToMIMEText() string {
	b := new(bytes.Buffer)
	b.WriteString(mimeHeader)
	for _, p := range c.parts {
		b.WriteString(fmt.Sprintf(partTemplate, p.contentType, p.contentValue))
	}
	b.WriteString(mimeFooter)
	return b.String()
}
