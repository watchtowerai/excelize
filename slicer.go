// Copyright 2016 - 2025 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.
//
// Package excelize providing a set of functions that allow you to write to and
// read from XLAM / XLSM / XLSX / XLTM / XLTX files. Supports reading and
// writing spreadsheet documents generated by Microsoft Excel™ 2007 and later.
// Supports complex components by high compatibility, and provided streaming
// API for generating or reading data from a worksheet with huge amounts of
// data. This library needs Go version 1.20 or later.

package excelize

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// SlicerOptions represents the settings of the slicer.
//
// Name specifies the slicer name, should be an existing field name of the given
// table or pivot table, this setting is required.
//
// Cell specifies the left top cell coordinates the position for inserting the
// slicer, this setting is required.
//
// TableSheet specifies the worksheet name of the table or pivot table, this
// setting is required.
//
// TableName specifies the name of the table or pivot table, this setting is
// required.
//
// Caption specifies the caption of the slicer, this setting is optional.
//
// Macro used for set macro for the slicer, the workbook extension should be
// XLSM or XLTM.
//
// Width specifies the width of the slicer, this setting is optional.
//
// Height specifies the height of the slicer, this setting is optional.
//
// DisplayHeader specifies if display header of the slicer, this setting is
// optional, the default setting is display.
//
// ItemDesc specifies descending (Z-A) item sorting, this setting is optional,
// and the default setting is false (represents ascending).
//
// Format specifies the format of the slicer, this setting is optional.
type SlicerOptions struct {
	slicerXML       string
	slicerCacheXML  string
	slicerCacheName string
	slicerSheetName string
	slicerSheetRID  string
	drawingXML      string
	Name            string
	Cell            string
	TableSheet      string
	TableName       string
	Caption         string
	Macro           string
	Width           uint
	Height          uint
	DisplayHeader   *bool
	ItemDesc        bool
	Format          GraphicOptions
}

// AddSlicer function inserts a slicer by giving the worksheet name and slicer
// settings.
//
// For example, insert a slicer on the Sheet1!E1 with field Column1 for the
// table named Table1:
//
//	err := f.AddSlicer("Sheet1", &excelize.SlicerOptions{
//	    Name:       "Column1",
//	    Cell:       "E1",
//	    TableSheet: "Sheet1",
//	    TableName:  "Table1",
//	    Caption:    "Column1",
//	    Width:      200,
//	    Height:     200,
//	})
func (f *File) AddSlicer(sheet string, opts *SlicerOptions) error {
	opts, err := parseSlicerOptions(opts)
	if err != nil {
		return err
	}
	table, pivotTable, colIdx, err := f.getSlicerSource(opts)
	if err != nil {
		return err
	}
	extURI, ns := ExtURISlicerListX14, NameSpaceDrawingMLA14
	if table != nil {
		extURI = ExtURISlicerListX15
		ns = NameSpaceDrawingMLSlicerX15
	}
	slicerID, err := f.addSheetSlicer(sheet, extURI)
	if err != nil {
		return err
	}
	slicerCacheName, err := f.setSlicerCache(colIdx, opts, table, pivotTable)
	if err != nil {
		return err
	}
	slicerName := f.genSlicerName(opts.Name)
	if err := f.addDrawingSlicer(sheet, slicerName, ns, opts); err != nil {
		return err
	}
	return f.addSlicer(slicerID, xlsxSlicer{
		Name:        slicerName,
		Cache:       slicerCacheName,
		Caption:     opts.Caption,
		ShowCaption: opts.DisplayHeader,
		RowHeight:   251883,
	})
}

// parseSlicerOptions provides a function to parse the format settings of the
// slicer with default value.
func parseSlicerOptions(opts *SlicerOptions) (*SlicerOptions, error) {
	if opts == nil {
		return nil, ErrParameterRequired
	}
	if opts.Name == "" || opts.Cell == "" || opts.TableSheet == "" || opts.TableName == "" {
		return nil, ErrParameterInvalid
	}
	if opts.Width == 0 {
		opts.Width = defaultSlicerWidth
	}
	if opts.Height == 0 {
		opts.Height = defaultSlicerHeight
	}
	if opts.Format.PrintObject == nil {
		opts.Format.PrintObject = boolPtr(true)
	}
	if opts.Format.Locked == nil {
		opts.Format.Locked = boolPtr(false)
	}
	if opts.Format.ScaleX == 0 {
		opts.Format.ScaleX = defaultDrawingScale
	}
	if opts.Format.ScaleY == 0 {
		opts.Format.ScaleY = defaultDrawingScale
	}
	return opts, nil
}

// countSlicers provides a function to get slicer files count storage in the
// folder xl/slicers.
func (f *File) countSlicers() int {
	count := 0
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/slicers/slicer") {
			count++
		}
		return true
	})
	return count
}

// countSlicerCache provides a function to get slicer cache files count storage
// in the folder xl/SlicerCaches.
func (f *File) countSlicerCache() int {
	count := 0
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/slicerCaches/slicerCache") {
			count++
		}
		return true
	})
	return count
}

