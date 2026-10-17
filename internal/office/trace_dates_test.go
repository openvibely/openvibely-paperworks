package office

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestTraceSerialDateCalendarAndPrecision(t *testing.T) {
	for _, c := range []struct {
		n     float64
		epoch bool
		want  string
	}{
		{0, false, "00:00:00"}, {.5, false, "12:00:00"}, {1, false, "1900-01-01T00:00:00"},
		{59, false, "1900-02-28T00:00:00"}, {60, false, "1900-02-28T00:00:00"}, {61, false, "1900-03-01T00:00:00"},
		{-1, false, "1899-12-29T00:00:00"}, {1, true, "1904-01-02T00:00:00"},
		{45292.5, false, "2024-01-01T12:00:00"}, {43830.5, true, "2024-01-01T12:00:00"},
		{45292 + 1234./86400000, false, "2024-01-01T00:00:01.234000"},
		{45292 + 86399999.9/86400000, false, "2024-01-02T00:00:00"},
	} {
		got, err := traceSerialDate(c.n, c.epoch)
		if err != nil || got != c.want {
			t.Errorf("%v epoch=%v: %q %v; want %q", c.n, c.epoch, got, err, c.want)
		}
	}
	for _, n := range []float64{math.NaN(), math.Inf(1), 1e30, -1e30} {
		if _, err := traceSerialDate(n, false); err == nil {
			t.Errorf("accepted %v", n)
		}
	}
}

