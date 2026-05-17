package database

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

type Page struct {
	Page     int
	PageSize int
}

func (p Page) Limit() int {
	if p.PageSize <= 0 {
		return defaultPageSize
	}
	if p.PageSize > maxPageSize {
		return maxPageSize
	}
	return p.PageSize
}

func (p Page) Offset() int {
	page := p.Page
	if page <= 0 {
		page = defaultPage
	}
	return (page - 1) * p.Limit()
}
