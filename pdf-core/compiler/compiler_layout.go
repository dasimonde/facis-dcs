package compiler

import (
	"strings"
)

func clauseSegmentText(seg clauseSegment) string {
	switch seg.Type {
	case "prose", "ontology-link", "external-link":
		return seg.Text
	case "typed-value":
		if seg.Unit == "" {
			return seg.Value
		}
		return seg.Value + " (" + seg.Unit + ")"
	default:
		return ""
	}
}

func wrapTextPreservingBreaksPts(input string, maxWidth float64, fontSize float64) []string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	parts := strings.Split(normalized, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, wrapTextPts(part, maxWidth, fontSize)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// layoutDocument lays out doc into pages and also returns the flat section list
// (depth-first traversal of the section tree) and corresponding depths. Callers
// that need to look up a clause by positionedLine.SectionIdx must use
// flatSections rather than doc.Sections.
func layoutDocument(doc documentModel) (pages []pageLayout, flatSections []sectionData, flatDepths []int) { //nolint:unparam
	return layoutDocumentFull(doc)
}

// layoutDocumentPages is a convenience wrapper for callers that only need the
// page list. It is used in tests and in update.go.
func layoutDocumentPages(doc documentModel) []pageLayout {
	pages, _, _ := layoutDocumentFull(doc)
	return pages
}

func layoutDocumentFull(doc documentModel) (pages []pageLayout, flatSections []sectionData, flatDepths []int) {
	const (
		pageWidth    = 612.0
		pageHeight   = 792.0
		leftMargin   = 54.0
		rightMargin  = 54.0
		topMargin    = 72.0
		bottomMargin = 54.0
	)

	textWidth := pageWidth - leftMargin - rightMargin // 504pt
	glossIndent := 18.0
	depthIndent := 18.0 // per-level X indent for subsections

	type lineMetadata struct {
		lineIndex      int
		sectionIdx     int // index into flatSections
		localClauseIdx int
	}
	var lineToClause []lineMetadata

	lineSpecs := make([]layoutLine, 0, 32)
	lineSpecs = append(lineSpecs, layoutLine{Text: doc.Title, FontSize: 18, Kind: "title"})
	lineSpecs = append(lineSpecs, layoutLine{Text: "Payload CID: " + doc.PayloadCID, FontSize: 9, Kind: "meta"})

	// addSection recursively flattens sections depth-first, emitting layoutLines.
	var addSection func(section sectionData, depth int)
	addSection = func(section sectionData, depth int) {
		flatIdx := len(flatSections)
		flatSections = append(flatSections, section)
		flatDepths = append(flatDepths, depth)

		headingKind := "section-heading"
		if depth > 0 {
			headingKind = "subsection-heading"
		}
		fontSize := 14.0 - float64(depth)*2
		if fontSize < 10 {
			fontSize = 10
		}
		if section.Heading != "" {
			lineSpecs = append(lineSpecs, layoutLine{
				Text:     section.Heading,
				FontSize: fontSize,
				Kind:     headingKind,
				Depth:    depth,
			})
		}
		clauseTextWidth := textWidth - float64(depth)*depthIndent
		for clauseIdx, clause := range section.Clauses {
			var clauseText strings.Builder
			for _, seg := range clause.Segments {
				clauseText.WriteString(clauseSegmentText(seg))
			}
			for _, wrapped := range wrapTextPreservingBreaksPts(clauseText.String(), clauseTextWidth, 11.0) {
				lineToClause = append(lineToClause, lineMetadata{
					lineIndex:      len(lineSpecs),
					sectionIdx:     flatIdx,
					localClauseIdx: clauseIdx,
				})
				lineSpecs = append(lineSpecs, layoutLine{Text: wrapped, FontSize: 11, Kind: "body", Depth: depth})
			}
		}
		for _, sub := range section.Subsections {
			addSection(sub, depth+1)
		}
	}

	for _, section := range doc.Sections {
		addSection(section, 0)
	}

	// Only add glossary section if there are referenced terms
	if len(doc.Glossary) > 0 {
		lineSpecs = append(lineSpecs, layoutLine{Text: "Glossary", FontSize: 14, Kind: "glossary-heading"})
		for _, entry := range doc.Glossary {
			lineSpecs = append(lineSpecs, layoutLine{Text: entry.Term, FontSize: 11, Kind: "glossary"})
			if entry.Definition != "" {
				for _, wrapped := range wrapTextPts(entry.Definition, textWidth-glossIndent, 10.0) {
					lineSpecs = append(lineSpecs, layoutLine{Text: "  " + wrapped, FontSize: 10, Kind: "glossary-definition"})
				}
			}
			if entry.TermURI != "" {
				lineSpecs = append(lineSpecs, layoutLine{Text: "  -> " + entry.TermURI, FontSize: 9, Kind: "glossary-uri"})
			}
		}
	}

	pages = []pageLayout{{}}
	pageIndex := 0
	y := pageHeight - topMargin
	mcid := 0

	termToGlossaryIndex := make(map[string]int)
	for i, term := range doc.Glossary {
		termToGlossaryIndex[term.TermURI] = i
	}

	// preSpacing defines extra vertical space inserted *before* a line of the
	// given kind. This space is not applied at the very top of a new page.
	preSpacing := map[string]float64{
		"meta":               6.0,
		"section-heading":    18.0,
		"subsection-heading": 10.0,
		"glossary-heading":   20.0,
		"glossary":           6.0,
	}

	var positionedLines []positionedLine
	atPageTop := true // true immediately after a page break (or page 1 start)

	for lineIdx, spec := range lineSpecs {
		lineHeight := spec.FontSize * 1.45
		extra := preSpacing[spec.Kind]
		if atPageTop {
			extra = 0
		}
		// Glossary always starts on a fresh page regardless of remaining space.
		if spec.Kind == "glossary-heading" {
			pages = append(pages, pageLayout{})
			pageIndex++
			y = pageHeight - topMargin
			extra = 0
		} else if y-(lineHeight+extra) < bottomMargin {
			pages = append(pages, pageLayout{})
			pageIndex++
			y = pageHeight - topMargin
			extra = 0
		}
		y -= extra
		atPageTop = false

		sectionIdx := -1
		localClauseIdx := -1
		for _, meta := range lineToClause {
			if meta.lineIndex == lineIdx {
				sectionIdx = meta.sectionIdx
				localClauseIdx = meta.localClauseIdx
				break
			}
		}

		lineX := leftMargin + float64(spec.Depth)*depthIndent

		posLine := positionedLine{
			Text:           spec.Text,
			FontSize:       spec.FontSize,
			X:              lineX,
			Y:              y,
			MCID:           mcid,
			Kind:           spec.Kind,
			PageIdx:        pageIndex,
			SectionIdx:     sectionIdx,
			LocalClauseIdx: localClauseIdx,
			SectionDepth:   spec.Depth,
		}
		positionedLines = append(positionedLines, posLine)
		pages[pageIndex].Lines = append(pages[pageIndex].Lines, posLine)
		mcid++

		y -= lineHeight
	}

	// Collect positioned glossary lines in order so body-text annotations can
	// reference them by index without a second pass over the full line list.
	var glossaryLines []positionedLine
	for _, pl := range positionedLines {
		if pl.Kind == "glossary" {
			glossaryLines = append(glossaryLines, pl)
		}
	}

	// URI action annotations for glossary-uri lines — use recorded Y directly.
	for _, posLine := range positionedLines {
		if posLine.Kind != "glossary-uri" {
			continue
		}
		uri := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(posLine.Text), "->"))
		if uri == "" {
			continue
		}
		lh := posLine.FontSize * 1.45
		pages[posLine.PageIdx].Annotations = append(pages[posLine.PageIdx].Annotations, annotationRef{
			Term: uri,
			URI:  uri,
			Rect: [4]float64{leftMargin + glossIndent, posLine.Y - 2, leftMargin + strPts(posLine.Text, 9.0), posLine.Y + lh - 4},
		})
	}

	// Glossary destination links from ontology-link segments — use recorded Y directly.
	// SectionIdx now indexes into flatSections (depth-first flattened).
	for _, posLine := range positionedLines {
		if posLine.Kind != "body" || posLine.SectionIdx < 0 || posLine.LocalClauseIdx < 0 {
			continue
		}
		if posLine.SectionIdx >= len(flatSections) {
			continue
		}
		clause := flatSections[posLine.SectionIdx].Clauses[posLine.LocalClauseIdx]
		lh := posLine.FontSize * 1.45
		for _, seg := range clause.Segments {
			if seg.Type != "ontology-link" || seg.Ref == "" {
				continue
			}
			glossIdx, ok := termToGlossaryIndex[seg.Ref]
			if !ok {
				continue
			}
			idx := strings.Index(posLine.Text, seg.Text)
			if idx < 0 {
				continue
			}
			if glossIdx >= len(glossaryLines) {
				continue
			}
			gl := glossaryLines[glossIdx]
			startX := posLine.X + strPts(posLine.Text[:idx], posLine.FontSize)
			endX := startX + strPts(seg.Text, posLine.FontSize)
			pages[posLine.PageIdx].Annotations = append(pages[posLine.PageIdx].Annotations, annotationRef{
				Term:        seg.Text,
				DestPageIdx: gl.PageIdx,
				DestY:       gl.Y,
				Rect:        [4]float64{startX, posLine.Y - 2, endX + 2, posLine.Y + lh - 4},
			})
		}
	}

	if len(doc.SignatureFields) > 0 {
		sigPage := pageLayout{}
		sigY := pageHeight - topMargin
		sigPage.Lines = append(sigPage.Lines, positionedLine{
			Text:           "Signature Fields",
			FontSize:       16,
			X:              leftMargin,
			Y:              sigY,
			MCID:           mcid,
			Kind:           "signature-heading",
			SectionIdx:     -1,
			LocalClauseIdx: -1,
		})
		mcid++
		sigY -= 40
		for _, sigField := range doc.SignatureFields {
			fieldWidth := 360.0
			fieldHeight := 70.0
			if sigY-fieldHeight < bottomMargin {
				break
			}
			sigPage.SigFields = append(sigPage.SigFields, sigFieldWidget{
				Name:  sigField.Name,
				Label: sigField.Label,
				Rect:  [4]float64{leftMargin, sigY - fieldHeight, leftMargin + fieldWidth, sigY},
			})
			sigY -= fieldHeight + 18
		}
		pages = append(pages, sigPage)
	}

	return pages, flatSections, flatDepths
}

