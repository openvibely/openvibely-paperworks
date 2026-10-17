package office

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var traceElapsedFormat = regexp.MustCompile(`(?i)\[(hh?|mm?|ss?)\]`)
var traceQuotedFormat = regexp.MustCompile(`"[^"\n]*?"`)
var traceBracketFormat = regexp.MustCompile(`\[[^\]]*\]`)

// traceCustomDateStyle follows the reference reader's independent date and
// timedelta predicates. Elapsed-looking text alone does not make a date style.
func traceCustomDateStyle(code string) traceDateStyle {
	code = strings.SplitN(code, ";", 2)[0]
	elapsed := traceElapsedFormat.MatchString(code)
	stripped := traceQuotedFormat.ReplaceAllString(code, "")
	stripped = traceBracketFormat.ReplaceAllStringFunc(stripped, func(s string) string {
		switch s {
		case "[h]", "[hh]", "[m]", "[mm]", "[s]", "[ss]":
			return s
		}
		return ""
	})
	for i := 0; i < len(stripped); i++ {
		if i > 0 && (stripped[i-1] == '_' || stripped[i-1] == '\\') {
			continue
		}
		if strings.ContainsRune("dmhysDMHYS", rune(stripped[i])) {
			return traceDateStyle{date: true, elapsed: elapsed}
		}
	}
	return traceDateStyle{}
}

type traceDateStyle struct{ date, elapsed bool }

func (wb *Workbook) traceDateStyles() ([]traceDateStyle, error) {
	if !wb.Pkg.Has("xl/styles.xml") {
		return nil, nil
	}
	d, err := wb.Pkg.XML("xl/styles.xml")
	if err != nil {
		return nil, err
	}
	custom := map[int]string{}
	for _, f := range Children(Child(d.Root(), NSS, "numFmts"), NSS, "numFmt") {
		id, err := strconv.Atoi(f.SelectAttrValue("numFmtId", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid number format ID")
		}
		custom[id] = f.SelectAttrValue("formatCode", "")
	}
	var out []traceDateStyle
	for _, xf := range Children(Child(d.Root(), NSS, "cellXfs"), NSS, "xf") {
		id, err := strconv.Atoi(xf.SelectAttrValue("numFmtId", "0"))
		if err != nil {
			return nil, err
		}
		style := traceDateStyle{date: builtinDateFormats[id], elapsed: id == 46}
		if code, ok := custom[id]; ok {
			style = traceCustomDateStyle(code)
		}
		out = append(out, style)
	}
	return out, nil
}

// traceSerialDate matches the reference reader's millisecond rounding and
// Excel's fictitious 1900 leap day. This is cache presentation, not calculation.
func traceSerialDate(n float64, date1904 bool) (string, error) {
	// Validate before float-to-int conversion. The calendars have different
	// serial bounds even though both support years 1 through 9999.
	minDay, maxDay := -693593., 2958465.
	if date1904 {
		minDay, maxDay = -695055, 2957003
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < minDay || n >= maxDay+1 {
		return "", fmt.Errorf("date cache outside supported calendar")
	}
	day := math.Floor(n)
	ms := int64(math.RoundToEven((n - day) * 86400000))
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	} else if n > 0 && n < 60 {
		day++
	}
	dt := base.AddDate(0, 0, int(day)).Add(time.Duration(ms) * time.Millisecond)
	if dt.Year() < 1 || dt.Year() > 9999 {
		return "", fmt.Errorf("date cache outside supported calendar")
	}
	format := "2006-01-02T15:04:05"
	if n >= 0 && n < 1 && ms < 86400000 {
		format = "15:04:05"
	}
	text := dt.Format(format)
	if dt.Nanosecond() != 0 {
		text += fmt.Sprintf(".%06d", dt.Nanosecond()/1000)
	}
	return text, nil
}

// traceDuration presents elapsed time independently of the workbook epoch.
// Unlike time.Duration this supports Excel's large day counts without overflow.
func traceDuration(days float64) (string, error) {
	if math.IsNaN(days) || math.IsInf(days, 0) || days < -999999999 || days >= 1000000000 {
		return "", fmt.Errorf("duration cache outside supported range")
	}
	whole := int64(math.Floor(days))
	// The reference reader first constructs a microsecond timedelta, then rounds
	// its fractional second to milliseconds using ties-to-even.
	micros := math.RoundToEven((days - float64(whole)) * 86400000000)
	ms := int64(math.RoundToEven(micros / 1000))
	if ms == 86400000 {
		whole++
		ms = 0
	}
	if whole > 999999999 {
		return "", fmt.Errorf("duration cache outside supported range")
	}
	return traceDurationText(whole, ms), nil
}

func traceDurationText(whole, ms int64) string {
	text := fmt.Sprintf("%d:%02d:%02d", ms/3600000, (ms/60000)%60, (ms/1000)%60)
	if ms%1000 != 0 {
		text += fmt.Sprintf(".%06d", (ms%1000)*1000)
	}
	if whole != 0 {
		unit := "days"
		if whole == 1 || whole == -1 {
			unit = "day"
		}
		text = fmt.Sprintf("%d %s, %s", whole, unit, text)
	}
	return text
}

var traceISODate = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})?(?:T)?(\d{2}:\d{2}(?::\d{2}(?:\.\d{1,3})?)?)?Z?$`)
var traceISODuration = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d{1,3})?)S)?$`)

