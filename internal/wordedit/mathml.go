package wordedit

import (
	"errors"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/mathml"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func equation(d *Doc, spec map[string]any) error {
	src := str(spec, "mathml")
	if strings.TrimSpace(src) == "" {
		return errors.New("mathml is required")
	}
	x, err := opc.ParseXML([]byte(src))
	if err != nil {
		return errors.New("invalid MathML: " + err.Error())
	}
	om, err := mathml.ToOMML(x.Root())
	if err != nil {
		return err
	}
	p := W("p")
	p.AddChild(om)
	appendBody(d, p)
	d.Touch(d.Main)
	return nil
}
