package compiler

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func renderPDF(ctx context.Context, doc documentModel) ([]byte, error) {
	pages, flatSections, flatDepths := layoutDocument(doc)
	ids := objectIDs{
		catalogID:        1,
		pagesID:          2,
		acroFormID:       14,
		fontID:           6,
		iccID:            7,
		outputIntentID:   8,
		c2paEmbeddedID:   9,
		c2paFileSpecID:   10,
		embeddedFileID:   11,
		fileSpecID:       12,
		metadataID:       13,
		fontDescriptorID: 16,
		fontFileID:       17,
	}
	nextID := 18
	for pageIndex := range pages {
		pages[pageIndex].ObjectID = nextID
		nextID++
		pages[pageIndex].ContentID = nextID
		nextID++
		for annotationIndex := range pages[pageIndex].Annotations {
			pages[pageIndex].Annotations[annotationIndex].ObjectID = nextID
			nextID++
		}
		for sigIndex := range pages[pageIndex].SigFields {
			pages[pageIndex].SigFields[sigIndex].AppearanceObjectID = nextID
			nextID++
			pages[pageIndex].SigFields[sigIndex].WidgetObjectID = nextID
			nextID++
		}
	}
	// ID 14 (acroFormID) is reserved above; next dynamic IDs start at nextID.

	xmpMetadata := renderXMPMetadata("")
	c2paManifest, err := renderC2PAManifestStore(ctx, doc.ContractID, doc.PayloadHash, payloadHashBytes(doc.PayloadHash), []c2paExclusion{{Start: 0, Length: 0}}, doc.CompiledAt)
	if err != nil {
		return nil, fmt.Errorf("render initial C2PA manifest: %w", err)
	}
	objects := make([]pdfObject, 0, 24+len(pages)*3)
	objects = append(objects,
		// Embedded TrueType font (Liberation Sans, metrically compatible with
		// Helvetica/Arial). PDF/A-3a clause 6.2.11.4.1 requires the font program
		// to be embedded. The Widths array covers ASCII 32-127 using the standard
		// Arial/Liberation Sans advance widths at 1/1000 em.
		pdfObject{ID: ids.fontFileID, Data: streamObject(liberationSansTTF, fmt.Sprintf("<< /Length %d /Length1 %d >>", len(liberationSansTTF), len(liberationSansTTF)))},
		pdfObject{ID: ids.fontDescriptorID, Data: []byte(fmt.Sprintf(
			"<< /Type /FontDescriptor /FontName /LiberationSans /Flags 32"+
				" /FontBBox [-166 -210 1000 1049] /ItalicAngle 0"+
				" /Ascent 905 /Descent -212 /CapHeight 714 /StemV 78"+
				" /FontFile2 %d 0 R >>",
			ids.fontFileID,
		))},
		pdfObject{ID: ids.fontID, Data: []byte(fmt.Sprintf(
			"<< /Type /Font /Subtype /TrueType /BaseFont /LiberationSans /Name /F1"+
				" /Encoding /WinAnsiEncoding /FirstChar 32 /LastChar 127"+
				// Advance widths derived from the embedded Liberation Sans TTF
				// (parseTTFWidths in compiler_pdfa_test.go validates consistency).
				// Chars 32-127; notable differences from Helvetica:
				//   39  ' apostrophe  → 191 (not 222)
				//   96  ` grave       → 333 (not 222)
				//   127 DEL           → 750 (not 350)
				" /Widths [278 278 355 556 556 889 667 191 333 333 389 584 278 333 278 278"+
				" 556 556 556 556 556 556 556 556 556 556 278 278 584 584 584 556 1015"+
				" 667 667 722 722 667 611 778 722 278 500 667 556 833 722 778 667 778"+
				" 722 667 611 722 667 944 667 667 611 278 278 278 469 556 333 556 556"+
				" 500 556 556 278 556 556 222 222 500 222 833 556 556 556 556 333 500"+
				" 278 556 500 722 500 500 500 334 260 334 584 750]"+
				" /FontDescriptor %d 0 R >>",
			ids.fontDescriptorID,
		))},
		pdfObject{ID: ids.iccID, Data: streamObject(srgbICCProfile, fmt.Sprintf("<< /N 3 /Alternate /DeviceRGB /Length %d >>", len(srgbICCProfile)))},
		pdfObject{ID: ids.outputIntentID, Data: []byte(fmt.Sprintf("<< /Type /OutputIntent /S /GTS_PDFA1 /OutputConditionIdentifier (sRGB IEC61966-2.1) /Info (DCS-PDF-CORE synthetic profile) /DestOutputProfile %d 0 R >>", ids.iccID))},
		pdfObject{ID: ids.c2paEmbeddedID, Data: streamObject(c2paManifest, fmt.Sprintf("<< /Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d >>", len(c2paManifest)))},
		// C2PA manifest store attachment per C2PA 2.4 Appendix A.4 for PDF embedding.
		pdfObject{ID: ids.c2paFileSpecID, Data: []byte(fmt.Sprintf("<< /Type /Filespec /F (content_credential.c2pa) /UF (content_credential.c2pa) /AFRelationship /C2PA_Manifest /Desc (Embedded C2PA manifest store) /EF << /F %d 0 R >> >>", ids.c2paEmbeddedID))},
		pdfObject{ID: ids.embeddedFileID, Data: streamObject(doc.EmbeddedPayload, fmt.Sprintf("<< /Type /EmbeddedFile /Subtype /application#2Fld+json /Length %d /Params << /Size %d /ModDate (D:20260604000000Z) /CheckSum <%s> >> >>", len(doc.EmbeddedPayload), len(doc.EmbeddedPayload), doc.PayloadHash[:32]))},
		// PayloadHash is the sha256 of the verbatim embedded payload bytes (a content-address).
		pdfObject{ID: ids.fileSpecID, Data: []byte(fmt.Sprintf("<< /Type /Filespec /F (contract.jsonld) /UF (contract.jsonld) /AFRelationship /Source /Desc (Embedded canonical JSON-LD payload) /EF << /F %d 0 R >> >>", ids.embeddedFileID))},
		pdfObject{ID: ids.metadataID, Data: streamObject(xmpMetadata, fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>", len(xmpMetadata)))},
	)

	for _, page := range pages {
		content := renderContentStream(page)
		objects = append(objects,
			pdfObject{ID: page.ContentID, Data: streamObject([]byte(content), fmt.Sprintf("<< /Length %d >>", len(content)))},
			pdfObject{ID: page.ObjectID, Data: []byte(renderPageObject(page, ids.fontID, ids.pagesID))},
		)
		for _, annotation := range page.Annotations {
			objects = append(objects, pdfObject{ID: annotation.ObjectID, Data: []byte(renderAnnotationObject(annotation, pages))})
		}
		for _, sigField := range page.SigFields {
			appearance := renderSigFieldAppearanceStream(sigField)
			objects = append(objects,
				pdfObject{ID: sigField.AppearanceObjectID, Data: streamObject([]byte(appearance), fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 %.2f %.2f] /Resources << /Font << /F1 %d 0 R >> >> /Length %d >>", sigField.Rect[2]-sigField.Rect[0], sigField.Rect[3]-sigField.Rect[1], ids.fontID, len(appearance)))},
				pdfObject{ID: sigField.WidgetObjectID, Data: []byte(renderSigFieldWidgetObject(sigField, page.ObjectID, sigField.AppearanceObjectID))},
			)
		}
	}

	var sigFieldRefs []string
	for _, page := range pages {
		for _, sigField := range page.SigFields {
			sigFieldRefs = append(sigFieldRefs, fmt.Sprintf("%d 0 R", sigField.WidgetObjectID))
		}
	}
	if len(sigFieldRefs) > 0 {
		objects = append(objects, pdfObject{ID: ids.acroFormID, Data: []byte(fmt.Sprintf("<< /Fields [%s] /SigFlags 3 /DA (/F1 10 Tf 0 g) >>", strings.Join(sigFieldRefs, " ")))})
	}

	// Build dynamic struct tree (replaces static 3-object placeholder).
	structRootID, structObjects := buildStructTreeObjects(flatSections, flatDepths, pages, nextID)
	nextID += len(structObjects)
	objects = append(objects, structObjects...)

	// Build PDF outline (bookmarks) from section headings, if any.
	outlineRootID, outlineObjects := buildOutlineObjects(pages, nextID)
	objects = append(objects, outlineObjects...)

	objects = append(objects,
		pdfObject{ID: ids.pagesID, Data: []byte(renderPagesObject(pages))},
		pdfObject{ID: ids.catalogID, Data: []byte(renderCatalogObject(ids, pages, len(sigFieldRefs) > 0, structRootID, outlineRootID))},
	)

	sort.Slice(objects, func(i, j int) bool {
		return objects[i].ID < objects[j].ID
	})

	var finalPDF []byte
	for iteration := 0; iteration < 4; iteration++ {
		pdf := serializePDF(objects, ids.catalogID, doc.FileID)
		streamStart, streamLen, found := findObjectStreamRange(pdf, ids.c2paEmbeddedID)
		if !found {
			finalPDF = pdf
			break
		}

		exclusions := buildC2PAExclusions(streamStart, streamLen)
		assetHash := sha256WithExclusions(pdf, exclusions)
		nextManifest, err := renderC2PAManifestStore(ctx, doc.ContractID, doc.PayloadHash, assetHash[:], exclusions, doc.CompiledAt)
		if err != nil {
			return nil, fmt.Errorf("render C2PA manifest (iteration %d): %w", iteration, err)
		}
		if bytes.Equal(nextManifest, c2paManifest) {
			finalPDF = pdf
			break
		}

		c2paManifest = nextManifest
		for i := range objects {
			if objects[i].ID == ids.c2paEmbeddedID {
				objects[i].Data = streamObject(c2paManifest, fmt.Sprintf("<< /Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d >>", len(c2paManifest)))
				break
			}
		}
	}
	if finalPDF == nil {
		finalPDF = serializePDF(objects, ids.catalogID, doc.FileID)
	}

	// Self-check: every page content stream byte must be covered by the C2PA
	// hard binding. A failure here is a compiler bug, not a user error.
	if err := CheckPageContentC2PACoverage(finalPDF); err != nil {
		panic(fmt.Sprintf("compiler bug — C2PA coverage invariant violated: %v", err))
	}

	return finalPDF, nil
}

// headingTag returns the PDF structure tag for a heading line at the given
// section nesting depth. Depth 0 = top-level section → /H1, 1 → /H2, etc.
// Glossary headings always use /H2 (they follow the body sections at depth 0).
func headingTag(kind string, depth int) string {
	switch kind {
	case "title":
		return "/Title"
	case "signature-heading":
		return "/H1"
	case "glossary-heading":
		return "/H2"
	case "section-heading", "subsection-heading":
		switch depth {
		case 0:
			return "/H1"
		case 1:
			return "/H2"
		case 2:
			return "/H3"
		default:
			return "/H4"
		}
	}
	return "/P"
}

func lineUsesBoldTextMode(kind string) bool {
	return kind == "section-heading" || kind == "subsection-heading"
}

func renderContentStream(page pageLayout) string {
	var builder strings.Builder
	for _, line := range page.Lines {
		tag := headingTag(line.Kind, line.SectionDepth)
		if tag == "" {
			tag = "/P"
		}
		builder.WriteString(fmt.Sprintf("%s <</MCID %d>> BDC\n", tag, line.MCID))
		builder.WriteString("BT\n")
		builder.WriteString(fmt.Sprintf("/F1 %.2f Tf\n", line.FontSize))
		builder.WriteString("0 g\n")
		if lineUsesBoldTextMode(line.Kind) {
			builder.WriteString("0 G\n0.35 w\n2 Tr\n")
		} else {
			builder.WriteString("0 Tr\n")
		}
		builder.WriteString(fmt.Sprintf("1 0 0 1 %.2f %.2f Tm\n", line.X, line.Y))
		builder.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFString(line.Text)))
		builder.WriteString("ET\nEMC\n")
	}
	return builder.String()
}

func renderPageObject(page pageLayout, fontID int, pagesID int) string {
	annotationRefs := make([]string, 0, len(page.Annotations))
	for _, annotation := range page.Annotations {
		annotationRefs = append(annotationRefs, fmt.Sprintf("%d 0 R", annotation.ObjectID))
	}
	for _, sigField := range page.SigFields {
		annotationRefs = append(annotationRefs, fmt.Sprintf("%d 0 R", sigField.WidgetObjectID))
	}
	annots := "[]"
	if len(annotationRefs) > 0 {
		annots = "[" + strings.Join(annotationRefs, " ") + "]"
	}
	return fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R /Annots %s /Tabs /S /StructParents 0 >>", pagesID, fontID, page.ContentID, annots)
}

func renderCatalogObject(ids objectIDs, pages []pageLayout, hasSigFields bool, structRootID int, outlineRootID int) string {
	nameEntries := []string{fmt.Sprintf("/EmbeddedFiles << /Names [(content_credential.c2pa) %d 0 R (contract.jsonld) %d 0 R] >>", ids.c2paFileSpecID, ids.fileSpecID)}
	acroFormEntry := ""
	if hasSigFields {
		acroFormEntry = fmt.Sprintf(" /AcroForm %d 0 R", ids.acroFormID)
	}
	outlineEntry := ""
	if outlineRootID > 0 {
		outlineEntry = fmt.Sprintf(" /Outlines %d 0 R", outlineRootID)
	}
	return fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /MarkInfo << /Marked true >> /StructTreeRoot %d 0 R /OutputIntents [%d 0 R] /AF [%d 0 R %d 0 R] /Metadata %d 0 R /Names << %s >> /Lang (en-US)%s%s >>", ids.pagesID, structRootID, ids.outputIntentID, ids.c2paFileSpecID, ids.fileSpecID, ids.metadataID, strings.Join(nameEntries, " "), acroFormEntry, outlineEntry)
}

// buildOutlineObjects builds a PDF /Outlines tree from section-heading lines.
// Returns 0, nil when no named sections exist.
func buildOutlineObjects(pages []pageLayout, startID int) (rootID int, objects []pdfObject) {
	type item struct {
		title  string
		pageID int
		destY  float64
	}
	var items []item
	for _, page := range pages {
		for _, line := range page.Lines {
			if line.Kind == "section-heading" || line.Kind == "subsection-heading" || line.Kind == "glossary-heading" {
				items = append(items, item{title: line.Text, pageID: page.ObjectID, destY: line.Y})
			}
		}
	}
	if len(items) == 0 {
		return 0, nil
	}

	rootID = startID
	itemIDs := make([]int, len(items))
	for i := range items {
		itemIDs[i] = startID + 1 + i
	}

	objects = append(objects, pdfObject{ID: rootID, Data: []byte(fmt.Sprintf(
		"<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
		itemIDs[0], itemIDs[len(itemIDs)-1], len(items),
	))})

	for i, itm := range items {
		prevRef, nextRef := "", ""
		if i > 0 {
			prevRef = fmt.Sprintf(" /Prev %d 0 R", itemIDs[i-1])
		}
		if i < len(items)-1 {
			nextRef = fmt.Sprintf(" /Next %d 0 R", itemIDs[i+1])
		}
		dest := fmt.Sprintf("[%d 0 R /XYZ 54.00 %.2f 0]", itm.pageID, itm.destY)
		objects = append(objects, pdfObject{ID: itemIDs[i], Data: []byte(fmt.Sprintf(
			"<< /Title (%s) /Parent %d 0 R /Dest %s%s%s /Count 0 >>",
			escapePDFString(itm.title), rootID, dest, prevRef, nextRef,
		))})
	}
	return rootID, objects
}

// buildStructTreeObjects constructs a complete PDF structure tree from the
// laid-out pages, assigning object IDs starting at startID.
// flatSections and flatDepths are the depth-first flattened section list
// produced by layoutDocument; SectionIdx in positionedLines indexes into them.
// Returns the StructTreeRoot object ID and all struct element objects.
func buildStructTreeObjects(flatSections []sectionData, flatDepths []int, pages []pageLayout, startID int) (rootID int, objects []pdfObject) {
	type lineInfo struct {
		mcid       int
		kind       string
		pageID     int
		sectionIdx int
		clauseIdx  int
		depth      int
	}
	var allLines []lineInfo
	for _, page := range pages {
		for _, line := range page.Lines {
			allLines = append(allLines, lineInfo{
				mcid:       line.MCID,
				kind:       line.Kind,
				pageID:     page.ObjectID,
				sectionIdx: line.SectionIdx,
				clauseIdx:  line.LocalClauseIdx,
				depth:      line.SectionDepth,
			})
		}
	}
	if len(allLines) == 0 {
		return 0, nil
	}

	maxMCID := 0
	for _, l := range allLines {
		if l.mcid > maxMCID {
			maxMCID = l.mcid
		}
	}
	// parentForMCID[mcid] = struct elem object ID that directly contains it.
	parentForMCID := make([]int, maxMCID+1)

	nextID := startID
	alloc := func() int { id := nextID; nextID++; return id }

	firstPageID := pages[0].ObjectID

	// Allocate IDs for fixed top-level elements.
	titleStructID := alloc()
	metaStructID := alloc()

	// Mark title and meta MCIDs.
	for _, l := range allLines {
		switch l.kind {
		case "title":
			parentForMCID[l.mcid] = titleStructID
		case "meta":
			parentForMCID[l.mcid] = metaStructID
		}
	}

	// Build parent/child relationships from the depth-first flat section list.
	// parentSectIdx[i] = index of parent section (-1 for top-level).
	// childSectIdxs[i] = ordered list of direct child section indices.
	parentSectIdx := make([]int, len(flatSections))
	childSectIdxs := make([][]int, len(flatSections))
	for i := range parentSectIdx {
		parentSectIdx[i] = -1
	}
	type depthEntry struct{ idx, depth int }
	var depthStack []depthEntry
	for i, depth := range flatDepths {
		for len(depthStack) > 0 && depthStack[len(depthStack)-1].depth >= depth {
			depthStack = depthStack[:len(depthStack)-1]
		}
		if len(depthStack) > 0 {
			pIdx := depthStack[len(depthStack)-1].idx
			parentSectIdx[i] = pIdx
			childSectIdxs[pIdx] = append(childSectIdxs[pIdx], i)
		}
		depthStack = append(depthStack, depthEntry{i, depth})
	}

	// Per-section struct elems (flat list, depth-first order).
	type sectionStruct struct {
		sectID    int
		headingID int // 0 if no heading
		paraIDs   []int
		depth     int
	}
	sectionStructs := make([]sectionStruct, len(flatSections))
	for sIdx := range flatSections {
		ss := sectionStruct{sectID: alloc(), depth: flatDepths[sIdx]}
		if flatSections[sIdx].Heading != "" {
			ss.headingID = alloc()
			for _, l := range allLines {
				if (l.kind == "section-heading" || l.kind == "subsection-heading") && l.sectionIdx == sIdx {
					parentForMCID[l.mcid] = ss.headingID
				}
			}
		}
		for cIdx := range flatSections[sIdx].Clauses {
			paraID := alloc()
			ss.paraIDs = append(ss.paraIDs, paraID)
			for _, l := range allLines {
				if l.kind == "body" && l.sectionIdx == sIdx && l.clauseIdx == cIdx {
					parentForMCID[l.mcid] = paraID
				}
			}
		}
		sectionStructs[sIdx] = ss
	}

	// Glossary struct elems.
	hasGlossary := false
	for _, l := range allLines {
		if l.kind == "glossary-heading" {
			hasGlossary = true
			break
		}
	}
	glossaryHeadingID := 0
	var glossaryEntryIDs []int
	if hasGlossary {
		glossaryHeadingID = alloc()
		for _, l := range allLines {
			if l.kind == "glossary-heading" {
				parentForMCID[l.mcid] = glossaryHeadingID
			}
		}
		// One entry ID per glossary term; group following def/uri lines with it.
		currentEntryID := 0
		for _, l := range allLines {
			switch l.kind {
			case "glossary":
				currentEntryID = alloc()
				glossaryEntryIDs = append(glossaryEntryIDs, currentEntryID)
				parentForMCID[l.mcid] = currentEntryID
			case "glossary-definition", "glossary-uri":
				if currentEntryID != 0 {
					parentForMCID[l.mcid] = currentEntryID
				}
			}
		}
	}

	// Signature heading struct elem.
	sigHeadingID := 0
	for _, l := range allLines {
		if l.kind == "signature-heading" {
			sigHeadingID = alloc()
			parentForMCID[l.mcid] = sigHeadingID
		}
	}

	// Document and root IDs allocated last so forward refs work.
	docStructID := alloc()
	structRootID := alloc()

	// ---- Emit objects ----

	// Title and meta elems.
	titlePageID, metaPageID := firstPageID, firstPageID
	titleMCID, metaMCID := 0, 0
	for _, l := range allLines {
		if l.kind == "title" {
			titlePageID, titleMCID = l.pageID, l.mcid
		}
		if l.kind == "meta" {
			metaPageID, metaMCID = l.pageID, l.mcid
		}
	}
	objects = append(objects,
		pdfObject{ID: titleStructID, Data: []byte(fmt.Sprintf(
			"<< /Type /StructElem /S /Title /P %d 0 R /Pg %d 0 R /K [%d] >>",
			docStructID, titlePageID, titleMCID,
		))},
		pdfObject{ID: metaStructID, Data: []byte(fmt.Sprintf(
			"<< /Type /StructElem /S /P /P %d 0 R /Pg %d 0 R /K [%d] >>",
			docStructID, metaPageID, metaMCID,
		))},
	)

	// Section elems.
	for sIdx, ss := range sectionStructs {
		if ss.headingID != 0 {
			hPageID := firstPageID
			var hMCIDs []string
			for _, l := range allLines {
				if (l.kind == "section-heading" || l.kind == "subsection-heading") && l.sectionIdx == sIdx {
					hPageID = l.pageID
					hMCIDs = append(hMCIDs, fmt.Sprintf("%d", l.mcid))
				}
			}
			htag := headingTag("section-heading", ss.depth)
			objects = append(objects, pdfObject{ID: ss.headingID, Data: []byte(fmt.Sprintf(
				"<< /Type /StructElem /S %s /P %d 0 R /Pg %d 0 R /K [%s] >>",
				htag, ss.sectID, hPageID, strings.Join(hMCIDs, " "),
			))})
		}

		for cIdx, paraID := range ss.paraIDs {
			var pMCIDs []string
			pPageID := firstPageID
			first := true
			for _, l := range allLines {
				if l.kind == "body" && l.sectionIdx == sIdx && l.clauseIdx == cIdx {
					if first {
						pPageID = l.pageID
						first = false
					}
					pMCIDs = append(pMCIDs, fmt.Sprintf("%d", l.mcid))
				}
			}
			if len(pMCIDs) == 0 {
				continue
			}
			objects = append(objects, pdfObject{ID: paraID, Data: []byte(fmt.Sprintf(
				"<< /Type /StructElem /S /P /P %d 0 R /Pg %d 0 R /K [%s] >>",
				ss.sectID, pPageID, strings.Join(pMCIDs, " "),
			))})
		}

		// Sect elem: K = [headingID, para0, para1, ..., childSect0, ...]
		// Parent: docStructID for top-level sections; parent section's sectID for subsections.
		var sectKids []string
		if ss.headingID != 0 {
			sectKids = append(sectKids, fmt.Sprintf("%d 0 R", ss.headingID))
		}
		for _, pid := range ss.paraIDs {
			sectKids = append(sectKids, fmt.Sprintf("%d 0 R", pid))
		}
		for _, cIdx := range childSectIdxs[sIdx] {
			sectKids = append(sectKids, fmt.Sprintf("%d 0 R", sectionStructs[cIdx].sectID))
		}
		sectPageID := firstPageID
		for _, l := range allLines {
			if l.sectionIdx == sIdx {
				sectPageID = l.pageID
				break
			}
		}
		sectParentID := docStructID
		if parentSectIdx[sIdx] >= 0 {
			sectParentID = sectionStructs[parentSectIdx[sIdx]].sectID
		}
		objects = append(objects, pdfObject{ID: ss.sectID, Data: []byte(fmt.Sprintf(
			"<< /Type /StructElem /S /Sect /P %d 0 R /Pg %d 0 R /K [%s] >>",
			sectParentID, sectPageID, strings.Join(sectKids, " "),
		))})
	}

	// Glossary elems.
	if glossaryHeadingID != 0 {
		ghPageID := firstPageID
		var ghMCIDs []string
		for _, l := range allLines {
			if l.kind == "glossary-heading" {
				ghPageID = l.pageID
				ghMCIDs = append(ghMCIDs, fmt.Sprintf("%d", l.mcid))
			}
		}
		objects = append(objects, pdfObject{ID: glossaryHeadingID, Data: []byte(fmt.Sprintf(
			"<< /Type /StructElem /S /H2 /P %d 0 R /Pg %d 0 R /K [%s] >>",
			docStructID, ghPageID, strings.Join(ghMCIDs, " "),
		))})
	}
	for _, entryID := range glossaryEntryIDs {
		var eMCIDs []string
		ePageID := firstPageID
		first := true
		for _, l := range allLines {
			if parentForMCID[l.mcid] == entryID {
				if first {
					ePageID = l.pageID
					first = false
				}
				eMCIDs = append(eMCIDs, fmt.Sprintf("%d", l.mcid))
			}
		}
		if len(eMCIDs) == 0 {
			continue
		}
		objects = append(objects, pdfObject{ID: entryID, Data: []byte(fmt.Sprintf(
			"<< /Type /StructElem /S /P /P %d 0 R /Pg %d 0 R /K [%s] >>",
			docStructID, ePageID, strings.Join(eMCIDs, " "),
		))})
	}

	// Sig heading elem.
	if sigHeadingID != 0 {
		shPageID := firstPageID
		var shMCIDs []string
		for _, l := range allLines {
			if l.kind == "signature-heading" {
				shPageID = l.pageID
				shMCIDs = append(shMCIDs, fmt.Sprintf("%d", l.mcid))
			}
		}
		objects = append(objects, pdfObject{ID: sigHeadingID, Data: []byte(fmt.Sprintf(
			"<< /Type /StructElem /S /H1 /P %d 0 R /Pg %d 0 R /K [%s] >>",
			docStructID, shPageID, strings.Join(shMCIDs, " "),
		))})
	}

	// Document elem: K = [title, meta, <top-level sections only>, glossaryHeading, entries..., sigHeading]
	// Subsection /Sect elements are children of their parent /Sect, not of /Document.
	var docKids []string
	docKids = append(docKids, fmt.Sprintf("%d 0 R", titleStructID), fmt.Sprintf("%d 0 R", metaStructID))
	for sIdx, ss := range sectionStructs {
		if parentSectIdx[sIdx] == -1 {
			docKids = append(docKids, fmt.Sprintf("%d 0 R", ss.sectID))
		}
	}
	if glossaryHeadingID != 0 {
		docKids = append(docKids, fmt.Sprintf("%d 0 R", glossaryHeadingID))
		for _, eid := range glossaryEntryIDs {
			docKids = append(docKids, fmt.Sprintf("%d 0 R", eid))
		}
	}
	if sigHeadingID != 0 {
		docKids = append(docKids, fmt.Sprintf("%d 0 R", sigHeadingID))
	}
	objects = append(objects, pdfObject{ID: docStructID, Data: []byte(fmt.Sprintf(
		"<< /Type /StructElem /S /Document /P %d 0 R /Pg %d 0 R /K [%s] >>",
		structRootID, firstPageID, strings.Join(docKids, " "),
	))})

	// ParentTree: key 0 → array indexed by MCID (all pages share /StructParents 0).
	parentRefs := make([]string, len(parentForMCID))
	for i, pid := range parentForMCID {
		if pid == 0 {
			parentRefs[i] = fmt.Sprintf("%d 0 R", docStructID)
		} else {
			parentRefs[i] = fmt.Sprintf("%d 0 R", pid)
		}
	}

	objects = append(objects, pdfObject{ID: structRootID, Data: []byte(fmt.Sprintf(
		"<< /Type /StructTreeRoot /RoleMap << /Title /H1 >> /K [%d 0 R] /ParentTree << /Nums [0 [%s]] >> /ParentTreeNextKey 1 >>",
		docStructID, strings.Join(parentRefs, " "),
	))})

	return structRootID, objects
}

func renderSigFieldAppearanceStream(sigField sigFieldWidget) string {
	width := sigField.Rect[2] - sigField.Rect[0]
	height := sigField.Rect[3] - sigField.Rect[1]
	label := sigField.Label
	if strings.TrimSpace(label) == "" {
		label = sigField.Name
	}
	var builder strings.Builder
	builder.WriteString("q\n")
	builder.WriteString("0.95 0.95 0.95 rg\n")
	builder.WriteString(fmt.Sprintf("0 0 %.2f %.2f re f\n", width, height))
	builder.WriteString("0 0 0 RG\n1 w\n")
	builder.WriteString(fmt.Sprintf("0 0 %.2f %.2f re S\n", width, height))
	builder.WriteString("BT\n/F1 11 Tf\n0 g\n")
	builder.WriteString(fmt.Sprintf("1 0 0 1 10 %.2f Tm\n", height-24))
	builder.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFString(label)))
	builder.WriteString("ET\nQ\n")
	return builder.String()
}