// getSlicerSource returns the slicer data source table or pivot table settings
// and the index of the given slicer fields in the table or pivot table
// column.
func (f *File) getSlicerSource(opts *SlicerOptions) (*Table, *PivotTableOptions, int, error) {
	var (
		table       *Table
		pivotTable  *PivotTableOptions
		colIdx      int
		err         error
		dataRange   string
		tables      []Table
		pivotTables []PivotTableOptions
	)
	if tables, err = f.GetTables(opts.TableSheet); err != nil {
		return table, pivotTable, colIdx, err
	}
	for _, tbl := range tables {
		if tbl.Name == opts.TableName {
			table = &tbl
			dataRange = fmt.Sprintf("%s!%s", opts.TableSheet, tbl.Range)
			break
		}
	}
	if table == nil {
		if pivotTables, err = f.GetPivotTables(opts.TableSheet); err != nil {
			return table, pivotTable, colIdx, err
		}
		for _, tbl := range pivotTables {
			if tbl.Name == opts.TableName {
				pivotTable = &tbl
				dataRange = tbl.DataRange
				break
			}
		}
		if pivotTable == nil {
			return table, pivotTable, colIdx, newNoExistTableError(opts.TableName)
		}
	}
	order, _ := f.getTableFieldsOrder(&PivotTableOptions{DataRange: dataRange})
	if colIdx = inStrSlice(order, opts.Name, true); colIdx == -1 {
		return table, pivotTable, colIdx, newInvalidSlicerNameError(opts.Name)
	}
	return table, pivotTable, colIdx, err
}

// addSheetSlicer adds a new slicer and updates the namespace and relationships
// parts of the worksheet by giving the worksheet name.
func (f *File) addSheetSlicer(sheet, extURI string) (int, error) {
	var (
		slicerID     = f.countSlicers() + 1
		ws, err      = f.workSheetReader(sheet)
		decodeExtLst = new(decodeExtLst)
	)
	if err != nil {
		return slicerID, err
	}
	if ws.ExtLst != nil {
		if err = f.xmlNewDecoder(strings.NewReader("<extLst>" + ws.ExtLst.Ext + "</extLst>")).
			Decode(decodeExtLst); err != nil && err != io.EOF {
			return slicerID, err
		}
		for _, ext := range decodeExtLst.Ext {
			if ext.URI == extURI {
				slicerList := new(decodeSlicerList)
				_ = f.xmlNewDecoder(strings.NewReader(ext.Content)).Decode(slicerList)
				for _, slicer := range slicerList.Slicer {
					if slicer.RID != "" {
						sheetRelationshipsDrawingXML := f.getSheetRelationshipsTargetByID(sheet, slicer.RID)
						slicerID, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(sheetRelationshipsDrawingXML, "../slicers/slicer"), ".xml"))
						return slicerID, err
					}
				}
			}
		}
	}
	sheetRelationshipsSlicerXML := "../slicers/slicer" + strconv.Itoa(slicerID) + ".xml"
	sheetXMLPath, _ := f.getSheetXMLPath(sheet)
	sheetRels := "xl/worksheets/_rels/" + strings.TrimPrefix(sheetXMLPath, "xl/worksheets/") + ".rels"
	rID := f.addRels(sheetRels, SourceRelationshipSlicer, sheetRelationshipsSlicerXML, "")
	f.addSheetNameSpace(sheet, NameSpaceSpreadSheetX14)
	return slicerID, f.addSheetTableSlicer(ws, rID, extURI)
}

// addSheetTableSlicer adds a new table slicer for the worksheet by giving the
// worksheet relationships ID and extension URI.
func (f *File) addSheetTableSlicer(ws *xlsxWorksheet, rID int, extURI string) error {
	var (
		decodeExtLst                 = new(decodeExtLst)
		err                          error
		slicerListBytes, extLstBytes []byte
	)
	if ws.ExtLst != nil {
		if err = f.xmlNewDecoder(strings.NewReader("<extLst>" + ws.ExtLst.Ext + "</extLst>")).
			Decode(decodeExtLst); err != nil && err != io.EOF {
			return err
		}
	}
	slicerListBytes, _ = xml.Marshal(&xlsxX14SlicerList{
		Slicer: []*xlsxX14Slicer{{RID: "rId" + strconv.Itoa(rID)}},
	})
	ext := &xlsxExt{
		xmlns: []xml.Attr{{Name: xml.Name{Local: "xmlns:" + NameSpaceSpreadSheetX14.Name.Local}, Value: NameSpaceSpreadSheetX14.Value}},
		URI:   extURI, Content: string(slicerListBytes),
	}
	if extURI == ExtURISlicerListX15 {
		ext.xmlns = []xml.Attr{{Name: xml.Name{Local: "xmlns:" + NameSpaceSpreadSheetX15.Name.Local}, Value: NameSpaceSpreadSheetX15.Value}}
	}
	decodeExtLst.Ext = append(decodeExtLst.Ext, ext)
	sort.Slice(decodeExtLst.Ext, func(i, j int) bool {
		return inStrSlice(worksheetExtURIPriority, decodeExtLst.Ext[i].URI, false) <
			inStrSlice(worksheetExtURIPriority, decodeExtLst.Ext[j].URI, false)
	})
	extLstBytes, err = xml.Marshal(decodeExtLst)
	ws.ExtLst = &xlsxExtLst{Ext: strings.TrimSuffix(strings.TrimPrefix(string(extLstBytes), "<extLst>"), "</extLst>")}
	return err
}

