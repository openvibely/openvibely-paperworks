package testsupport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func TestReferencePresentationSaveOrder(t *testing.T) {
	var fixture struct {
		FixtureSHA string   `json:"fixture_sha256"`
		Order      []string `json:"order"`
	}
	data, err := os.ReadFile("testdata/runtime470-order.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	bundle, err := os.ReadFile("../../testdata/reference-mirror/runtime-workflow-fixtures.zip")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(bundle)) != fixture.FixtureSHA {
		t.Fatal("reference input fixture changed")
	}
	source := OriginalPackageFixture(t, "runtime-workflow-fixtures.zip", "chart-formula", ".pptx")
	p, err := opc.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[string][]byte)
	for _, name := range p.Names() {
		before[name] = append([]byte(nil), p.Get(name)...)
	}
	// This actual presentation includes cycles, shared targets, and an external
	// link. The expected traversal order comes from the unchanged Reference save.
	if reflect.DeepEqual(p.Names(), fixture.Order) {
		t.Fatal("fixture no longer exercises changed order")
	}
	if err = OrderPresentationFixture(p); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Names(), fixture.Order) {
		t.Fatalf("order %v; want %v", p.Names(), fixture.Order)
	}
	for name, payload := range before {
		if !bytes.Equal(payload, p.Get(name)) {
			t.Fatalf("payload changed: %s", name)
		}
	}
	raw, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	if !reflect.DeepEqual(names, fixture.Order) {
		t.Fatal("writer changed requested order")
	}
	if err = OrderPresentationFixture(p); err != nil || !reflect.DeepEqual(p.Names(), fixture.Order) {
		t.Fatal("second ordering changed result", err)
	}
	// Unsupported orphan handling is explicit and leaves the caller's order and
	// bytes unchanged. Do not silently remove parts to imitate a Reference save.
	p.Set("orphan.bin", []byte("retained"))
	originalOrder := p.Names()
	if err = OrderPresentationFixture(p); err == nil {
		t.Fatal("unreachable part silently accepted")
	}
	if !reflect.DeepEqual(p.Names(), originalOrder) || string(p.Get("orphan.bin")) != "retained" {
		t.Fatal("failed ordering changed package")
	}
}