func renderSigFieldWidgetObject(sigField sigFieldWidget, pageObjectID int, appearanceObjectID int) string {
	label := sigField.Label
	if strings.TrimSpace(label) == "" {
		label = sigField.Name
	}
	return fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Sig /F 4 /T (%s) /TU (%s) /Rect [%.2f %.2f %.2f %.2f] /P %d 0 R /AP << /N %d 0 R >> >>",
		escapePDFString(sigField.Name),
		escapePDFString(label),
		sigField.Rect[0], sigField.Rect[1], sigField.Rect[2], sigField.Rect[3],
		pageObjectID,
		appearanceObjectID,
	)
}

func renderAnnotationObject(annotation annotationRef, pages []pageLayout) string {
	rect := annotation.Rect
	// /F 4 sets the Print flag, required for all non-Popup annotations by
	// ISO 19005-3:2012 clause 6.3.2.
	if annotation.URI != "" {
		return fmt.Sprintf("<< /Type /Annot /Subtype /Link /F 4 /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] /A << /S /URI /URI (%s) >> /Contents (%s) >>", rect[0], rect[1], rect[2], rect[3], escapePDFString(annotation.URI), escapePDFString(annotation.Term))
	}
	// GoTo action with explicit array destination. The /A << /S /GoTo /D [page /XYZ x y 0] >>
	// form is universally supported; a bare /Dest entry in a Link annotation is not
	// processed reliably by all PDF viewers (Edge, Foxit, SumatraPDF ignore it).
	destPageObjID := pages[annotation.DestPageIdx].ObjectID
	return fmt.Sprintf("<< /Type /Annot /Subtype /Link /F 4 /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] /A << /S /GoTo /D [%d 0 R /XYZ 54.00 %.2f 0] >> /Contents (%s) >>", rect[0], rect[1], rect[2], rect[3], destPageObjID, annotation.DestY, escapePDFString(annotation.Term))
}

