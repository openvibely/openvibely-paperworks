package cli

import (
	"context"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"github.com/openvibely/openvibely-paperworks/internal/xlsx"
)

func init() {
	register(&Command{
		Name:    "inspect",
		Summary: "Read-only structure report for PDF/DOCX/XLS/XLSX/PPTX (forms, formulas, comments, notes, geometry).",
		Usage:   "paperworks inspect INPUT",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := existing(in[0]); err != nil {
				return nil, err
			}
			if strings.EqualFold(ext(in[0]), ".pdf") {
				return pdf.Report(ctx, in[0])
			}
			if ext(in[0]) == ".xls" {
				return xlsx.ReadLegacy(in[0])
			}
			return office.PackageReport(in[0])
		},
	})
	register(&Command{
		Name:    "repair-notes",
		Summary: "Restore a missing PPTX notes-master reference into a new file, preserving other parts.",
		Usage:   "paperworks repair-notes INPUT.pptx --output NEW.pptx",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			out := a.Path("output")
			if err := newFile(out, in[0]); err != nil {
				return nil, err
			}
			return office.RepairNotes(in[0], out)
		},
	})
	register(&Command{
		Name:    "verify-preservation",
		Summary: "Fail unless only explicitly allowed package parts changed between two Office files.",
		Usage:   "paperworks verify-preservation BEFORE --against AFTER --requirements contract.json\n  contract.json: {\"allow_changed\":[\"word/document.xml\"]}",
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := a.Require("against"); err != nil {
				return nil, err
			}
			var spec struct {
				AllowChanged *[]string `json:"allow_changed"`
			}
			if err := a.JSON("requirements", &spec); err != nil {
				return nil, err
			}
			if spec.AllowChanged == nil {
				return nil, usagef("allow_changed must list exact package part paths")
			}
			return office.VerifyPreservation(in[0], a.Path("against"), *spec.AllowChanged)
		},
	})
}

func ext(p string) string {
	i := strings.LastIndexByte(p, '.')
	if i < 0 {
		return ""
	}
	return strings.ToLower(p[i:])
}
