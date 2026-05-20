package database

import "gorm.io/gorm"

func Paginate(page, pageSize int) func(db *gorm.DB) *gorm.DB {
	p := Page{Page: page, PageSize: pageSize}

	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(p.Offset()).Limit(p.Limit())
	}
}