// addSlicer adds a new slicer to the workbook by giving the slicer ID and
// settings.
func (f *File) addSlicer(slicerID int, slicer xlsxSlicer) error {
	slicerXML := "xl/slicers/slicer" + strconv.Itoa(slicerID) + ".xml"
	slicers, err := f.slicerReader(slicerXML)
	if err != nil {
		return err
	}
	if err := f.addContentTypePart(slicerID, "slicer"); err != nil {
		return err
	}
	slicers.Slicer = append(slicers.Slicer, slicer)
	output, err := xml.Marshal(slicers)
	f.saveFileList(slicerXML, output)
	return err
}

// genSlicerName generates a unique slicer cache name by giving the slicer name.
func (f *File) genSlicerName(name string) string {
	var (
		cnt        int
		slicerName string
		names      []string
	)
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/slicers/slicer") {
			slicers, err := f.slicerReader(k.(string))
			if err != nil {
				return true
			}
			for _, slicer := range slicers.Slicer {
				names = append(names, slicer.Name)
			}
		}
		if strings.Contains(k.(string), "xl/timelines/timeline") {
			timelines, err := f.timelineReader(k.(string))
			if err != nil {
				return true
			}
			for _, timeline := range timelines.Timeline {
				names = append(names, timeline.Name)
			}
		}
		return true
	})
	slicerName = name
	for {
		tmp := slicerName
		if cnt > 0 {
			tmp = fmt.Sprintf("%s %d", slicerName, cnt)
		}
		if inStrSlice(names, tmp, true) == -1 {
			slicerName = tmp
			break
		}
		cnt++
	}
	return slicerName
}

// genSlicerCacheName generates a unique slicer cache name by giving the slicer name.
func (f *File) genSlicerCacheName(name string) string {
	var (
		cnt             int
		definedNames    []string
		slicerCacheName string
	)
	for _, dn := range f.GetDefinedName() {
		if dn.Scope == "Workbook" {
			definedNames = append(definedNames, dn.Name)
		}
	}
	for i, c := range name {
		if unicode.IsLetter(c) {
			slicerCacheName += string(c)
			continue
		}
		if i > 0 && (unicode.IsDigit(c) || c == '.') {
			slicerCacheName += string(c)
			continue
		}
		slicerCacheName += "_"
	}
	slicerCacheName = fmt.Sprintf("Slicer_%s", slicerCacheName)
	for {
		tmp := slicerCacheName
		if cnt > 0 {
			tmp = fmt.Sprintf("%s%d", slicerCacheName, cnt)
		}
		if inStrSlice(definedNames, tmp, true) == -1 {
			slicerCacheName = tmp
			break
		}
		cnt++
	}
	return slicerCacheName
}

// setSlicerCache check if a slicer cache already exists or add a new slicer
// cache by giving the column index, slicer, table options, and returns the
// slicer cache name.
func (f *File) setSlicerCache(colIdx int, opts *SlicerOptions, table *Table, pivotTable *PivotTableOptions) (string, error) {
	var ok bool
	var slicerCacheName string
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/slicerCaches/slicerCache") {
			slicerCache, err := f.slicerCacheReader(k.(string))
			if err != nil {
				return true
			}
			if pivotTable != nil && slicerCache.PivotTables != nil {
				for _, tbl := range slicerCache.PivotTables.PivotTable {
					if tbl.Name == pivotTable.Name {
						ok, slicerCacheName = true, slicerCache.Name
						return false
					}
				}
			}
			if table == nil || slicerCache.ExtLst == nil {
				return true
			}
			ext := new(xlsxExt)
			_ = f.xmlNewDecoder(strings.NewReader(slicerCache.ExtLst.Ext)).Decode(ext)
			if ext.URI == ExtURISlicerCacheDefinition {
				tableSlicerCache := new(decodeTableSlicerCache)
				_ = f.xmlNewDecoder(strings.NewReader(ext.Content)).Decode(tableSlicerCache)
				if tableSlicerCache.TableID == table.tID && tableSlicerCache.Column == colIdx+1 {
					ok, slicerCacheName = true, slicerCache.Name
					return false
				}
			}
		}
		return true
	})
	if ok {
		return slicerCacheName, nil
	}
	slicerCacheName = f.genSlicerCacheName(opts.Name)
	return slicerCacheName, f.addSlicerCache(slicerCacheName, colIdx, opts, table, pivotTable)
}

// slicerReader provides a function to get the pointer to the structure
// after deserialization of xl/slicers/slicer%d.xml.
func (f *File) slicerReader(slicerXML string) (*xlsxSlicers, error) {
	content, ok := f.Pkg.Load(slicerXML)
	slicer := &xlsxSlicers{
		XMLNSXMC:  SourceRelationshipCompatibility.Value,
		XMLNSX:    NameSpaceSpreadSheet.Value,
		XMLNSXR10: NameSpaceSpreadSheetXR10.Value,
	}
	if ok && content != nil {
		if err := f.xmlNewDecoder(bytes.NewReader(namespaceStrictToTransitional(content.([]byte)))).
			Decode(slicer); err != nil && err != io.EOF {
			return nil, err
		}
	}
	return slicer, nil
}

// slicerCacheReader provides a function to get the pointer to the structure
// after deserialization of xl/slicerCaches/slicerCache%d.xml.
func (f *File) slicerCacheReader(slicerCacheXML string) (*xlsxSlicerCacheDefinition, error) {
	content, ok := f.Pkg.Load(slicerCacheXML)
	slicerCache := &xlsxSlicerCacheDefinition{}
	if ok && content != nil {
		if err := f.xmlNewDecoder(bytes.NewReader(namespaceStrictToTransitional(content.([]byte)))).
			Decode(slicerCache); err != nil && err != io.EOF {
			return nil, err
		}
	}
	return slicerCache, nil
}