func TestTraceStyledDateCachesPreserveSource(t *testing.T) {
	p, file := traceFixture(t, func(f *excelize.File) {
		f.SetCellValue("Inputs", "A1", 45292.5)
		style, err := f.NewStyle(&excelize.Style{NumFmt: 22})
		if err != nil {
			t.Fatal(err)
		}
		f.SetCellStyle("Inputs", "A1", "A1", style)
		f.SetCellFormula("Output", "C1", "Inputs!A1+1")
		f.SetCellStyle("Output", "C1", "C1", style)
		format := `0 "days"`
		literal, err := f.NewStyle(&excelize.Style{CustomNumFmt: &format})
		if err != nil {
			t.Fatal(err)
		}
		f.SetCellValue("Inputs", "B1", 42)
		f.SetCellStyle("Inputs", "B1", "B1", literal)
	})
	// Install a deliberately stale saved formula result. Tracing must report it,
	// not recompute it from Inputs!A1.
	wb, err := OpenWorkbook(p)
	if err != nil {
		t.Fatal(err)
	}
	c := wb.Sheets["Output"].Cells["C1"]
	c.RemoveAttr("t")
	v := Child(c, NSS, "v")
	if v == nil {
		v = c.CreateElement("v")
	}
	v.SetText("61.5")
	if err = p.SetXML(wb.Sheets["Output"].Part, wb.Sheets["Output"].Doc); err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, part := range p.Parts {
		parts[part.Name] = bytes.Clone(part.Data)
	}
	before, _ := os.ReadFile(file)
	r := traceRun(t, p, "Output", "C1", 200, 20)
	if !r.Complete || r.Nodes["'Output'!C1"].CachedValue != "1900-03-01T12:00:00" || r.Nodes["'Inputs'!A1"].CachedValue != "2024-01-01T12:00:00" {
		t.Fatalf("%+v", r)
	}
	if got := traceRun(t, p, "Inputs", "B1", 200, 20).Nodes["'Inputs'!B1"].CachedValue; got != float64(42) {
		t.Fatal(got)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	for _, part := range p.Parts {
		if !bytes.Equal(parts[part.Name], part.Data) {
			t.Fatal("part mutated", part.Name)
		}
	}
}

func TestTraceCustomDateFormatsAndElapsedValues(t *testing.T) {
	for _, c := range []struct {
		format   string
		want     any
		complete bool
	}{
		{`yyyy-mm-dd`, "2024-01-01T12:00:00", true},
		{`[Red]yyyy-mm-dd`, "2024-01-01T12:00:00", true},
		{`[$-409]mmm d, yyyy`, "2024-01-01T12:00:00", true},
		{`[$-411]yyyy"年"m"月"d"日"`, "2024-01-01T12:00:00", true},
		{`[$USD-409]#,##0.00`, float64(45292.5), true},
		{`[Blue][>=0]yyyy-mm-dd;0`, "2024-01-01T12:00:00", true},
		{`"days" 0.00`, float64(45292.5), true},
		{`0;yyyy-mm-dd`, float64(45292.5), true},
		{`0 \d`, float64(45292.5), true},
		{`0 _d`, float64(45292.5), true},
		{`0 "[h]"`, float64(45292.5), true},
		{`[h]:mm:ss`, "45292 days, 12:00:00", true},
	} {
		t.Run(c.format, func(t *testing.T) {
			p, _ := traceFixture(t, func(f *excelize.File) {
				style, err := f.NewStyle(&excelize.Style{CustomNumFmt: &c.format})
				if err != nil {
					t.Fatal(err)
				}
				f.SetCellValue("Inputs", "A1", 45292.5)
				f.SetCellStyle("Inputs", "A1", "A1", style)
			})
			r := traceRun(t, p, "Inputs", "A1", 200, 20)
			if _, err := json.Marshal(r); err != nil {
				t.Fatal(err)
			}
			if r.Nodes["'Inputs'!A1"].CachedValue != c.want || r.Complete != c.complete {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestTraceDurationRepresentation(t *testing.T) {
	for _, c := range []struct {
		days float64
		want string
	}{
		{0, "0:00:00"}, {.5, "12:00:00"}, {1.5, "1 day, 12:00:00"},
		{-0.5, "-1 day, 12:00:00"}, {-1.5, "-2 days, 12:00:00"},
		{49. / 24, "2 days, 1:00:00"}, {1.234 / 86400, "0:00:01.234000"},
		{-0.001 / 86400, "-1 day, 23:59:59.999000"},
		{86399.9999 / 86400, "1 day, 0:00:00"}, {999999999, "999999999 days, 0:00:00"},
	} {
		got, err := traceDuration(c.days)
		if err != nil || got != c.want {
			t.Errorf("%v: %q %v; want %q", c.days, got, err, c.want)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), 1e9, -1e9} {
		if _, err := traceDuration(v); err == nil {
			t.Errorf("accepted %v", v)
		}
	}
}

func TestTraceISODateCellsAndPreservation(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want any
	}{
		{"2024-02-29", "2024-02-29"}, {"2024-02-29T12:34:56Z", "2024-02-29T12:34:56"},
		{"2024-01-01T12:34:56.123", "2024-01-01T12:34:56.123000"},
		{"12:34", "12:34:00"}, {"T12:34:56.1Z", "12:34:56.100000"},
		{"PT49H2M3.125S", "2 days, 1:02:03.125000"},
		{"PT23999999976H0.001S", "999999999 days, 0:00:00.001000"},
		{"PT23999999999H59M59.999S", "999999999 days, 23:59:59.999000"},
		{"PT24000000H0.001S", "1000000 days, 0:00:00.001000"},
		{"PT1439999998560M0.125S", "999999999 days, 0:00:00.125000"}, {"PT0S", "0:00:00"}, {"", nil},
	} {
		t.Run(c.raw, func(t *testing.T) {
			p, file := traceFixture(t, nil)
			wb, err := OpenWorkbook(p)
			if err != nil {
				t.Fatal(err)
			}
			cell := wb.Sheets["Inputs"].Cells["A1"]
			cell.CreateAttr("t", "d")
			Child(cell, NSS, "v").SetText(c.raw)
			if err = p.SetXML(wb.Sheets["Inputs"].Part, wb.Sheets["Inputs"].Doc); err != nil {
				t.Fatal(err)
			}
			parts := map[string][]byte{}
			for _, part := range p.Parts {
				parts[part.Name] = bytes.Clone(part.Data)
			}
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			// A dependent formula must still trace through the ISO-valued precedent.
			r := traceRun(t, p, "Output", "A1", 200, 20)
			if !r.Complete || r.Nodes["'Inputs'!A1"].CachedValue != c.want {
				t.Fatalf("%+v", r)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(original, after) {
				t.Fatal("input mutated")
			}
			for _, part := range p.Parts {
				if !bytes.Equal(parts[part.Name], part.Data) {
					t.Fatal("part mutated", part.Name)
				}
			}
		})
	}
	for _, s := range []string{"2023-02-29", "0000-01-01", "24:00:00", "12:60:00", "2024-01-01garbage", "PT", "P1D", "PT999999999999999999999H", "PT24000000000H", "PT23999999999H60M", "PT86400000000000S", "2024-01-01T12:00:00+02:00", "12:00:00.1234"} {
		if _, err := traceISOValue(s); err == nil {
			t.Errorf("accepted malformed/unsupported ISO value %q", s)
		}
	}
}

// Expected classifications follow the reference reader's published
// is_date_format/is_timedelta_format predicates, including their asymmetric
// treatment of uppercase bracket fields. These are cache types, not display.
func TestTraceReferenceDateFormatEdges(t *testing.T) {
	cases := []struct {
		format        string
		date, elapsed bool
		want          any
	}{
		{`[hhh]`, false, false, float64(1.5)},
		{`[hhh]:mm`, true, false, "1900-01-01T12:00:00"},
		{`[hh]`, true, true, "1 day, 12:00:00"},
		{`[mm]`, true, true, "1 day, 12:00:00"},
		{`[ss].000`, true, true, "1 day, 12:00:00"},
		{`[H]`, false, false, float64(1.5)},
		{`[HH]:mm`, true, true, "1 day, 12:00:00"},
		{`0 "[h]"`, false, false, float64(1.5)},
		{`yyyy "[h]"`, true, true, "1 day, 12:00:00"},
		{`\d`, false, false, float64(1.5)},
		{`\\d`, false, false, float64(1.5)},
		{`_d`, false, false, float64(1.5)},
		{`__d`, false, false, float64(1.5)},
		{`\dd`, true, false, "1900-01-01T12:00:00"},
		{`[$-409][Red]0.00`, false, false, float64(1.5)},
		{`"month" 0.00;[h]`, false, false, float64(1.5)},
	}
	for _, c := range cases {
		t.Run(c.format, func(t *testing.T) {
			style := traceCustomDateStyle(c.format)
			if style.date != c.date || style.elapsed != c.elapsed {
				t.Fatalf("classification %+v", style)
			}
			p, _ := traceFixture(t, func(f *excelize.File) {
				id, err := f.NewStyle(&excelize.Style{CustomNumFmt: &c.format})
				if err != nil {
					t.Fatal(err)
				}
				if err = f.SetCellValue("Inputs", "A1", 1.5); err != nil {
					t.Fatal(err)
				}
				if err = f.SetCellStyle("Inputs", "A1", "A1", id); err != nil {
					t.Fatal(err)
				}
			})
			before := map[string][]byte{}
			for _, part := range p.Parts {
				before[part.Name] = bytes.Clone(part.Data)
			}
			r := traceRun(t, p, "Inputs", "A1", 200, 20)
			if !r.Complete || r.Nodes["'Inputs'!A1"].CachedValue != c.want {
				t.Fatalf("%+v", r)
			}
			for _, part := range p.Parts {
				if !bytes.Equal(before[part.Name], part.Data) {
					t.Fatal("source part changed", part.Name)
				}
			}
		})
	}
}

// The reference reader adds days to the selected epoch within Reference's
// year 1..9999 calendar, then applies the rounded fractional day. The lower
// bound is therefore -695055 in the 1904 system, not the 1900 bound -693593.
func TestTraceSerialDateEpochBoundaries(t *testing.T) {
	for _, epoch := range []struct {
		name        string
		mac         bool
		first, last float64
	}{
		{"1900", false, -693593, 2958465}, {"1904", true, -695055, 2957003},
	} {
		for _, c := range []struct {
			name  string
			value float64
			want  string
		}{
			{"first", epoch.first, "0001-01-01T00:00:00"},
			{"first noon", epoch.first + .5, "0001-01-01T12:00:00"},
			{"last", epoch.last, "9999-12-31T00:00:00"},
			{"last noon", epoch.last + .5, "9999-12-31T12:00:00"},
			{"last millisecond", epoch.last + 86399999./86400000, "9999-12-31T23:59:59.999000"},
		} {
			t.Run(epoch.name+"/"+c.name, func(t *testing.T) {
				p, file := traceFixture(t, func(f *excelize.File) {
					if err := f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &epoch.mac}); err != nil {
						t.Fatal(err)
					}
					if err := f.SetCellValue("Inputs", "D1", c.value); err != nil {
						t.Fatal(err)
					}
					style, err := f.NewStyle(&excelize.Style{NumFmt: 22})
					if err != nil {
						t.Fatal(err)
					}
					if err = f.SetCellStyle("Inputs", "D1", "D1", style); err != nil {
						t.Fatal(err)
					}
				})
				before, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				parts := map[string][]byte{}
				for _, part := range p.Parts {
					parts[part.Name] = bytes.Clone(part.Data)
				}
				r := traceRun(t, p, "Inputs", "D1", 200, 20)
				if !r.Complete || r.Nodes["'Inputs'!D1"].CachedValue != c.want {
					t.Fatalf("%+v; want %s", r, c.want)
				}
				if _, err = json.Marshal(r); err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("source changed")
				}
				for _, part := range p.Parts {
					if !bytes.Equal(parts[part.Name], part.Data) {
						t.Fatal("part changed", part.Name)
					}
				}
			})
		}
		for _, n := range []float64{epoch.first - 1, math.Nextafter(epoch.first, math.Inf(-1)), epoch.last + 1, math.Nextafter(epoch.last+1, math.Inf(-1))} {
			if got, err := traceSerialDate(n, epoch.mac); err == nil {
				t.Errorf("%s accepted out-of-calendar/rounding overflow %v as %s", epoch.name, n, got)
			}
		}
	}
}
