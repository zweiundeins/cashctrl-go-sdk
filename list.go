package cashctrl

import (
	"context"
	"iter"
)

// All pages through a list endpoint with start and limit, pageSize rows per
// request (100 when 0), and yields every row; it stops at the first error,
// which it yields. fetch makes one request:
//
//	rows := cashctrl.All(ctx, 0, func(ctx context.Context, start, limit int) (*cashctrl.List[cashctrl.Person], error) {
//		return c.Person().List(ctx, &cashctrl.PersonListParams{Start: &start, Limit: &limit})
//	})
//	for p, err := range rows { ... }
func All[T any](ctx context.Context, pageSize int, fetch func(ctx context.Context, start, limit int) (*List[T], error)) iter.Seq2[T, error] {
	if pageSize <= 0 {
		pageSize = 100
	}
	return func(yield func(T, error) bool) {
		var zero T
		for start := 0; ; {
			page, err := fetch(ctx, start, pageSize)
			if err != nil {
				yield(zero, err)
				return
			}
			for _, row := range page.Data {
				if !yield(row, nil) {
					return
				}
			}
			start += len(page.Data)
			if len(page.Data) == 0 || len(page.Data) < pageSize || start >= page.Total {
				return
			}
		}
	}
}
