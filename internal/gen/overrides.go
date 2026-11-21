package gen

import "regexp"

// Corrections to the scraped spec, ported from the TypeScript SDK's
// scripts/overrides.ts, which records the evidence for each. spec/SOURCE keeps
// that file's hash; scripts/sync-spec.sh warns when a sync moves it, and this
// file must then follow.

// typeOverrides replace the Go type of a (nested) parameter. An exact path
// wins over "*".
var typeOverrides = []struct{ path, param, goType, reason string }{
	{"/api/v1/order/create.json", "items.unitId", "any",
		"documented as TEXT, but a foreign key to a unit whose id is a number; the API takes both"},
	{"/api/v1/order/update.json", "items.unitId", "any",
		"same as order/create.json"},
}

func findOverride(path, param string) (string, string, bool) {
	for _, o := range typeOverrides {
		if o.path == path && o.param == param {
			return o.goType, o.reason, true
		}
	}
	for _, o := range typeOverrides {
		if o.path == "*" && o.param == param {
			return o.goType, o.reason, true
		}
	}
	return "", "", false
}

var customFieldModules = []string{
	"JOURNAL", "ACCOUNT", "INVENTORY_ARTICLE", "INVENTORY_ASSET", "ORDER", "PERSON", "FILE", "SALARY_STATEMENT",
}

// paramAdditions are parameters the API requires but the reference omits.
var paramAdditions = []struct {
	path  string
	param Param
}{
	{"/api/v1/customfield/reorder.json", Param{Name: "type", Type: "TEXT", Required: true, Enum: customFieldModules,
		Description: "The module the custom fields belong to. Undocumented, but the request fails without it."}},
	{"/api/v1/customfield/group/reorder.json", Param{Name: "type", Type: "TEXT", Required: true, Enum: customFieldModules,
		Description: "The module the custom field groups belong to. Undocumented, but the request fails without it."}},
}

// openParamEndpoints take caller-chosen keys; their params are a map.
var openParamEndpoints = map[string]string{
	"/api/v1/setting/update.json": "the keys setting/read.json answers (THOUSAND_SEPARATOR, CSV_DELIMITER, ...); the set is per organisation",
}

// arrayCorrections are JSON params that are arrays although their
// description omits the wording the scraper keys on.
var arrayCorrections = []struct{ path, param string }{
	{"/api/v1/tax/create.json", "components"},
	{"/api/v1/tax/update.json", "components"},
	{"/api/v1/tax/create.json", "rates"},
	{"/api/v1/tax/update.json", "rates"},
	{"/api/v1/salary/statement/update_multiple.json", "attachments"},
}

// applySpecCorrections folds the corrections into spec, in the order
// overrides.ts applies them.
func applySpecCorrections(spec *Spec) {
	for i := range spec.Endpoints {
		e := &spec.Endpoints[i]
		known := map[string]bool{}
		for _, p := range e.Params {
			known[p.Name] = true
		}
		for _, a := range paramAdditions {
			if a.path == e.Path && !known[a.param.Name] {
				e.Params = append(e.Params, a.param)
			}
		}
	}
	// A nested parameter table is the statement that a value is structured,
	// whatever type label the reference gives it.
	for i := range spec.Endpoints {
		for j := range spec.Endpoints[i].Params {
			p := &spec.Endpoints[i].Params[j]
			if len(p.Fields) > 0 && p.Type != "JSON" {
				p.Type = "JSON"
			}
		}
	}
	// "Either firstName, lastName or company must be set" is documented as
	// three mandatory parameters; one of them is. The Go SDK's own
	// correction, not in overrides.ts: a value field would send the other two
	// as empty strings.
	for i := range spec.Endpoints {
		for j := range spec.Endpoints[i].Params {
			p := &spec.Endpoints[i].Params[j]
			if p.Required && eitherOr.MatchString(p.Description) {
				p.Required = false
				p.Description += " (One of these is mandatory.)"
			}
		}
	}
	for _, c := range arrayCorrections {
		for i := range spec.Endpoints {
			if spec.Endpoints[i].Path != c.path {
				continue
			}
			for j := range spec.Endpoints[i].Params {
				if spec.Endpoints[i].Params[j].Name == c.param {
					spec.Endpoints[i].Params[j].IsArray = true
				}
			}
		}
	}
}

// binarySuffixes and binaryPaths name the endpoints that answer a file.
var binarySuffixes = map[string]bool{"pdf": true, "xlsx": true, "csv": true, "zip": true, "vcf": true, "xml": true, "html": true}

var binaryPaths = map[string]bool{
	"/api/v1/file/get":                true, // redirects to object storage
	"/api/v1/domain/current/logo":     true,
	"/api/v1/order/payment/download":  true, // pain.001 XML or PDF
	"/api/v1/salary/payment/download": true,
}

var eitherOr = regexp.MustCompile(`\bEither [\w ,]+ or \w+ must be set`)
