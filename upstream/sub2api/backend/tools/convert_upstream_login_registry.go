package main

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	upstreamnotify "github.com/Wei-Shaw/sub2api/internal/notify"
)

const (
	maxWorkbookBytes = 16 << 20
	maxXMLPartBytes  = 8 << 20
)

var (
	errUnsafeInput      = errors.New("unsafe input workbook")
	errUnsafeOutput     = errors.New("unsafe output registry")
	errInvalidWorkbook  = errors.New("invalid workbook")
	errRegistryConflict = errors.New("conflicting credentials for normalized base URL")
)

type registryEntry struct {
	LoginAccount  string `json:"login_account"`
	LoginPassword string `json:"login_password"`
}

type registryDocument struct {
	Version int                      `json:"version"`
	Entries map[string]registryEntry `json:"entries"`
}

type workbookXML struct {
	Sheets []workbookSheet `xml:"sheets>sheet"`
}

type workbookSheet struct {
	RelationshipID string `xml:"id,attr"`
}

type relationshipsXML struct {
	Relationships []workbookRelationship `xml:"Relationship"`
}

type workbookRelationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

type worksheetXML struct {
	Rows []worksheetRow `xml:"sheetData>row"`
}

type worksheetRow struct {
	Number int             `xml:"r,attr"`
	Cells  []worksheetCell `xml:"c"`
}

type worksheetCell struct {
	Reference string       `xml:"r,attr"`
	Type      string       `xml:"t,attr"`
	Value     string       `xml:"v"`
	Inline    richTextItem `xml:"is"`
}

type sharedStringsXML struct {
	Items []richTextItem `xml:"si"`
}

type richTextItem struct {
	Text string        `xml:"t"`
	Runs []richTextRun `xml:"r"`
}

