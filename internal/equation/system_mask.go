/* -*- Mode: c; c-basic-offset: 4; indent-tabs-mode: t; tab-width: 8; -*- */
/*
 * Copyright © 2005 Keith Packard
 *
 * This library is free software; you can redistribute it and/or
 * modify it either under the terms of the GNU Lesser General Public
 * License version 2.1 as published by the Free Software Foundation
 * (the "LGPL") or, at your option, under the terms of the Mozilla
 * Public License Version 1.1 (the "MPL"). If you do not alter this
 * notice, a recipient may use your version of this file under either
 * the MPL or the LGPL.
 *
 * You should have received a copy of the LGPL along with this library
 * in the file COPYING-LGPL-2.1; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Suite 500, Boston, MA 02110-1335, USA
 * You should have received a copy of the MPL along with this library
 * in the file COPYING-MPL-1.1
 *
 * The contents of this file are subject to the Mozilla Public License
 * Version 1.1 (the "License"); you may not use this file except in
 * compliance with the License. You may obtain a copy of the License at
 * http://www.mozilla.org/MPL/
 *
 * This software is distributed on an "AS IS" basis, WITHOUT WARRANTY
 * OF ANY KIND, either express or implied. See the LGPL or the MPL for
 * the specific language governing rights and limitations.
 *
 * The Original Code is the cairo graphics library.
 *
 * The Initial Developer of the Original Code is Keith Packard
 *
 * Contributor(s):
 *      Keith Packard <keithp@keithp.com>
 *	Carl D. Worth <cworth@cworth.org>
 *      Graydon Hoare <graydon@redhat.com>
 *      Owen Taylor <otaylor@redhat.com>
 *      Behdad Esfahbod <behdad@behdad.org>
 *      Chris Wilson <chris@chris-wilson.co.uk>
 */

// Go adaptation of Cairo 1.18.4 cairo-scaled-font.c bitmap tracing.
package equation

import (
	"fmt"
	"image"
	"math"

	"github.com/luo-studio/go-tex/tex/path"
)

// systemMaskOutline follows Cairo's bitmap-glyph fallback: an A1 mask becomes
// one unit rectangle per set pixel. The caller supplies the independently
// selected/rasterized mask and device offset; this does not select or scale sbix.
// Coordinates remain SVG-local pixels until the caller applies its transform.
func systemMaskOutline(mask *image.Alpha, x, y float64) ([]path.Command, error) {
	return systemMaskPath(mask, x, y, false)
}

// systemMaskRuns combines adjacent set pixels into a row rectangle. It preserves
// the filled union, not the captured command sequence. Masks remain caller inputs;
// this does not infer a font, strike, or backend mask from a glyph identifier.
func systemMaskRuns(mask *image.Alpha, x, y float64) ([]path.Command, error) {
	return systemMaskPath(mask, x, y, true)
}
func systemMaskPath(mask *image.Alpha, x, y float64, runs bool) ([]path.Command, error) {
	if mask == nil || mask.Bounds().Dx() < 0 || mask.Bounds().Dy() < 0 || mask.Bounds().Dx() > 4096 || mask.Bounds().Dy() > 4096 || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || math.Abs(x) > 1e6 || math.Abs(y) > 1e6 {
		return nil, fmt.Errorf("invalid glyph mask")
	}
	x = math.RoundToEven(x*256) / 256
	y = math.RoundToEven(y*256) / 256
	var out []path.Command
	for row := mask.Bounds().Min.Y; row < mask.Bounds().Max.Y; row++ {
		for col := mask.Bounds().Min.X; col < mask.Bounds().Max.X; col++ {
			if mask.AlphaAt(col, row).A < 128 {
				continue
			}
			if len(out) >= 5000000 {
				return nil, fmt.Errorf("glyph mask path exceeds budget")
			}
			start := col
			if runs {
				for col+1 < mask.Bounds().Max.X && mask.AlphaAt(col+1, row).A >= 128 {
					col++
				}
			}
			a, b := x+float64(start-mask.Bounds().Min.X), y+float64(row-mask.Bounds().Min.Y)
			end := x + float64(col+1-mask.Bounds().Min.X)
			out = append(out, path.MoveTo(a, b), path.LineTo(end, b), path.LineTo(end, b+1), path.LineTo(a, b+1), path.Close())
		}
	}
	return out, nil
}
