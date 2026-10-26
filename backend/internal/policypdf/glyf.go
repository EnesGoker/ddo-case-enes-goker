package policypdf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

// GoPDF v0.38.3 copies only one level of composite components into its font subset.
// Noto draws Ö, Ü, Ğ and İ through a nested mark glyph, so those marks were missing on
// the page while text extraction stayed correct. flattenComposites makes every composite
// glyph refer to simple glyphs directly: only x/y offsets are added, so outlines, the
// embedded font file and its license stay unchanged.
var pdfFont = sync.OnceValues(func() ([]byte, error) { return flattenComposites(font) })

var errBadFont = errors.New("font: bozuk TrueType tablosu")

const (
	compWords   = 0x0001
	compXY      = 0x0002
	compScale   = 0x0008
	compMore    = 0x0020
	compXYScale = 0x0040
	comp2x2     = 0x0080
	compInstr   = 0x0100
	compMetrics = 0x0200
)

type component struct {
	flags, glyph uint16
	dx, dy       int32
	transform    []byte
}

func table(font []byte, tag string) (rec, off, size int, err error) {
	if len(font) < 12 {
		return 0, 0, 0, errBadFont
	}
	for i := 0; i < int(binary.BigEndian.Uint16(font[4:])); i++ {
		rec = 12 + 16*i
		if rec+16 > len(font) {
			break
		}
		if string(font[rec:rec+4]) == tag {
			off, size = int(binary.BigEndian.Uint32(font[rec+8:])), int(binary.BigEndian.Uint32(font[rec+12:]))
			if off+size > len(font) {
				return 0, 0, 0, errBadFont
			}
			return rec, off, size, nil
		}
	}
	return 0, 0, 0, fmt.Errorf("font: %s tablosu yok", tag)
}

// glyphData splits glyf with the short or long loca format declared in head.
func glyphData(font []byte) ([][]byte, error) {
	_, head, headSize, err := table(font, "head")
	if err != nil {
		return nil, err
	}
	_, maxp, maxpSize, err := table(font, "maxp")
	if err != nil {
		return nil, err
	}
	_, loca, locaSize, err := table(font, "loca")
	if err != nil {
		return nil, err
	}
	_, glyf, glyfSize, err := table(font, "glyf")
	if err != nil {
		return nil, err
	}
	if headSize < 54 || maxpSize < 6 {
		return nil, errBadFont
	}
	long := binary.BigEndian.Uint16(font[head+50:]) == 1
	n := int(binary.BigEndian.Uint16(font[maxp+4:]))
	width := 2
	if long {
		width = 4
	}
	if locaSize < (n+1)*width {
		return nil, errBadFont
	}
	at := func(i int) int {
		if long {
			return int(binary.BigEndian.Uint32(font[loca+4*i:]))
		}
		return 2 * int(binary.BigEndian.Uint16(font[loca+2*i:]))
	}
	out := make([][]byte, n)
	for i := range out {
		a, b := at(i), at(i+1)
		if a > b || b > glyfSize {
			return nil, errBadFont
		}
		out[i] = font[glyf+a : glyf+b]
	}
	return out, nil
}

// parseComposite returns nil components for simple or empty glyphs.
func parseComposite(g []byte) (comps []component, instr []byte, err error) {
	if len(g) < 10 || int16(binary.BigEndian.Uint16(g)) >= 0 {
		return nil, nil, nil
	}
	p, hasInstr := 10, false
	for more := true; more; {
		if p+4 > len(g) {
			return nil, nil, errBadFont
		}
		c := component{flags: binary.BigEndian.Uint16(g[p:]), glyph: binary.BigEndian.Uint16(g[p+2:])}
		p += 4
		switch {
		case c.flags&compWords != 0 && p+4 <= len(g):
			c.dx, c.dy = int32(binary.BigEndian.Uint16(g[p:])), int32(binary.BigEndian.Uint16(g[p+2:]))
			if c.flags&compXY != 0 {
				c.dx, c.dy = int32(int16(c.dx)), int32(int16(c.dy))
			}
			p += 4
		case c.flags&compWords == 0 && p+2 <= len(g):
			c.dx, c.dy = int32(g[p]), int32(g[p+1])
			if c.flags&compXY != 0 {
				c.dx, c.dy = int32(int8(g[p])), int32(int8(g[p+1]))
			}
			p += 2
		default:
			return nil, nil, errBadFont
		}
		size := 0
		switch {
		case c.flags&compScale != 0:
			size = 2
		case c.flags&compXYScale != 0:
			size = 4
		case c.flags&comp2x2 != 0:
			size = 8
		}
		if p+size > len(g) {
			return nil, nil, errBadFont
		}
		c.transform, p = g[p:p+size], p+size
		hasInstr = hasInstr || c.flags&compInstr != 0
		more = c.flags&compMore != 0
		comps = append(comps, c)
	}
	if hasInstr {
		if p+2 > len(g) || p+2+int(binary.BigEndian.Uint16(g[p:])) > len(g) {
			return nil, nil, errBadFont
		}
		instr = g[p : p+2+int(binary.BigEndian.Uint16(g[p:]))]
	}
	return comps, instr, nil
}

