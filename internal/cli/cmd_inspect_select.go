package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/xuri/excelize/v2"
	"sort"
	"strconv"
	"strings"
)

func init() {
	register(&Command{Name: "inspect-select", Summary: "Select a JSON pointer from an inspection report, or search bounded matching values.", Usage: "paperworks inspect-select INPUT [--path /slides/0] [--sheet NAME --range A1:D20] [--search TEXT] [--max-items 100] [--max-chars 20000]", Run: func(ctx context.Context, a Args) (any, error) {
		report, err := commands["inspect"].Run(ctx, a)
		if err != nil {
			return nil, err
		}
		if a.Str("sheet") != "" || a.Str("range") != "" {
			if a.Str("sheet") == "" || a.Str("range") == "" {
				return nil, usagef("sheet and range must be supplied together")
			}
			if len(a.Pos) != 1 || ext(a.Pos[0]) != ".xlsx" {
				return nil, usagef("sheet/range selection requires XLSX")
			}
			f, err := excelize.OpenFile(a.Pos[0])
			if err != nil {
				return nil, err
			}
			defer f.Close()
			start, end, ok := strings.Cut(a.Str("range"), ":")
			if !ok {
				end = start
			}
			x, y, err := excelize.CellNameToCoordinates(start)
			if err != nil {
				return nil, err
			}
			xx, yy, err := excelize.CellNameToCoordinates(end)
			if err != nil {
				return nil, err
			}
			if xx < x || yy < y || int64(xx-x+1)*int64(yy-y+1) > 10000 {
				return nil, usagef("range must contain 1..10000 cells")
			}
			cells := []map[string]any{}
			for row := y; row <= yy; row++ {
				for col := x; col <= xx; col++ {
					cell, _ := excelize.CoordinatesToCellName(col, row)
					v, err := f.GetCellValue(a.Str("sheet"), cell)
					if err != nil {
						return nil, err
					}
					formula, err := f.GetCellFormula(a.Str("sheet"), cell)
					if err != nil {
						return nil, err
					}
					cells = append(cells, map[string]any{"cell": cell, "value": v, "formula": formula})
				}
			}
			report = map[string]any{"sheet": a.Str("sheet"), "cells": cells, "cache_warning": "Values may be stale until recalculated."}
		}
		data, err := json.Marshal(report)
		if err != nil {
			return nil, err
		}
		var value any
		if err = json.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		pointer := a.Str("path")
		if pointer != "" {
			if !strings.HasPrefix(pointer, "/") {
				return nil, usagef("path must be a JSON pointer beginning with /")
			}
			for _, segment := range strings.Split(pointer[1:], "/") {
				segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
				switch v := value.(type) {
				case map[string]any:
					var ok bool
					value, ok = v[segment]
					if !ok {
						return nil, usagef("path not found")
					}
				case []any:
					i, err := strconv.Atoi(segment)
					if err != nil || i < 0 || i >= len(v) {
						return nil, usagef("array index out of range")
					}
					value = v[i]
				default:
					return nil, usagef("path crosses a scalar")
				}
			}
		}
		limit, err := a.Int("max-items", 100)
		if err != nil {
			return nil, err
		}
		chars, err := a.Int("max-chars", 20000)
		if err != nil {
			return nil, err
		}
		if limit < 1 || limit > 10000 || chars < 1 || chars > 1000000 {
			return nil, usagef("max-items must be 1..10000 and max-chars 1..1000000")
		}
		needle := strings.ToLower(a.Str("search"))
		if needle == "" {
			data, _ := json.Marshal(value)
			if len(data) > chars {
				return nil, usagef("selection exceeds max-chars; narrow --path or use --search")
			}
			return value, nil
		}
		results := []map[string]any{}
		truncated := false
		used := 0
		var walk func(any, string)
		walk = func(v any, p string) {
			if truncated {
				return
			}
			switch x := v.(type) {
			case map[string]any:
				keys := make([]string, 0, len(x))
				for k := range x {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					walk(x[k], p+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"))
				}
			case []any:
				for i, v := range x {
					walk(v, fmt.Sprintf("%s/%d", p, i))
				}
			default:
				data, _ := json.Marshal(x)
				if strings.Contains(strings.ToLower(string(data)), needle) {
					entry := map[string]any{"path": p, "value": x}
					raw, _ := json.Marshal(entry)
					if len(results) >= limit || used+len(raw) > chars {
						truncated = true
						return
					}
					used += len(raw)
					results = append(results, entry)
				}
			}
		}
		walk(value, pointer)
		return map[string]any{"matches": results, "truncated": truncated}, nil
	}})
}