func renderPagesObject(pages []pageLayout) string {
	kids := make([]string, 0, len(pages))
	for _, page := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", page.ObjectID))
	}
	return fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))
}

// renderXMPMetadata renders the XMP metadata packet. When remoteManifestURL
// is non-empty, it adds a dcterms:provenance property referencing it — the
// C2PA-normative mechanism for remote manifest discovery (DCS-OR-C2PA-008
// AC3). This is deliberately NOT done via a "remote_manifests" C2PA claim
// field: c2pa-rs 0.85.1 (c2patool 0.26.61) hard-rejects that as an unknown V2
// claim field ("claim could not be converted from CBOR"), which broke
// c2patool/veraPDF validation for every DCS-produced PDF carrying a manifest
// URL. dcterms is a well-known, PDF/A-registered Dublin Core namespace (see
// ISO 19005-3:2012 clause 6.6.2.3.1, which only prohibits UNregistered
// schemas such as the ad-hoc http://c2pa.org/c2pa one — C2PA provenance data
// itself still belongs in the binary JUMBF attachment, not in XMP).
func renderXMPMetadata(remoteManifestURL string) []byte {
	// ISO 19005-3:2012 clause 6.6.4 requires the PDF/A version and conformance
	// level to be declared via the pdfaid schema (pdfaid:part=3, pdfaid:conformance=A).
	// The xpacket processing instructions are required by ISO 19005-3:2012
	// clause 6.6.3. The begin attribute carries the UTF-8 BOM (U+FEFF) so
	// tools can detect byte order; the id value is fixed by the XMP spec.
	// The XML declaration must precede the xpacket PI (it may not appear
	// after a processing instruction in the XML prolog).
	provenanceDescription := ""
	if remoteManifestURL != "" {
		provenanceDescription = "  <rdf:Description rdf:about=\"\"\n" +
			"    xmlns:dcterms=\"http://purl.org/dc/terms/\"\n" +
			"    dcterms:provenance=\"" + remoteManifestURL + "\"/>\n" +
			// ISO 19005-3:2012 clause 6.6.2.3.2: dcterms is not a predefined XMP
			// schema, so it must be declared as a PDF/A extension schema.
			"  <rdf:Description rdf:about=\"\"\n" +
			"    xmlns:pdfaExtension=\"http://www.aiim.org/pdfa/ns/extension/\"\n" +
			"    xmlns:pdfaSchema=\"http://www.aiim.org/pdfa/ns/schema#\"\n" +
			"    xmlns:pdfaProperty=\"http://www.aiim.org/pdfa/ns/property#\">\n" +
			"    <pdfaExtension:schemas>\n" +
			"      <rdf:Bag>\n" +
			"        <rdf:li rdf:parseType=\"Resource\">\n" +
			"          <pdfaSchema:schema>Dublin Core Terms</pdfaSchema:schema>\n" +
			"          <pdfaSchema:namespaceURI>http://purl.org/dc/terms/</pdfaSchema:namespaceURI>\n" +
			"          <pdfaSchema:prefix>dcterms</pdfaSchema:prefix>\n" +
			"          <pdfaSchema:property>\n" +
			"            <rdf:Seq>\n" +
			"              <rdf:li rdf:parseType=\"Resource\">\n" +
			"                <pdfaProperty:name>provenance</pdfaProperty:name>\n" +
			"                <pdfaProperty:valueType>Text</pdfaProperty:valueType>\n" +
			"                <pdfaProperty:category>external</pdfaProperty:category>\n" +
			"                <pdfaProperty:description>Remote C2PA manifest URL</pdfaProperty:description>\n" +
			"              </rdf:li>\n" +
			"            </rdf:Seq>\n" +
			"          </pdfaSchema:property>\n" +
			"        </rdf:li>\n" +
			"      </rdf:Bag>\n" +
			"    </pdfaExtension:schemas>\n" +
			"  </rdf:Description>\n"
	}
	xmp := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<?xpacket begin=\"\xef\xbb\xbf\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n" +
		"<x:xmpmeta xmlns:x=\"adobe:ns:meta/\" x:xmptk=\"DCS-PDF-CORE 1.0\">\n" +
		"<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n" +
		"  <rdf:Description rdf:about=\"\"\n" +
		"    xmlns:pdfaid=\"http://www.aiim.org/pdfa/ns/id/\"\n" +
		"    pdfaid:part=\"3\"\n" +
		"    pdfaid:conformance=\"A\"/>\n" +
		"  <rdf:Description rdf:about=\"\"\n" +
		"    xmlns:xmp=\"http://ns.adobe.com/xap/1.0/\"\n" +
		"    xmp:CreatorTool=\"DCS-PDF-CORE " + RendererVersion + "\"\n" +
		"    xmp:MetadataDate=\"2026-06-04T00:00:00Z\"/>\n" +
		provenanceDescription +
		"</rdf:RDF>\n" +
		"</x:xmpmeta>\n" +
		"<?xpacket end=\"w\"?>"
	return []byte(xmp)
}