// timelineReader provides a function to get the pointer to the structure
// after deserialization of xl/timelines/timeline%d.xml.
func (f *File) timelineReader(timelineXML string) (*xlsxTimelines, error) {
	content, ok := f.Pkg.Load(timelineXML)
	timeline := &xlsxTimelines{
		XMLNSXMC:  SourceRelationshipCompatibility.Value,
		XMLNSX:    NameSpaceSpreadSheet.Value,
		XMLNSXR10: NameSpaceSpreadSheetXR10.Value,
	}
	if ok && content != nil {
		if err := f.xmlNewDecoder(bytes.NewReader(namespaceStrictToTransitional(content.([]byte)))).
			Decode(timeline); err != nil && err != io.EOF {
			return nil, err
		}
	}
	return timeline, nil
}

// addSlicerCache adds a new slicer cache by giving the slicer cache name,
// column index, slicer, and table or pivot table options.
func (f *File) addSlicerCache(slicerCacheName string, colIdx int, opts *SlicerOptions, table *Table, pivotTable *PivotTableOptions) error {
	var (
		sortOrder                                       string
		slicerCacheBytes, tableSlicerBytes, extLstBytes []byte
		extURI                                          = ExtURISlicerCachesX14
		slicerCacheID                                   = f.countSlicerCache() + 1
		decodeExtLst                                    = new(decodeExtLst)
		slicerCache                                     = xlsxSlicerCacheDefinition{
			XMLNSXMC:   SourceRelationshipCompatibility.Value,
			XMLNSX:     NameSpaceSpreadSheet.Value,
			XMLNSX15:   NameSpaceSpreadSheetX15.Value,
			XMLNSXR10:  NameSpaceSpreadSheetXR10.Value,
			Name:       slicerCacheName,
			SourceName: opts.Name,
		}
	)
	if opts.ItemDesc {
		sortOrder = "descending"
	}
	if pivotTable != nil {
		pivotCacheID, err := f.addPivotCacheSlicer(pivotTable)
		if err != nil {
			return err
		}
		slicerCache.PivotTables = &xlsxSlicerCachePivotTables{
			PivotTable: []xlsxSlicerCachePivotTable{
				{TabID: f.getSheetID(opts.TableSheet), Name: pivotTable.Name},
			},
		}
		slicerCache.Data = &xlsxSlicerCacheData{
			Tabular: &xlsxTabularSlicerCache{
				PivotCacheID: pivotCacheID,
				SortOrder:    sortOrder,
				ShowMissing:  boolPtr(false),
				Items: &xlsxTabularSlicerCacheItems{
					Count: 1, I: []xlsxTabularSlicerCacheItem{{S: true}},
				},
			},
		}
	}
	if table != nil {
		tableSlicerBytes, _ = xml.Marshal(&xlsxTableSlicerCache{
			TableID:   table.tID,
			Column:    colIdx + 1,
			SortOrder: sortOrder,
		})
		decodeExtLst.Ext = append(decodeExtLst.Ext, &xlsxExt{
			xmlns: []xml.Attr{{Name: xml.Name{Local: "xmlns:" + NameSpaceSpreadSheetX15.Name.Local}, Value: NameSpaceSpreadSheetX15.Value}},
			URI:   ExtURISlicerCacheDefinition, Content: string(tableSlicerBytes),
		})
		extLstBytes, _ = xml.Marshal(decodeExtLst)
		slicerCache.ExtLst = &xlsxExtLst{Ext: strings.TrimSuffix(strings.TrimPrefix(string(extLstBytes), "<extLst>"), "</extLst>")}
		extURI = ExtURISlicerCachesX15
	}
	slicerCacheXML := "xl/slicerCaches/slicerCache" + strconv.Itoa(slicerCacheID) + ".xml"
	slicerCacheBytes, _ = xml.Marshal(slicerCache)
	f.saveFileList(slicerCacheXML, slicerCacheBytes)
	if err := f.addContentTypePart(slicerCacheID, "slicerCache"); err != nil {
		return err
	}
	if err := f.addWorkbookSlicerCache(slicerCacheID, extURI); err != nil {
		return err
	}
	return f.SetDefinedName(&DefinedName{Name: slicerCacheName, RefersTo: formulaErrorNA})
}

