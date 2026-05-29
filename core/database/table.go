package database

import (
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

// TableName 获取模型表名（含 GORM TablePrefix，不含 schema，不带 SQL 引号）。
func TableName(model interface{}) string {
	if Default == nil {
		return ""
	}
	return Default.TableName(model)
}

// SchemaName 获取模型所在 schema。模型未声明 schema 时返回配置中的默认 schema。
func SchemaName(model interface{}) string {
	if Default == nil {
		return ""
	}
	return Default.SchemaName(model)
}

// TablePath 获取模型完整表名（含 schema，不带 SQL 引号）。
func TablePath(model interface{}) string {
	if Default == nil {
		return ""
	}
	return Default.TablePath(model)
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
	_, table := splitTablePath(stmt.Schema.Table)
	return table
}

func (db *Database) SchemaName(model any) string {
	if db == nil || db.Gorm == nil {
		return ""
	}
	stmt := &gorm.Statement{DB: db.Gorm}
	if err := stmt.Parse(model); err != nil {
		return ""
	}
	schemaName, _ := splitTablePath(stmt.Schema.Table)
	if schemaName != "" {
		return schemaName
	}
	return db.Config.Schema
}

func (db *Database) TablePath(model any) string {
	if db == nil || db.Gorm == nil {
		return ""
	}
	stmt := &gorm.Statement{DB: db.Gorm}
	if err := stmt.Parse(model); err != nil {
		return ""
	}
	schemaName, table := splitTablePath(stmt.Schema.Table)
	if table == "" {
		return ""
	}
	if schemaName == "" {
		schemaName = db.Config.Schema
	}
	return joinTablePath(schemaName, table)
}

func (db *Database) Table(model any) string {
	path := db.TablePath(model)
	if path == "" {
		return ""
	}
	return QuoteTablePath(path)
}

func TablePathWithName(model any, table string) string {
	if Default == nil {
		return table
	}
	return Default.TablePathWithName(model, table)
}

func TableWithName(model any, table string) string {
	if Default == nil {
		return QuoteTablePath(table)
	}
	return Default.TableWithName(model, table)
}

func (db *Database) TablePathWithName(model any, table string) string {
	table = strings.TrimSpace(table)
	if table == "" {
		return ""
	}
	if schemaName, name := splitTablePath(table); schemaName != "" {
		return joinTablePath(schemaName, name)
	}
	if strings.Contains(table, ".") {
		return table
	}
	schemaName := db.SchemaName(model)
	return joinTablePath(schemaName, table)
}

func (db *Database) TableWithName(model any, table string) string {
	return QuoteTablePath(db.TablePathWithName(model, table))
}

func QuoteTablePath(path string) string {
	schemaName, table := splitTablePath(path)
	if table == "" {
		return ""
	}
	if schemaName != "" {
		return pgx.Identifier{schemaName, table}.Sanitize()
	}
	return pgx.Identifier{table}.Sanitize()
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

func splitTablePath(path string) (string, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ""
	}
	parts := strings.Split(path, ".")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", path
}

func joinTablePath(schemaName string, table string) string {
	schemaName = strings.TrimSpace(schemaName)
	table = strings.TrimSpace(table)
	if table == "" {
		return ""
	}
	if schemaName == "" {
		return table
	}
	return schemaName + "." + table
}