func traceISOValue(raw string) (any, error) {
	if len(raw) > 128 {
		return nil, fmt.Errorf("ISO date cache exceeds 128-byte limit")
	}
	if raw == "" {
		return nil, nil
	}
	if m := traceISODuration.FindStringSubmatch(raw); m != nil {
		// Keep ISO components exact. Converting them to a float day count
		// discards milliseconds and can round a valid upper bound out of range.
		const maxMS uint64 = 1000000000*86400000 - 1
		var total uint64
		present := false
		for i, mult := range []uint64{3600000, 60000, 1000} {
			raw := m[i+1]
			if raw == "" {
				continue
			}
			present = true
			fraction := uint64(0)
			if i == 2 {
				parts := strings.SplitN(raw, ".", 2)
				raw = parts[0]
				if len(parts) == 2 {
					value, err := strconv.ParseUint(parts[1]+strings.Repeat("0", 3-len(parts[1])), 10, 64)
					if err != nil {
						return nil, err
					}
					fraction = value
				}
			}
			value, err := strconv.ParseUint(raw, 10, 64)
			if err != nil || value > (maxMS-total)/mult {
				return nil, fmt.Errorf("duration cache outside supported range")
			}
			total += value * mult
			if fraction > maxMS-total {
				return nil, fmt.Errorf("duration cache outside supported range")
			}
			total += fraction
		}
		if !present {
			return nil, fmt.Errorf("empty ISO duration")
		}
		return traceDurationText(int64(total/86400000), int64(total%86400000)), nil
	}
	m := traceISODate.FindStringSubmatch(raw)
	if m == nil || m[1] == "" && m[2] == "" {
		return nil, fmt.Errorf("invalid ISO date cache %q", raw)
	}
	date, clock := m[1], m[2]
	if date != "" {
		dt, err := time.Parse("2006-01-02", date)
		if err != nil || dt.Year() < 1 {
			return nil, fmt.Errorf("invalid ISO calendar date %q", raw)
		}
	}
	if clock != "" {
		format := "15:04:05"
		if len(clock) == 5 {
			format = "15:04"
		}
		dt, err := time.Parse(format, clock)
		if err != nil {
			return nil, fmt.Errorf("invalid ISO clock %q", raw)
		}
		clock = dt.Format("15:04:05")
		if dt.Nanosecond() != 0 {
			clock += fmt.Sprintf(".%06d", dt.Nanosecond()/1000)
		}
	}
	if date == "" {
		return clock, nil
	}
	if clock == "" {
		return date, nil
	}
	return date + "T" + clock, nil
}