// addPivotCacheSlicer adds a new slicer cache by giving the pivot table options
// and returns pivot table cache ID.
func (f *File) addPivotCacheSlicer(opts *PivotTableOptions) (int, error) {
	var (
		pivotCacheID                  int
		pivotCacheBytes, extLstBytes  []byte
		decodeExtLst                  = new(decodeExtLst)
		decodeX14PivotCacheDefinition = new(decodeX14PivotCacheDefinition)
	)
	pc, err := f.pivotCacheReader(opts.pivotCacheXML)
	if err != nil {
		return pivotCacheID, err
	}
	if pc.ExtLst != nil {
		_ = f.xmlNewDecoder(strings.NewReader("<extLst>" + pc.ExtLst.Ext + "</extLst>")).Decode(decodeExtLst)
		for _, ext := range decodeExtLst.Ext {
			if ext.URI == ExtURIPivotCacheDefinition {
				_ = f.xmlNewDecoder(strings.NewReader(ext.Content)).Decode(decodeX14PivotCacheDefinition)
				return decodeX14PivotCacheDefinition.PivotCacheID, err
			}
		}
	}
	pivotCacheID = f.genPivotCacheDefinitionID()
	pivotCacheBytes, _ = xml.Marshal(&xlsxX14PivotCacheDefinition{PivotCacheID: pivotCacheID})
	ext := &xlsxExt{
		xmlns: []xml.Attr{{Name: xml.Name{Local: "xmlns:" + NameSpaceSpreadSheetX14.Name.Local}, Value: NameSpaceSpreadSheetX14.Value}},
		URI:   ExtURIPivotCacheDefinition, Content: string(pivotCacheBytes),
	}
	decodeExtLst.Ext = append(decodeExtLst.Ext, ext)
	extLstBytes, _ = xml.Marshal(decodeExtLst)
	pc.ExtLst = &xlsxExtLst{Ext: strings.TrimSuffix(strings.TrimPrefix(string(extLstBytes), "<extLst>"), "</extLst>")}
	pivotCache, err := xml.Marshal(pc)
	f.saveFileList(opts.pivotCacheXML, pivotCache)
	return pivotCacheID, err
}

// addDrawingSlicer adds a slicer shape and fallback shape by giving the
// worksheet name, slicer name, and slicer options.
func (f *File) addDrawingSlicer(sheet, slicerName string, ns xml.Attr, opts *SlicerOptions) error {
	drawingID := f.countDrawings() + 1
	drawingXML := "xl/drawings/drawing" + strconv.Itoa(drawingID) + ".xml"
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return err
	}
	drawingID, drawingXML = f.prepareDrawing(ws, drawingID, sheet, drawingXML)
	content, twoCellAnchor, cNvPrID, err := f.twoCellAnchorShape(sheet, drawingXML, opts.Cell, opts.Width, opts.Height, opts.Format)
	if err != nil {
		return err
	}
	graphicFrame := xlsxGraphicFrame{
		Macro: opts.Macro,
		NvGraphicFramePr: xlsxNvGraphicFramePr{
			CNvPr: &xlsxCNvPr{
				ID:   cNvPrID,
				Name: slicerName,
			},
		},
		Xfrm: xlsxXfrm{Off: xlsxOff{}, Ext: aExt{}},
		Graphic: &xlsxGraphic{
			GraphicData: &xlsxGraphicData{
				URI: NameSpaceDrawingMLSlicer.Value,
				Sle: &xlsxSle{XMLNS: NameSpaceDrawingMLSlicer.Value, Name: slicerName},
			},
		},
	}
	graphic, _ := xml.Marshal(graphicFrame)
	sp := xdrSp{
		Macro: opts.Macro,
		NvSpPr: &xdrNvSpPr{
			CNvPr: &xlsxCNvPr{
				ID: cNvPrID,
			},
			CNvSpPr: &xdrCNvSpPr{
				TxBox: true,
			},
		},
		SpPr: &xlsxSpPr{
			Xfrm:      xlsxXfrm{Off: xlsxOff{X: 2914650, Y: 152400}, Ext: aExt{Cx: 1828800, Cy: 2238375}},
			SolidFill: &xlsxInnerXML{Content: "<a:prstClr val=\"white\"/>"},
			PrstGeom: xlsxPrstGeom{
				Prst: "rect",
			},
			Ln: xlsxLineProperties{W: 1, SolidFill: &xlsxInnerXML{Content: "<a:prstClr val=\"black\"/>"}},
		},
		TxBody: &xdrTxBody{
			BodyPr: &aBodyPr{VertOverflow: "clip", HorzOverflow: "clip"},
			P: []*aP{
				{R: &aR{T: "This shape represents a table slicer. Table slicers are not supported in this version of Excel."}},
				{R: &aR{T: "If the shape was modified in an earlier version of Excel, or if the workbook was saved in Excel 2007 or earlier, the slicer can't be used."}},
			},
		},
	}
	shape, _ := xml.Marshal(sp)
	twoCellAnchor.ClientData = &xdrClientData{
		FLocksWithSheet:  *opts.Format.Locked,
		FPrintsWithSheet: *opts.Format.PrintObject,
	}
	choice := xlsxChoice{Requires: ns.Name.Local, Content: string(graphic)}
	if ns.Value == NameSpaceDrawingMLA14.Value { // pivot table slicer
		choice.XMLNSA14 = ns.Value
	}
	if ns.Value == NameSpaceDrawingMLSlicerX15.Value { // table slicer
		choice.XMLNSSle15 = ns.Value
	}
	fallback := xlsxFallback{Content: string(shape)}
	choiceBytes, _ := xml.Marshal(choice)
	shapeBytes, _ := xml.Marshal(fallback)
	twoCellAnchor.AlternateContent = append(twoCellAnchor.AlternateContent, &xlsxAlternateContent{
		XMLNSMC: SourceRelationshipCompatibility.Value,
		Content: string(choiceBytes) + string(shapeBytes),
	})
	content.TwoCellAnchor = append(content.TwoCellAnchor, twoCellAnchor)
	f.Drawings.Store(drawingXML, content)
	return f.addContentTypePart(drawingID, "drawings")
}

