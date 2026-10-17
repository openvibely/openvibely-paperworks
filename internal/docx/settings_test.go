package docx

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
	"testing"
)

func TestTemplateSettingsPreserved(t *testing.T) {
	var baseline []byte
	for _, mode := range []int{14, 15, 0} {
		raw, err := Build(Spec{CompatibilityMode: mode}, "")
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Read(raw)
		if err != nil {
			t.Fatal(err)
		}
		settings := p.Get("word/settings.xml")
		if mode == 14 {
			// Pinned unchanged installed Word example and blank reference DOCX library template.
			if got := fmt.Sprintf("%x", sha256.Sum256(settings)); got != "51a0d348fe85965c66e4748a03c3c0d055d78455514f03cb121c334af7d73689" {
				t.Fatalf("reference settings hash: %s", got)
			}
			baseline = settings
		} else {
			normalized := bytes.Replace(settings, []byte(`w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"`), []byte(`w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="14"`), 1)
			if !bytes.Equal(normalized, baseline) {
				t.Fatal("unrelated template settings changed")
			}
		}
	}
}
