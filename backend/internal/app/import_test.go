package app

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	localdb "Vestige/pkg/db"

	"github.com/xuri/excelize/v2"
)

func TestReadContactsXLSXMapsCompanySpreadsheet(t *testing.T) {
	book := excelize.NewFile()
	sheet := book.GetSheetName(0)
	rows := [][]any{
		{"公司名", "发票金额(元)", "是否已收集", "联系电话", "邮箱", "官网", "数据来源", "收集时间", "行业", "规模"},
		{"上海示例企业有限公司", "199.90", "是", "021-00000000", "contact@example.com", "example.com", "官网", "2026-06-01", "教育", "10000人以上"},
	}
	for rowIndex, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, rowIndex+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := book.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := book.Write(&buf); err != nil {
		t.Fatal(err)
	}

	contacts, err := readContactsXLSX(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	contact := contacts[0]
	if contact.Name != "上海示例企业有限公司" || contact.Company != "上海示例企业有限公司" {
		t.Fatalf("unexpected company mapping: %#v", contact)
	}
	if contact.Email != "contact@example.com" {
		t.Fatalf("unexpected email: %s", contact.Email)
	}
	if contact.Phone != "021-00000000" {
		t.Fatalf("unexpected phone: %s", contact.Phone)
	}
	if contact.Tags != "教育,10000人以上" {
		t.Fatalf("unexpected tags: %s", contact.Tags)
	}
}

func TestImportContactsSkipsInvalidEmail(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	router := New(conn, os.DirFS(t.TempDir()))
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "contacts.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("公司名,邮箱\nAcme,hello@example.com\nBad,bad-address\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/contacts/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM contacts WHERE email='bad-address'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected invalid email to be skipped, got %d rows", count)
	}
}