// addWorkbookSlicerCache add the association ID of the slicer cache in
// workbook.xml.
func (f *File) addWorkbookSlicerCache(slicerCacheID int, URI string) error {
	var (
		wb                                               *xlsxWorkbook
		err                                              error
		idx                                              int
		appendMode                                       bool
		decodeExtLst                                     = new(decodeExtLst)
		decodeSlicerCaches                               = new(decodeSlicerCaches)
		x14SlicerCaches                                  = new(xlsxX14SlicerCaches)
		x15SlicerCaches                                  = new(xlsxX15SlicerCaches)
		ext                                              *xlsxExt
		slicerCacheBytes, slicerCachesBytes, extLstBytes []byte
	)
	if wb, err = f.workbookReader(); err != nil {
		return err
	}
	rID := f.addRels(f.getWorkbookRelsPath(), SourceRelationshipSlicerCache, fmt.Sprintf("/xl/slicerCaches/slicerCache%d.xml", slicerCacheID), "")
	if wb.ExtLst != nil { // append mode ext
		if err = f.xmlNewDecoder(strings.NewReader("<extLst>" + wb.ExtLst.Ext + "</extLst>")).
			Decode(decodeExtLst); err != nil && err != io.EOF {
			return err
		}
		for idx, ext = range decodeExtLst.Ext {
			if ext.URI == URI {
				_ = f.xmlNewDecoder(strings.NewReader(ext.Content)).Decode(decodeSlicerCaches)
				slicerCache := xlsxX14SlicerCache{RID: fmt.Sprintf("rId%d", rID)}
				slicerCacheBytes, _ = xml.Marshal(slicerCache)
				if URI == ExtURISlicerCachesX14 { // pivot table slicer
					x14SlicerCaches.Content = decodeSlicerCaches.Content + string(slicerCacheBytes)
					x14SlicerCaches.XMLNS = NameSpaceSpreadSheetX14.Value
					slicerCachesBytes, _ = xml.Marshal(x14SlicerCaches)
				}
				if URI == ExtURISlicerCachesX15 { // table slicer
					x15SlicerCaches.Content = decodeSlicerCaches.Content + string(slicerCacheBytes)
					x15SlicerCaches.XMLNS = NameSpaceSpreadSheetX14.Value
					slicerCachesBytes, _ = xml.Marshal(x15SlicerCaches)
				}
				decodeExtLst.Ext[idx].Content = string(slicerCachesBytes)
				appendMode = true
			}
		}
	}
	if !appendMode {
		slicerCache := xlsxX14SlicerCache{RID: fmt.Sprintf("rId%d", rID)}
		slicerCacheBytes, _ = xml.Marshal(slicerCache)
		if URI == ExtURISlicerCachesX14 {
			x14SlicerCaches.Content = string(slicerCacheBytes)
			x14SlicerCaches.XMLNS = NameSpaceSpreadSheetX14.Value
			slicerCachesBytes, _ = xml.Marshal(x14SlicerCaches)
			decodeExtLst.Ext = append(decodeExtLst.Ext, &xlsxExt{
				xmlns: []xml.Attr{{Name: xml.Name{Local: "xmlns:" + NameSpaceSpreadSheetX14.Name.Local}, Value: NameSpaceSpreadSheetX14.Value}},
				URI:   ExtURISlicerCachesX14, Content: string(slicerCachesBytes),
			})
		}
		if URI == ExtURISlicerCachesX15 {
			x15SlicerCaches.Content = string(slicerCacheBytes)
			x15SlicerCaches.XMLNS = NameSpaceSpreadSheetX14.Value
			slicerCachesBytes, _ = xml.Marshal(x15SlicerCaches)
			decodeExtLst.Ext = append(decodeExtLst.Ext, &xlsxExt{
				xmlns: []xml.Attr{{Name: xml.Name{Local: "xmlns:" + NameSpaceSpreadSheetX15.Name.Local}, Value: NameSpaceSpreadSheetX15.Value}},
				URI:   ExtURISlicerCachesX15, Content: string(slicerCachesBytes),
			})
		}
	}
	sort.Slice(decodeExtLst.Ext, func(i, j int) bool {
		return inStrSlice(workbookExtURIPriority, decodeExtLst.Ext[i].URI, false) <
			inStrSlice(workbookExtURIPriority, decodeExtLst.Ext[j].URI, false)
	})
	extLstBytes, err = xml.Marshal(decodeExtLst)
	wb.ExtLst = &xlsxExtLst{Ext: strings.TrimSuffix(strings.TrimPrefix(string(extLstBytes), "<extLst>"), "</extLst>")}
	return err
}

// GetSlicers provides the method to get all slicers in a worksheet by a given
// worksheet name. Note that, this function does not support getting the height,
// width, and graphic options of the slicer shape currently.
func (f *File) GetSlicers(sheet string) ([]SlicerOptions, error) {
	var (
		slicers      []SlicerOptions
		ws, err      = f.workSheetReader(sheet)
		decodeExtLst = new(decodeExtLst)
	)
	if err != nil {
		return slicers, err
	}
	if ws.ExtLst == nil {
		return slicers, err
	}
	target := f.getSheetRelationshipsTargetByID(sheet, ws.Drawing.RID)
	drawingXML := strings.TrimPrefix(strings.ReplaceAll(target, "..", "xl"), "/")
	if err = f.xmlNewDecoder(strings.NewReader("<extLst>" + ws.ExtLst.Ext + "</extLst>")).
		Decode(decodeExtLst); err != nil && err != io.EOF {
		return slicers, err
	}
	for _, ext := range decodeExtLst.Ext {
		if ext.URI == ExtURISlicerListX14 || ext.URI == ExtURISlicerListX15 {
			slicerList := new(decodeSlicerList)
			_ = f.xmlNewDecoder(strings.NewReader(ext.Content)).Decode(&slicerList)
			for _, slicer := range slicerList.Slicer {
				if slicer.RID != "" {
					opts, err := f.getSlicers(sheet, slicer.RID, drawingXML)
					if err != nil {
						return slicers, err
					}
					slicers = append(slicers, opts...)
				}
			}
		}
	}
	return slicers, err
}

