package httputil

// PagedResponse is a generic paginated response wrapper.
type PagedResponse[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

// NormalizePage clamps page to a minimum of 1 and pageSize to [1, 100]
// (defaulting to 20 when out of range), and returns the resulting offset.
func NormalizePage(page, pageSize int) (normalizedPage, normalizedPageSize, offset int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize, (page - 1) * pageSize
}