func streamObject(stream []byte, dict string) []byte {
	var builder bytes.Buffer
	builder.WriteString(dict)
	builder.WriteString("\nstream\n")
	builder.Write(stream)
	builder.WriteString("\nendstream")
	return builder.Bytes()
}

func serializePDF(objects []pdfObject, rootID int, fileID string) []byte {
	var builder bytes.Buffer
	// The second line must be a comment (%) followed by at least four bytes
	// each with value > 127, per ISO 19005-3:2012 clause 6.1.2. This signals
	// that the file contains binary data and prevents text-mode line-ending
	// conversion.
	builder.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	maxID := 0
	for _, object := range objects {
		if object.ID > maxID {
			maxID = object.ID
		}
	}
	offsets := make([]int, maxID+1)
	for _, object := range objects {
		offsets[object.ID] = builder.Len()
		builder.WriteString(fmt.Sprintf("%d 0 obj\n", object.ID))
		builder.Write(object.Data)
		builder.WriteString("\nendobj\n")
	}
	startxref := builder.Len()
	builder.WriteString(fmt.Sprintf("xref\n0 %d\n", maxID+1))
	builder.WriteString("0000000000 65535 f \n")
	for id := 1; id <= maxID; id++ {
		if offsets[id] == 0 {
			builder.WriteString("0000000000 00000 f \n")
			continue
		}
		builder.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[id]))
	}
	builder.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root %d 0 R /ID [<%s> <%s>] >>\nstartxref\n%d\n%%%%EOF\n", maxID+1, rootID, fileID, fileID, startxref))
	return builder.Bytes()
}

func previousStartXref(pdf []byte) (int, error) {
	idx := bytes.LastIndex(pdf, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("startxref marker not found")
	}
	match := startXrefPattern.FindSubmatch(pdf[idx:])
	if len(match) != 2 {
		return 0, fmt.Errorf("startxref value not found")
	}
	value, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return 0, fmt.Errorf("invalid startxref value: %w", err)
	}
	return value, nil
}

func escapePDFString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `(`, `\(`)
	value = strings.ReplaceAll(value, `)`, `\)`)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