type richTextRun struct {
	Text string `xml:"t"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "upstream login registry conversion failed:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("convert-upstream-login-registry", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "protected XLSX input")
	output := flags.String("output", "", "protected JSON output")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*input) == "" || strings.TrimSpace(*output) == "" {
		return errors.New("both -input and -output are required")
	}
	return convertUpstreamLoginRegistry(*input, *output)
}

func convertUpstreamLoginRegistry(inputPath, outputPath string) error {
	input, inputInfo, err := openProtectedWorkbook(inputPath)
	if err != nil {
		return err
	}
	defer input.Close()

	rows, err := readWorkbookRows(input, inputInfo.Size())
	if err != nil {
		return errInvalidWorkbook
	}
	entries, err := buildRegistryEntries(rows)
	if err != nil {
		return err
	}
	document := registryDocument{Version: 1, Entries: entries}
	raw, err := json.Marshal(document)
	if err != nil {
		return errUnsafeOutput
	}
	raw = append(raw, '\n')
	return writeProtectedRegistry(outputPath, raw)
}

func openProtectedWorkbook(inputPath string) (*os.File, os.FileInfo, error) {
	if err := validateProtectedParent(filepath.Dir(inputPath), errUnsafeInput); err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(inputPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > maxWorkbookBytes {
		return nil, nil, errUnsafeInput
	}
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, nil, errUnsafeInput
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm() != 0o600 || !os.SameFile(info, openedInfo) {
		file.Close()
		return nil, nil, errUnsafeInput
	}
	return file, openedInfo, nil
}

func validateProtectedParent(parentPath string, failure error) error {
	info, err := os.Lstat(parentPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return failure
	}
	return nil
}

func readWorkbookRows(reader io.ReaderAt, size int64) ([]worksheetRow, error) {
	archive, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, err
	}
	parts := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		cleanName := path.Clean(file.Name)
		if cleanName != file.Name || strings.HasPrefix(cleanName, "../") || strings.HasPrefix(cleanName, "/") {
			return nil, errInvalidWorkbook
		}
		parts[cleanName] = file
	}

	var workbook workbookXML
	if err := decodeXMLPart(parts["xl/workbook.xml"], &workbook); err != nil || len(workbook.Sheets) == 0 {
		return nil, errInvalidWorkbook
	}
	var relationships relationshipsXML
	if err := decodeXMLPart(parts["xl/_rels/workbook.xml.rels"], &relationships); err != nil {
		return nil, errInvalidWorkbook
	}
	sheetPath, err := firstWorksheetPath(workbook.Sheets[0].RelationshipID, relationships.Relationships)
	if err != nil {
		return nil, err
	}

	sharedStrings, err := readSharedStrings(parts["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	var worksheet worksheetXML
	if err := decodeXMLPart(parts[sheetPath], &worksheet); err != nil {
		return nil, errInvalidWorkbook
	}
	for rowIndex := range worksheet.Rows {
		for cellIndex := range worksheet.Rows[rowIndex].Cells {
			cell := &worksheet.Rows[rowIndex].Cells[cellIndex]
			value, err := cellText(*cell, sharedStrings)
			if err != nil {
				return nil, errInvalidWorkbook
			}
			cell.Value = value
		}
	}
	return worksheet.Rows, nil
}

func firstWorksheetPath(relationshipID string, relationships []workbookRelationship) (string, error) {
	for _, relationship := range relationships {
		if relationship.ID != relationshipID || !strings.HasSuffix(relationship.Type, "/worksheet") {
			continue
		}
		target := strings.TrimSpace(relationship.Target)
		if target == "" || strings.Contains(target, "\\") {
			return "", errInvalidWorkbook
		}
		var resolved string
		if strings.HasPrefix(target, "/") {
			resolved = path.Clean(strings.TrimPrefix(target, "/"))
		} else {
			resolved = path.Clean(path.Join("xl", target))
		}
		if !strings.HasPrefix(resolved, "xl/worksheets/") {
			return "", errInvalidWorkbook
		}
		return resolved, nil
	}
	return "", errInvalidWorkbook
}

func readSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	var document sharedStringsXML
	if err := decodeXMLPart(file, &document); err != nil {
		return nil, err
	}
	values := make([]string, len(document.Items))
	for index, item := range document.Items {
		values[index] = richText(item)
	}
	return values, nil
}

func decodeXMLPart(file *zip.File, target any) error {
	if file == nil || file.UncompressedSize64 > maxXMLPartBytes {
		return errInvalidWorkbook
	}
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, maxXMLPartBytes+1)
	decoder := xml.NewDecoder(limited)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.InputOffset() > maxXMLPartBytes {
		return errInvalidWorkbook
	}
	return nil
}

func cellText(cell worksheetCell, sharedStrings []string) (string, error) {
	switch cell.Type {
	case "inlineStr":
		return richText(cell.Inline), nil
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err != nil || index < 0 || index >= len(sharedStrings) {
			return "", errInvalidWorkbook
		}
		return sharedStrings[index], nil
	case "", "n", "str", "b":
		return cell.Value, nil
	default:
		return "", errInvalidWorkbook
	}
}

func richText(item richTextItem) string {
	if len(item.Runs) == 0 {
		return item.Text
	}
	var builder strings.Builder
	builder.WriteString(item.Text)
	for _, run := range item.Runs {
		builder.WriteString(run.Text)
	}
	return builder.String()
}

func buildRegistryEntries(rows []worksheetRow) (map[string]registryEntry, error) {
	if len(rows) == 0 {
		return nil, errInvalidWorkbook
	}
	header := rowValues(rows[0])
	if !strings.EqualFold(strings.TrimSpace(header["C"]), "baseURL") || strings.TrimSpace(header["D"]) != "账号" || strings.TrimSpace(header["E"]) != "密码" {
		return nil, errInvalidWorkbook
	}
	entries := make(map[string]registryEntry)
	for _, row := range rows[1:] {
		values := rowValues(row)
		rawBaseURL := strings.TrimSpace(values["C"])
		if rawBaseURL == "" {
			continue
		}
		normalized, err := upstreamnotify.NormalizeBaseURL(rawBaseURL)
		if err != nil {
			return nil, errInvalidWorkbook
		}
		entry := registryEntry{
			LoginAccount:  strings.TrimSpace(values["D"]),
			LoginPassword: strings.TrimSpace(values["E"]),
		}
		if existing, found := entries[normalized]; found && existing != entry {
			return nil, errRegistryConflict
		}
		entries[normalized] = entry
	}
	return entries, nil
}

func rowValues(row worksheetRow) map[string]string {
	values := make(map[string]string, len(row.Cells))
	for _, cell := range row.Cells {
		column := cellColumn(cell.Reference)
		if column != "" {
			values[column] = cell.Value
		}
	}
	return values
}

func cellColumn(reference string) string {
	index := 0
	for index < len(reference) && reference[index] >= 'A' && reference[index] <= 'Z' {
		index++
	}
	if index == 0 {
		return ""
	}
	return reference[:index]
}

func writeProtectedRegistry(outputPath string, raw []byte) error {
	parent := filepath.Dir(outputPath)
	if err := validateProtectedParent(parent, errUnsafeOutput); err != nil {
		return err
	}
	if info, err := os.Lstat(outputPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return errUnsafeOutput
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errUnsafeOutput
	}

	temporary, err := os.CreateTemp(parent, ".upstream-login-registry-*.tmp")
	if err != nil {
		return errUnsafeOutput
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return errUnsafeOutput
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return errUnsafeOutput
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return errUnsafeOutput
	}
	if err := temporary.Close(); err != nil {
		return errUnsafeOutput
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return errUnsafeOutput
	}
	removeTemporary = false
	info, err := os.Lstat(outputPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errUnsafeOutput
	}
	return nil
}
