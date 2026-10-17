package convert

import (
	"reflect"
	"testing"
)

func TestReferenceFenceDiscoveryContract(t *testing.T) {
	for _, c := range []struct {
		text string
		want []string
	}{
		{"```sh\na\n```", []string{"a"}},
		{"prefix```sh\na\n```suffix\n```sh\nb\n```", []string{"a", "b"}},
		{"```sh\nunclosed", []string{}},
		{"```sh\na\n```sh\nb\n```", []string{"a"}},
		{"```sh\n\n```", []string{""}},
		{"```sh\r\na\r\n```", []string{}},
	} {
		if got := shellBlocks(t, c.text); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%q: %#v != %#v", c.text, got, c.want)
		}
	}
}
