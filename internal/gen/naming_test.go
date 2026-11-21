package gen

import "testing"

func TestNaming(t *testing.T) {
	for in, want := range map[string]string{
		"associateId": "AssociateID", "fileIds": "FileIDs", "vatUid": "VatUID", "isDisplayItemGross": "IsDisplayItemGross",
		"DEFAULT_DEBTOR_ACCOUNT_ID": "DefaultDebtorAccountID", "url": "URL", "2fa": "X2fa",
	} {
		if got := goName(in); got != want {
			t.Errorf("goName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"list.json": "List", "list.csv": "ListCSV", "read.pdf": "ReadPDF", "update_attachments.json": "UpdateAttachments",
		"get": "Get", "download": "Download", "list.xlsx": "ListXLSX",
	} {
		if got := methodName(in); got != want {
			t.Errorf("methodName(%q) = %q, want %q", in, got, want)
		}
	}
	for name, want := range map[string]bool{"id": true, "categoryId": true, "fileIds": true, "sequenceNrIdInherited": true,
		"idInherited": true, "DEFAULT_DEBTOR_ACCOUNT_ID": true, "identity": false, "isIdle": false, "paid": false, "valid": false} {
		if idLike(name) != want {
			t.Errorf("idLike(%q) = %v", name, !want)
		}
	}
	for in, want := range map[string]string{"items": "item", "categories": "category", "status": "status", "addresses": "address", "class": "class", "children": "child"} {
		if got := singular(in); got != want {
			t.Errorf("singular(%q) = %q, want %q", in, got, want)
		}
	}
}
