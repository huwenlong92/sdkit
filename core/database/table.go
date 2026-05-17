package database

import (
	"reflect"

	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

// TableName 获取模型的完整表名（含 GORM TablePrefix，不带 SQL 引号）。
func TableName(model interface{}) string {
	if Default == nil {
		return ""
	}
	return Default.TableName(model)
}

// Table 获取模型的 SQL 安全表名，可直接拼接到原生 SQL 的标识符位置。
func Table(model interface{}) string {
	if Default == nil {
		return ""
	}
	return Default.Table(model)
}

func (db *Database) TableName(model any) string {
	if db == nil || db.Gorm == nil {
		return ""
	}
	stmt := &gorm.Statement{DB: db.Gorm}
	if err := stmt.Parse(model); err != nil {
		return ""
	}
	return stmt.Schema.Table
}

func (db *Database) Table(model any) string {
	name := db.TableName(model)
	if name == "" {
		return ""
	}
	if db != nil && db.Config.Schema != "" {
		return pgx.Identifier{db.Config.Schema, name}.Sanitize()
	}
	return pgx.Identifier{name}.Sanitize()
}

func TableOf[T any](db *Database) string {
	if db == nil {
		return ""
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t != nil && t.Kind() == reflect.Pointer {
		return db.Table(reflect.New(t.Elem()).Interface())
	}
	return db.Table(new(T))
}
