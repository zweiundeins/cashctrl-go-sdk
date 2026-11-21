package gen

import (
	"regexp"
	"strings"
	"unicode"
)

var initialisms = map[string]string{
	"id": "ID", "ids": "IDs", "url": "URL", "uid": "UID", "iban": "IBAN", "bic": "BIC",
	"pdf": "PDF", "csv": "CSV", "xml": "XML", "html": "HTML", "json": "JSON", "xlsx": "XLSX", "vcf": "VCF",
}

var formats = map[string]bool{"json": true, "csv": true, "pdf": true, "xlsx": true, "zip": true, "vcf": true, "xml": true, "html": true}

// words splits camelCase, snake_case, kebab-case and ALL_CAPS into lower-case
// words.
func words(s string) []string {
	var out []string
	for _, part := range regexp.MustCompile(`[^A-Za-z0-9]+`).Split(s, -1) {
		if part == "" {
			continue
		}
		if strings.ToUpper(part) == part {
			out = append(out, strings.ToLower(part))
			continue
		}
		start := 0
		r := []rune(part)
		for i := 1; i < len(r); i++ {
			if unicode.IsUpper(r[i]) && !unicode.IsUpper(r[i-1]) {
				out = append(out, strings.ToLower(string(r[start:i])))
				start = i
			}
		}
		out = append(out, strings.ToLower(string(r[start:])))
	}
	return out
}

// goName is an exported Go identifier for s: associateId is AssociateID,
// DEFAULT_DEBTOR_ACCOUNT_ID is DefaultDebtorAccountID.
func goName(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		if up, ok := initialisms[w]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	name := b.String()
	if name == "" || unicode.IsDigit(rune(name[0])) {
		name = "X" + name
	}
	return name
}

// plainName pascal-cases s without initialisms, for resource segments, so
// typeName stays as the TypeScript SDK spells it (costcenter: Costcenter).
func plainName(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// splitPath splits /api/v1/account/category/list.json into its resource
// segments and its verb.
func splitPath(path string) ([]string, string) {
	segs := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	return segs[:len(segs)-1], segs[len(segs)-1]
}

// typeName is the Go type stem of a resource: account/category is
// AccountCategory.
func typeName(resource []string) string {
	var b strings.Builder
	for _, s := range resource {
		b.WriteString(plainName(s))
	}
	return b.String()
}

// methodName is the Go method of a verb: list.json is List, list.csv ListCSV,
// update_attachments.json UpdateAttachments.
func methodName(verb string) string {
	base, ext := verb, ""
	if i := strings.LastIndex(verb, "."); i >= 0 && formats[verb[i+1:]] {
		base, ext = verb[:i], verb[i+1:]
	}
	name := goName(base)
	if ext != "" && ext != "json" {
		if up, ok := initialisms[ext]; ok {
			name += up
		} else {
			name += strings.ToUpper(ext)
		}
	}
	return name
}

// singular is the TypeScript SDK's naive singular, good enough for CashCtrl's
// field names.
func singular(w string) string {
	if w == "children" {
		return "child"
	}
	switch {
	case strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case regexp.MustCompile(`(ses|xes|zes|ches|shes)$`).MatchString(w):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us"):
		return w[:len(w)-1]
	}
	return w
}

// idLike reports whether a parameter or field holds record ids, which are
// integers.
func idLike(name string) bool {
	return idPrefix.MatchString(name) || idWord.MatchString(name) || strings.HasSuffix(name, "_ID")
}

// comment wraps text as Go comment lines at indent.
func comment(text, indent string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	width := 78 - len(indent)
	var b strings.Builder
	line := ""
	for _, w := range strings.Fields(text) {
		if line != "" && len(line)+1+len(w) > width {
			b.WriteString(indent + "// " + line + "\n")
			line = w
			continue
		}
		if line == "" {
			line = w
		} else {
			line += " " + w
		}
	}
	if line != "" {
		b.WriteString(indent + "// " + line + "\n")
	}
	return b.String()
}

// idWord finds an Id or Ids word inside camelCase: categoryId, fileIds,
// sequenceNrIdInherited.
var (
	idWord   = regexp.MustCompile(`Ids?([A-Z]|$)`)
	idPrefix = regexp.MustCompile(`^ids?([A-Z]|$)`)
)