// flatten replaces a reference to a composite glyph by that glyph's own components.
func flatten(glyphs [][]byte, id uint16, depth int) ([]component, error) {
	comps, _, err := parseComposite(glyphs[id])
	if err != nil {
		return nil, err
	}
	var out []component
	for _, c := range comps {
		if int(c.glyph) >= len(glyphs) || depth > 8 {
			return nil, errBadFont
		}
		if sub, _, err := parseComposite(glyphs[c.glyph]); err != nil {
			return nil, err
		} else if sub == nil {
			out = append(out, c)
			continue
		}
		if c.flags&compXY == 0 || len(c.transform) > 0 {
			return nil, fmt.Errorf("font: glif %d için desteklenmeyen bileşen dönüşümü", id)
		}
		inner, err := flatten(glyphs, c.glyph, depth+1)
		if err != nil {
			return nil, err
		}
		for _, s := range inner {
			if s.flags&compXY == 0 || len(s.transform) > 0 {
				return nil, fmt.Errorf("font: glif %d için desteklenmeyen bileşen dönüşümü", id)
			}
			s.dx, s.dy = s.dx+c.dx, s.dy+c.dy
			s.flags = s.flags&^compMetrics | s.flags&c.flags&compMetrics
			out = append(out, s)
		}
	}
	return out, nil
}

func encodeComposite(header []byte, comps []component, instr []byte) ([]byte, error) {
	out := append([]byte(nil), header[:10]...)
	for i, c := range comps {
		if c.dx < -32768 || c.dx > 65535 || c.dy < -32768 || c.dy > 65535 {
			return nil, errBadFont
		}
		f := c.flags&^(compMore|compInstr) | compWords
		if i < len(comps)-1 {
			f |= compMore
		} else if instr != nil {
			f |= compInstr
		}
		out = binary.BigEndian.AppendUint16(out, f)
		out = binary.BigEndian.AppendUint16(out, c.glyph)
		out = binary.BigEndian.AppendUint16(out, uint16(c.dx))
		out = binary.BigEndian.AppendUint16(out, uint16(c.dy))
		out = append(out, c.transform...)
	}
	out = append(out, instr...)
	for len(out)%4 != 0 {
		out = append(out, 0)
	}
	return out, nil
}

func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}

// flattenComposites appends rewritten glyf and long-format loca tables to a copy of
// the font and points the table directory at them.
func flattenComposites(src []byte) ([]byte, error) {
	glyphs, err := glyphData(src)
	if err != nil {
		return nil, err
	}
	var glyf []byte
	loca := make([]byte, 0, 4*(len(glyphs)+1))
	maxComponents := 0
	for id, g := range glyphs {
		loca = binary.BigEndian.AppendUint32(loca, uint32(len(glyf)))
		comps, instr, err := parseComposite(g)
		if err != nil {
			return nil, err
		}
		flat, err := flatten(glyphs, uint16(id), 0)
		if err != nil {
			return nil, err
		}
		maxComponents = max(maxComponents, len(flat))
		if len(flat) == len(comps) {
			nested := false
			for i := range comps {
				nested = nested || comps[i].glyph != flat[i].glyph
			}
			if !nested {
				glyf = append(glyf, g...)
				continue
			}
		}
		raw, err := encodeComposite(g, flat, instr)
		if err != nil {
			return nil, err
		}
		glyf = append(glyf, raw...)
	}
	loca = binary.BigEndian.AppendUint32(loca, uint32(len(glyf)))

	out := append([]byte(nil), src...)
	_, head, _, _ := table(out, "head")
	binary.BigEndian.PutUint16(out[head+50:], 1)
	if _, maxp, size, _ := table(out, "maxp"); size >= 32 && int(binary.BigEndian.Uint16(out[maxp+28:])) < maxComponents {
		binary.BigEndian.PutUint16(out[maxp+28:], uint16(maxComponents))
	}
	for _, t := range []struct {
		tag  string
		data []byte
	}{{"glyf", glyf}, {"loca", loca}} {
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
		rec, _, _, err := table(out, t.tag)
		if err != nil {
			return nil, err
		}
		binary.BigEndian.PutUint32(out[rec+4:], checksum(t.data))
		binary.BigEndian.PutUint32(out[rec+8:], uint32(len(out)))
		binary.BigEndian.PutUint32(out[rec+12:], uint32(len(t.data)))
		out = append(out, t.data...)
	}
	return out, nil
}
