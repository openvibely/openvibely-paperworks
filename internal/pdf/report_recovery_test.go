package pdf

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestReportDirectEmptyAppearanceRecovery(t *testing.T) {
	data, err := os.ReadFile("../../testdata/reference-mirror/library-bad-appearance.pdf")
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(data)
	report, err := ReportBytes(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if report.Fields["account"] == nil || *report.Fields["account"] != "Robot" || len(report.Widgets) != 1 || report.Widgets[0].Name != "account" || report.Widgets[0].Value == nil || *report.Widgets[0].Value != "Robot" || report.Widgets[0].NormalAppearance {
		t.Fatalf("recovery: %+v", report)
	}
	if !reflect.DeepEqual(report.Issues, []string{"Widget missing usable normal appearance: account"}) {
		t.Fatal(report.Issues)
	}
	unsupported := bytes.Replace(data, []byte("/Length 0"), []byte("/Length 1"), 1)
	unreadable, err := ReportBytes(context.Background(), unsupported)
	if err != nil {
		t.Fatal(err)
	}
	if len(unreadable.Widgets) != 0 || !strings.Contains(strings.Join(unreadable.Issues, "\n"), "unreadable annotation") {
		t.Fatalf("unsupported recovery lost warning: %+v", unreadable)
	}
	if !bytes.Equal(data, before) {
		t.Fatal("report mutated input")
	}
	for _, text := range []string{
		"<< /AP << /N << /Length 1 >> stream\nx\nendstream >> >>",
		"<< /AP << /N << /Length 0 >> stream\nx\nendstream >> >>",
		"<< /AP << /N << /Length 0 >> stream\n\nendstreamX >> >>",
		"<< /AP << /N << /Length 0 /Filter /FlateDecode >> stream\n\nendstream >> >>",
		"<< /Other << /Length 0 >> stream\n\nendstream >>",
		"<< /AP << /N << /Length 0 /Length 0 >> stream\n\nendstream >> >>",
	} {
		p := emptyAppearanceParser{ctx: context.Background(), text: text}
		if _, err := p.read(0, ""); err == nil {
			t.Errorf("accepted %s", text)
		}
	}
	// Literal tokens must never trigger recovery, even when they resemble syntax.
	p := emptyAppearanceParser{ctx: context.Background(), text: "<< /T (stream endstream /AP /N) >>"}
	if _, err := p.read(0, ""); err != nil || p.recovered {
		t.Fatal(err, p.recovered)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p = emptyAppearanceParser{ctx: ctx, text: strings.Repeat("<< /A ", 40)}
	if _, err := p.read(0, ""); err == nil {
		t.Fatal("ignored cancellation")
	}
}
