package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/lo"
	"github.com/openvibely/openvibely-paperworks/internal/testdeps"
)

func TestMain(m *testing.M) {
	if err := testdeps.Office(lo.Find); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