// helveticaWidth returns the width of rune c in a 1-point Helvetica font,
// in points. Values are from ISO 32000-1:2008 Annex D (standard Type 1 fonts).
// Unknown glyphs fall back to the Helvetica average (556/1000 pt).
func helveticaWidth(c rune) float64 {
	const avg = 556
	widths := [128]int{
		// 0x20–0x7E printable ASCII
		' ': 278, '!': 278, '"': 355, '#': 556, '$': 556, '%': 889, '&': 667,
		'\'': 191, '(': 333, ')': 333, '*': 389, '+': 584, ',': 278, '-': 333,
		'.': 278, '/': 278,
		'0': 556, '1': 556, '2': 556, '3': 556, '4': 556,
		'5': 556, '6': 556, '7': 556, '8': 556, '9': 556,
		':': 278, ';': 278, '<': 584, '=': 584, '>': 584, '?': 556, '@': 1015,
		'A': 667, 'B': 667, 'C': 722, 'D': 722, 'E': 667, 'F': 611, 'G': 778,
		'H': 722, 'I': 278, 'J': 500, 'K': 667, 'L': 556, 'M': 833, 'N': 722,
		'O': 778, 'P': 667, 'Q': 778, 'R': 722, 'S': 667, 'T': 611, 'U': 722,
		'V': 667, 'W': 944, 'X': 667, 'Y': 667, 'Z': 611,
		'[': 278, '\\': 278, ']': 278, '^': 469, '_': 556, '`': 333,
		'a': 556, 'b': 556, 'c': 500, 'd': 556, 'e': 556, 'f': 278, 'g': 556,
		'h': 556, 'i': 222, 'j': 222, 'k': 500, 'l': 222, 'm': 833, 'n': 556,
		'o': 556, 'p': 556, 'q': 556, 'r': 333, 's': 500, 't': 278, 'u': 556,
		'v': 500, 'w': 722, 'x': 500, 'y': 500, 'z': 500,
		'{': 334, '|': 260, '}': 334, '~': 584,
	}
	if c >= 0 && int(c) < len(widths) && widths[c] != 0 {
		return float64(widths[c]) / 1000.0
	}
	return float64(avg) / 1000.0
}

// strPts returns the width of s rendered in Helvetica at fontSize points.
func strPts(s string, fontSize float64) float64 {
	w := 0.0
	for _, c := range s {
		w += helveticaWidth(c)
	}
	return w * fontSize
}

// wrapTextPts wraps input into lines that fit within maxWidth points when
// rendered in Helvetica at fontSize. Breaks only at word boundaries.
func wrapTextPts(input string, maxWidth float64, fontSize float64) []string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return []string{""}
	}
	words := strings.Fields(trimmed)
	lines := []string{words[0]}
	for _, word := range words[1:] {
		candidate := lines[len(lines)-1] + " " + word
		if strPts(candidate, fontSize) <= maxWidth {
			lines[len(lines)-1] = candidate
			continue
		}
		lines = append(lines, word)
	}
	return lines
}