// getSlicerCache provides a function to get a slicer cache by given slicer
// cache name and slicer options.
func (f *File) getSlicerCache(slicerCacheName string, opt *SlicerOptions) *xlsxSlicerCacheDefinition {
	var (
		err         error
		slicerCache *xlsxSlicerCacheDefinition
	)
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/slicerCaches/slicerCache") {
			slicerCache, err = f.slicerCacheReader(k.(string))
			if err != nil {
				return true
			}
			if slicerCache.Name == slicerCacheName {
				opt.slicerCacheXML = k.(string)
				return false
			}
		}
		return true
	})
	return slicerCache
}

// getSlicers provides a function to get slicers options by given worksheet
// name, slicer part relationship ID and drawing part path.
func (f *File) getSlicers(sheet, rID, drawingXML string) ([]SlicerOptions, error) {
	var (
		opts                        []SlicerOptions
		sheetRelationshipsSlicerXML = f.getSheetRelationshipsTargetByID(sheet, rID)
		slicerXML                   = strings.ReplaceAll(sheetRelationshipsSlicerXML, "..", "xl")
		slicers, err                = f.slicerReader(slicerXML)
	)
	if err != nil {
		return opts, err
	}
	for _, slicer := range slicers.Slicer {
		opt := SlicerOptions{
			slicerXML:       slicerXML,
			slicerCacheName: slicer.Cache,
			slicerSheetName: sheet,
			slicerSheetRID:  rID,
			drawingXML:      drawingXML,
			Name:            slicer.Name,
			Caption:         slicer.Caption,
			DisplayHeader:   slicer.ShowCaption,
		}
		slicerCache := f.getSlicerCache(slicer.Cache, &opt)
		if slicerCache == nil {
			return opts, err
		}
		if err := f.extractTableSlicer(slicerCache, &opt); err != nil {
			return opts, err
		}
		if err := f.extractPivotTableSlicer(slicerCache, &opt); err != nil {
			return opts, err
		}
		if err = f.extractSlicerCellAnchor(drawingXML, &opt); err != nil {
			return opts, err
		}
		opts = append(opts, opt)
	}
	return opts, err
}

// extractTableSlicer extract table slicer options from slicer cache.
func (f *File) extractTableSlicer(slicerCache *xlsxSlicerCacheDefinition, opt *SlicerOptions) error {
	if slicerCache.ExtLst != nil {
		tables, err := f.getTables()
		if err != nil {
			return err
		}
		ext := new(xlsxExt)
		_ = f.xmlNewDecoder(strings.NewReader(slicerCache.ExtLst.Ext)).Decode(ext)
		if ext.URI == ExtURISlicerCacheDefinition {
			tableSlicerCache := new(decodeTableSlicerCache)
			_ = f.xmlNewDecoder(strings.NewReader(ext.Content)).Decode(tableSlicerCache)
			opt.ItemDesc = tableSlicerCache.SortOrder == "descending"
			for sheetName, sheetTables := range tables {
				for _, table := range sheetTables {
					if tableSlicerCache.TableID == table.tID {
						opt.TableName = table.Name
						opt.TableSheet = sheetName
					}
				}
			}
		}
	}
	return nil
}

// extractPivotTableSlicer extract pivot table slicer options from slicer cache.
func (f *File) extractPivotTableSlicer(slicerCache *xlsxSlicerCacheDefinition, opt *SlicerOptions) error {
	pivotTables, err := f.getPivotTables()
	if err != nil {
		return err
	}
	if slicerCache.PivotTables != nil {
		for _, pt := range slicerCache.PivotTables.PivotTable {
			opt.TableName = pt.Name
			for sheetName, sheetPivotTables := range pivotTables {
				for _, pivotTable := range sheetPivotTables {
					if opt.TableName == pivotTable.Name {
						opt.TableSheet = sheetName
					}
				}
			}
		}
		if slicerCache.Data != nil && slicerCache.Data.Tabular != nil {
			opt.ItemDesc = slicerCache.Data.Tabular.SortOrder == "descending"
		}
	}
	return nil
}

