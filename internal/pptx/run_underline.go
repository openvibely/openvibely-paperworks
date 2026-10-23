package pptx

import "fmt"

func validateRunUnderline(r Run) error {
	switch r.UnderlineStyle {
	case "", "none", "words", "sng", "dbl", "heavy", "dotted", "dottedHeavy", "dash", "dashHeavy", "dashLong", "dashLongHeavy", "dotDash", "dotDashHeavy", "dotDotDash", "dotDotDashHeavy", "wavy", "wavyHeavy", "wavyDbl":
	default:
		return fmt.Errorf("invalid underline_style %q", r.UnderlineStyle)
	}
	if r.Underline && r.UnderlineStyle != "" && r.UnderlineStyle != "sng" {
		return fmt.Errorf("use underline_style instead of underline")
	}
	return nil
}
