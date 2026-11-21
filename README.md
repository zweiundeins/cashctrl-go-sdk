# cashctrl-go-sdk

A typed Go client for the [CashCtrl](https://cashctrl.com) accounting API,
covering all 376 endpoints of the
[published reference](https://app.cashctrl.com/static/help/en/api/index.html).

The endpoint methods are generated from the specification of the TypeScript
SDK, [cashctrl-ts-sdk](https://github.com/zweiundeins/cashctrl-ts-sdk): its
`spec/api.json` (scraped from the reference) for the requests and its
`spec/responses.json` (probed from a live organisation) for the response
shapes. `spec/SOURCE` names the version this module was generated from. No
dependencies outside the standard library; Go 1.23 or newer.

> Unofficial and not affiliated with CashCtrl.

## Install

```sh
go get github.com/zweiundeins/cashctrl-go-sdk
```

You need an API key: **Settings > Users & Roles > Add > Add API user**. It is
scoped to one organisation and has the rights of the role you give it.

## Usage

Resources follow the API's paths, so `POST /api/v1/order/category/create.json`
is `c.Order().Category().Create(ctx, &cashctrl.OrderCategoryCreateParams{...})`.
Every method takes a `context.Context`.

```go
c := &cashctrl.Client{
	Org:    "myorg", // the subdomain of myorg.cashctrl.com
	APIKey: os.Getenv("CASHCTRL_APIKEY"),
	Lang:   cashctrl.LangDE, // error messages and generated documents
}

page, err := c.Account().List(ctx, &cashctrl.AccountListParams{OnlyActive: cashctrl.Ptr(true)})

res, err := c.Person().Create(ctx, &cashctrl.PersonCreateParams{
	Company:    cashctrl.Ptr("Muster AG"),
	IsCustomer: cashctrl.Ptr(true),
	Addresses: []cashctrl.PersonAddressParam{{
		Type: cashctrl.PersonAddressTypeMain, Zip: cashctrl.Ptr("8001"), City: cashctrl.Ptr("Zürich"),
	}},
})

_, err = c.Order().Create(ctx, &cashctrl.OrderCreateParams{
	AssociateID: []int64{res.InsertID},
	CategoryID:  4,
	Date:        cashctrl.DateOf(time.Now()),
	Items: []cashctrl.OrderItemParam{{AccountID: 42, Name: "Beratung", UnitPrice: 180, Quantity: cashctrl.Ptr(8.0)}},
})
```

### Parameters

Each endpoint has a params struct. Mandatory parameters are plain fields and are
always sent. Optional ones are pointers (`cashctrl.Ptr(v)`) or slices: nil is
not sent, a pointer to `false`, `0` or `""` is. JSON parameters are typed
structs down to their nested fields; CSV parameters are `[]int64` for ids and
`[]string` otherwise; dates are `cashctrl.Date`, sent as `YYYY-MM-DD`;
documented value lists are string types with constants (`cashctrl.OrderTypeSales`).

**Updates replace the whole record.** CashCtrl clears every writable parameter
an `update` call leaves out, so read the record first and send everything you
want to keep.

### Lists

List endpoints return `*cashctrl.List[T]` with `Data` and `Total`. They take
`Filter` (`[]cashctrl.Filter`, all of which must match), `Sort`, `Dir`,
`Query`, and on most lists `Start` and `Limit` (default 100). `cashctrl.All`
pages through a list:

```go
rows := cashctrl.All(ctx, 0, func(ctx context.Context, start, limit int) (*cashctrl.List[cashctrl.Person], error) {
	return c.Person().List(ctx, &cashctrl.PersonListParams{
		Filter: []cashctrl.Filter{cashctrl.Where("nr", cashctrl.Eq, "LIB-1")},
		Start:  &start, Limit: &limit,
	})
})
for p, err := range rows {
	...
}
```

A list covers the API user's current fiscal period unless it takes and gets
`FiscalPeriodID` (`c.Fiscalperiod().List` names the periods).

### Errors and retries

CashCtrl answers a validation error with HTTP 200 and `"success": false`. Every
refusal, that one included, is an `*cashctrl.APIError` with the endpoint, the
HTTP status (200 for a validation error), the message and the field errors:

```go
var apiErr *cashctrl.APIError
if errors.As(err, &apiErr) && apiErr.Validation() {
	for field, msgs := range apiErr.ByField() { ... }
}
```

`apiErr.Temporary()` is true for 429 and 5xx. An answer that does not fit the
generated type is a `*cashctrl.DecodeError`, which carries the raw answer.

A request answered 429 is sent again, up to `Client.MaxRetries` times (3 by
default, a negative value disables it), after `Retry-After` or a backoff from
`RetryBaseDelay` (1 s) doubling up to `MaxRetryDelay` (30 s). GET requests are
also repeated after 408, 502, 503, 504 and network errors. A POST is never
repeated after a 5xx, since the write may have happened.

### Documents

Endpoints that answer a file (`.pdf`, `.xlsx`, `.csv`, `.zip`, `.vcf`, `.xml`,
`.html`, and `file/get`, `domain/current/logo`, `order/payment/download`,
`salary/payment/download`) return `*cashctrl.Download` with the content type,
the file name and the bytes. A refusal sent as JSON is an `*APIError`.
`cashctrl.WithLang(ctx, cashctrl.LangFR)` sets the language for one call:

```go
pdf, err := c.Order().Document().ReadPDF(cashctrl.WithLang(ctx, cashctrl.LangFR),
	&cashctrl.OrderDocumentReadPDFParams{IDs: []int64{412}})
```

### Tests and proxies

`Client.BaseURL` replaces `https://<org>.cashctrl.com/api/v1/`, and
`Client.HTTPClient` the HTTP client. `Client.Do` sends a request to an
endpoint the specification does not have.

## Layout

```
client.go, errors.go, params.go, list.go   hand-written: transport, errors, parameter types
*_gen.go                                   generated, one file per endpoint group (19)
endpoints_gen_test.go                      generated: one call of every method, for the tests
cmd/gen, internal/gen                      the generator
spec/api.json, spec/responses.json         copied from cashctrl-ts-sdk, never edited here
spec/SOURCE                                the tag, commit and hashes they come from
scripts/sync-spec.sh                       pulls a new spec and regenerates
```

Do not edit `*_gen.go`. Change the generator and run `go generate ./...`;
`TestGeneratedIsCurrent` and CI fail when the committed files differ from what
the generator writes.

## Staying in sync with the TypeScript SDK

The TypeScript SDK's weekly workflow (`.github/workflows/upstream.yml`,
Mondays) re-scrapes the reference and opens a pull request when the spec moved.
CashCtrl also announces API changes by email to its API users, and such a
change does not always reach the reference: on an announcement, re-probe the
response shapes in the TypeScript SDK (`deno task probe`, see its
CONTRIBUTING.md), release it, and sync. Once a spec change is merged and
released, sync this module to the release:

```sh
scripts/sync-spec.sh v0.6.0                          # from GitHub
scripts/sync-spec.sh v0.6.0 ../cashctrl-ts-sdk       # from a local clone
```

The script copies both spec files from that tag or commit, rewrites
`spec/SOURCE`, regenerates, and runs gofmt, vet and the tests. Review the
`spec/*.json` diff, which is the API change, then commit the spec and the
generated files together. When the sync moves the TypeScript SDK's
`scripts/overrides.ts`, the script says so: port the change to
`internal/gen/overrides.go` before committing.

## How the generator reads the spec

It follows the TypeScript SDK's `scripts/generate.ts`, so both SDKs agree on
envelopes and types:

- The corrections of `overrides.ts` are applied first: the undocumented `type`
  of the two `customfield` reorder endpoints, `setting/update.json` taking
  free keys (a `map[string]string` here), structured parameters labelled TEXT,
  and arrays the reference does not call arrays. Two more are this module's
  own: "either firstName, lastName or company" (and "either id or type") are
  optional fields here, where the reference marks all of them mandatory; and a
  tree endpoint answers its own node type, since tree nodes carry `cls`,
  `leaf` and `data` that the list entity lacks.
- A resource's record type merges the probed `read.json` and `list.json`
  shapes. A field that was null in every sample takes its type from the
  documented create or update parameter of the same name.
- `list.json` and `tree.json` unwrap `data` into a `List`, `read.json`
  unwraps `data` into the record, other GETs follow the probe. An endpoint the
  probe has no shape for answers `json.RawMessage` (27 of the JSON endpoints).
- Numbers are `int64` for ids, `int` for counts (`limit`, `start`, `dueDays`,
  digits) and `float64` otherwise.

## Live test

`TestLive` reads from a disposable test organisation and never writes. It
takes the same variables as cashctrl-ts-sdk's write harness, refuses an
organisation or key that matches `CASHCTRL_DOMAINID`, `CASHCTRL_ORG`,
`CASHCTRL_APIKEY` or `CASHCTRL_API_KEY`, and wants the organisation's name
repeated:

```sh
CASHCTRL_TEST_DOMAINID=mytestorg CASHCTRL_TEST_APIKEY=... \
CASHCTRL_LIVE_TEST=mytestorg go test -run TestLive -v .
```

Use an API user with a read-only role.

## License

MIT, see [LICENSE](LICENSE).