// extractSlicerCellAnchor extract slicer drawing object from two cell anchor by
// giving drawing part path and slicer options.
func (f *File) extractSlicerCellAnchor(drawingXML string, opt *SlicerOptions) error {
	var (
		wsDr         *xlsxWsDr
		deCellAnchor = new(decodeCellAnchor)
		deChoice     = new(decodeChoice)
		err          error
	)
	if wsDr, _, err = f.drawingParser(drawingXML); err != nil {
		return err
	}
	wsDr.mu.Lock()
	defer wsDr.mu.Unlock()
	cond := func(ac *xlsxAlternateContent) bool {
		if ac != nil {
			_ = f.xmlNewDecoder(strings.NewReader(ac.Content)).Decode(&deChoice)
			if deChoice.XMLNSSle15 == NameSpaceDrawingMLSlicerX15.Value || deChoice.XMLNSA14 == NameSpaceDrawingMLA14.Value {
				if deChoice.GraphicFrame.NvGraphicFramePr.CNvPr.Name == opt.Name {
					return true
				}
			}
		}
		return false
	}
	for _, anchor := range wsDr.TwoCellAnchor {
		for _, ac := range anchor.AlternateContent {
			if cond(ac) {
				if anchor.From != nil {
					opt.Macro = deChoice.GraphicFrame.Macro
					if opt.Cell, err = CoordinatesToCellName(anchor.From.Col+1, anchor.From.Row+1); err != nil {
						return err
					}
				}
				return err
			}
		}
		_ = f.xmlNewDecoder(strings.NewReader("<decodeCellAnchor>" + anchor.GraphicFrame + "</decodeCellAnchor>")).Decode(&deCellAnchor)
		for _, ac := range deCellAnchor.AlternateContent {
			if cond(ac) {
				if deCellAnchor.From != nil {
					opt.Macro = deChoice.GraphicFrame.Macro
					if opt.Cell, err = CoordinatesToCellName(deCellAnchor.From.Col+1, deCellAnchor.From.Row+1); err != nil {
						return err
					}
				}
				return err
			}
		}
	}
	return err
}

// getAllSlicers provides a function to get all slicers in a workbook.
func (f *File) getAllSlicers() (map[string][]SlicerOptions, error) {
	slicers := map[string][]SlicerOptions{}
	for _, sheetName := range f.GetSheetList() {
		sles, err := f.GetSlicers(sheetName)
		e := ErrSheetNotExist{sheetName}
		if err != nil && err.Error() != newNotWorksheetError(sheetName).Error() && err.Error() != e.Error() {
			return slicers, err
		}
		slicers[sheetName] = append(slicers[sheetName], sles...)
	}
	return slicers, nil
}

// DeleteSlicer provides the method to delete a slicer by a given slicer name.
func (f *File) DeleteSlicer(name string) error {
	sles, err := f.getAllSlicers()
	if err != nil {
		return err
	}
	for _, slicers := range sles {
		for _, slicer := range slicers {
			if slicer.Name != name {
				continue
			}
			_ = f.deleteSlicer(slicer)
			return f.deleteSlicerCache(sles, slicer)
		}
	}
	return newNoExistSlicerError(name)
}

// getSlicers provides a function to delete slicer by given slicer options.
func (f *File) deleteSlicer(opts SlicerOptions) error {
	slicers, err := f.slicerReader(opts.slicerXML)
	if err != nil {
		return err
	}
	for i := 0; i < len(slicers.Slicer); i++ {
		if slicers.Slicer[i].Name == opts.Name {
			slicers.Slicer = append(slicers.Slicer[:i], slicers.Slicer[i+1:]...)
			i--
		}
	}
	if len(slicers.Slicer) == 0 {
		var (
			extLstBytes  []byte
			ws, err      = f.workSheetReader(opts.slicerSheetName)
			decodeExtLst = new(decodeExtLst)
		)
		if err != nil {
			return err
		}
		if err = f.xmlNewDecoder(strings.NewReader("<extLst>" + ws.ExtLst.Ext + "</extLst>")).
			Decode(decodeExtLst); err != nil && err != io.EOF {
			return err
		}
		for i, ext := range decodeExtLst.Ext {
			if ext.URI == ExtURISlicerListX14 || ext.URI == ExtURISlicerListX15 {
				slicerList := new(decodeSlicerList)
				_ = f.xmlNewDecoder(strings.NewReader(ext.Content)).Decode(slicerList)
				for _, slicer := range slicerList.Slicer {
					if slicer.RID == opts.slicerSheetRID {
						decodeExtLst.Ext = append(decodeExtLst.Ext[:i], decodeExtLst.Ext[i+1:]...)
						extLstBytes, err = xml.Marshal(decodeExtLst)
						ws.ExtLst = &xlsxExtLst{Ext: strings.TrimSuffix(strings.TrimPrefix(string(extLstBytes), "<extLst>"), "</extLst>")}
						f.Pkg.Delete(opts.slicerXML)
						_ = f.removeContentTypesPart(ContentTypeSlicer, "/"+opts.slicerXML)
						f.deleteSheetRelationships(opts.slicerSheetName, opts.slicerSheetRID)
						return err
					}
				}
			}
		}
	}
	output, err := xml.Marshal(slicers)
	f.saveFileList(opts.slicerXML, output)
	return err
}

// deleteSlicerCache provides a function to delete the slicer cache by giving
// slicer options if the slicer cache is no longer used.
func (f *File) deleteSlicerCache(sles map[string][]SlicerOptions, opts SlicerOptions) error {
	for _, slicers := range sles {
		for _, slicer := range slicers {
			if slicer.Name != opts.Name && slicer.slicerCacheName == opts.slicerCacheName {
				return nil
			}
		}
	}
	if err := f.DeleteDefinedName(&DefinedName{Name: opts.slicerCacheName}); err != nil {
		return err
	}
	f.Pkg.Delete(opts.slicerCacheXML)
	return f.removeContentTypesPart(ContentTypeSlicerCache, "/"+opts.slicerCacheXML)
}
