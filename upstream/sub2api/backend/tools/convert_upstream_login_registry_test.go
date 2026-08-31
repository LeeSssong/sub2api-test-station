package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertUpstreamLoginRegistryReadsFixedColumnsAndMergesMatchingCredentials(t *testing.T) {
	dir := protectedTempDir(t)
	input := filepath.Join(dir, "accounts.xlsx")
	output := filepath.Join(dir, "registry.json")
	require.NoError(t, os.WriteFile(input, workbookFixture(t, [][]string{
		{"账号名称", "账号ID", "baseURL", "账号", "密码"},
		{"first", "11", "HTTPS://UPSTREAM.EXAMPLE.INVALID/v1/", "operator.invalid", "fake-password-a"},
		{"second", "12", "https://upstream.example.invalid/v1", "operator.invalid", "fake-password-a"},
		{"blank", "13", "https://blank.example.invalid/", "", ""},
		{"no-base-url", "14", "", "ignored.invalid", "ignored-password"},
	}), 0o600))

	require.NoError(t, convertUpstreamLoginRegistry(input, output))

	info, err := os.Lstat(output)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	var document registryDocument
	raw, err := os.ReadFile(output)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &document))
	require.Equal(t, 1, document.Version)
	require.Equal(t, map[string]registryEntry{
		"https://blank.example.invalid": {LoginAccount: "", LoginPassword: ""},
		"https://upstream.example.invalid/v1": {
			LoginAccount: "operator.invalid", LoginPassword: "fake-password-a",
		},
	}, document.Entries)
}

func TestConvertUpstreamLoginRegistryRejectsConflictingCredentials(t *testing.T) {
	dir := protectedTempDir(t)
	input := filepath.Join(dir, "accounts.xlsx")
	output := filepath.Join(dir, "registry.json")
	require.NoError(t, os.WriteFile(input, workbookFixture(t, [][]string{
		{"账号名称", "账号ID", "baseURL", "账号", "密码"},
		{"first", "11", "https://upstream.example.invalid/", "operator-a.invalid", "fake-password-a"},
		{"second", "12", "https://UPSTREAM.example.invalid", "operator-b.invalid", "fake-password-b"},
	}), 0o600))

	err := convertUpstreamLoginRegistry(input, output)
	require.ErrorIs(t, err, errRegistryConflict)
	_, statErr := os.Lstat(output)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestConvertUpstreamLoginRegistryAcceptsAbsoluteWorksheetPartName(t *testing.T) {
	dir := protectedTempDir(t)
	input := filepath.Join(dir, "accounts.xlsx")
	output := filepath.Join(dir, "registry.json")
	require.NoError(t, os.WriteFile(input, workbookFixtureWithTarget(t, [][]string{
		{"账号名称", "账号ID", "baseURL", "账号", "密码"},
		{"first", "11", "https://upstream.example.invalid", "operator.invalid", "fake-password-a"},
	}, "/xl/worksheets/sheet1.xml"), 0o600))

	require.NoError(t, convertUpstreamLoginRegistry(input, output))
}

func TestConvertUpstreamLoginRegistryRejectsUnsafeFiles(t *testing.T) {
	dir := protectedTempDir(t)
	input := filepath.Join(dir, "accounts.xlsx")
	output := filepath.Join(dir, "registry.json")
	require.NoError(t, os.WriteFile(input, workbookFixture(t, [][]string{
		{"账号名称", "账号ID", "baseURL", "账号", "密码"},
		{"first", "11", "https://upstream.example.invalid", "operator.invalid", "fake-password-a"},
	}), 0o600))

	require.NoError(t, os.WriteFile(output, []byte("unsafe"), 0o644))
	require.ErrorIs(t, convertUpstreamLoginRegistry(input, output), errUnsafeOutput)

	require.NoError(t, os.Remove(output))
	require.NoError(t, os.Chmod(input, 0o644))
	require.ErrorIs(t, convertUpstreamLoginRegistry(input, output), errUnsafeInput)
}

func protectedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	return dir
}

func workbookFixture(t *testing.T, rows [][]string) []byte {
	t.Helper()
	return workbookFixtureWithTarget(t, rows, "worksheets/sheet1.xml")
}

func workbookFixtureWithTarget(t *testing.T, rows [][]string, worksheetTarget string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	writeZipFixtureFile(t, writer, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`)
	writeZipFixtureFile(t, writer, "xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="accounts" sheetId="1" r:id="rId1"/></sheets>
</workbook>`)
	writeZipFixtureFile(t, writer, "xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="`+xmlEscape(worksheetTarget)+`"/>
</Relationships>`)

	var sheet bytes.Buffer
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range rows {
		sheet.WriteString(`<row r="` + itoa(rowIndex+1) + `">`)
		for columnIndex, value := range row {
			sheet.WriteString(`<c r="` + columnName(columnIndex+1) + itoa(rowIndex+1) + `" t="inlineStr"><is><t>` + xmlEscape(value) + `</t></is></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	writeZipFixtureFile(t, writer, "xl/worksheets/sheet1.xml", sheet.String())
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func writeZipFixtureFile(t *testing.T, writer *zip.Writer, name, value string) {
	t.Helper()
	file, err := writer.Create(name)
	require.NoError(t, err)
	_, err = file.Write([]byte(value))
	require.NoError(t, err)
}

func itoa(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	return string(buffer[index:])
}

func columnName(value int) string {
	name := ""
	for value > 0 {
		value--
		name = string(rune('A'+value%26)) + name
		value /= 26
	}
	return name
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
