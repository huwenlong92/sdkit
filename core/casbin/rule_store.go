package casbin

import (
	"context"
	"strings"

	"github.com/huwenlong92/sdkit/core/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Rule struct {
	PType string `gorm:"column:ptype"`
	V0    string `gorm:"column:v0"`
	V1    string `gorm:"column:v1"`
	V2    string `gorm:"column:v2"`
	V3    string `gorm:"column:v3"`
	V4    string `gorm:"column:v4"`
	V5    string `gorm:"column:v5"`
}

func (Rule) TableName() string {
	return DefaultRuleTable
}

type RuleFilter struct {
	PType    string
	V0       string
	V0In     []string
	V1       string
	V1Prefix string
	V2       string
	V3       string
	V4       string
	V5       string
}

func EnsurePolicyTable(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return db.WithContext(ctx).Exec(ruleTableDDL(DefaultRuleTable)).Error
}

func ListPolicyRules(ctx context.Context, db *gorm.DB, filter RuleFilter) ([]Rule, error) {
	rules := make([]Rule, 0)
	if db == nil {
		return rules, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := EnsurePolicyTable(ctx, db); err != nil {
		return nil, err
	}
	err := applyRuleFilter(db.WithContext(ctx).Model(&Rule{}), filter).
		Select("ptype", "v0", "v1", "v2", "v3", "v4", "v5").
		Find(&rules).Error
	return rules, err
}

func PolicyRuleCountsByV0(ctx context.Context, db *gorm.DB, filter RuleFilter, keyFunc func(Rule) string) (map[string]int64, error) {
	counts := make(map[string]int64)
	rules, err := ListPolicyRules(ctx, db, filter)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]map[string]struct{})
	for _, rule := range rules {
		key := ruleKey(rule)
		if keyFunc != nil {
			key = keyFunc(rule)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[rule.V0]; !ok {
			seen[rule.V0] = make(map[string]struct{})
		}
		if _, ok := seen[rule.V0][key]; ok {
			continue
		}
		seen[rule.V0][key] = struct{}{}
		counts[rule.V0]++
	}
	return counts, nil
}

func InsertPolicyRules(ctx context.Context, db *gorm.DB, rules []Rule) error {
	if len(rules) == 0 || db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := EnsurePolicyTable(ctx, db); err != nil {
		return err
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(rules, 100).Error
}

func DeletePolicyRules(ctx context.Context, db *gorm.DB, filter RuleFilter) error {
	if db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := EnsurePolicyTable(ctx, db); err != nil {
		return err
	}
	return applyRuleFilter(db.WithContext(ctx).Model(&Rule{}), filter).Delete(&Rule{}).Error
}

func ReplacePolicyRules(ctx context.Context, db *gorm.DB, deleteFilter RuleFilter, rules []Rule) error {
	if db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := EnsurePolicyTable(ctx, db); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := applyRuleFilter(tx.Model(&Rule{}), deleteFilter).Delete(&Rule{}).Error; err != nil {
			return err
		}
		if len(rules) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rules, 100).Error
	})
}

func DeletePolicyTuples(ctx context.Context, db *gorm.DB, rules []Rule) error {
	if len(rules) == 0 || db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := EnsurePolicyTable(ctx, db); err != nil {
		return err
	}

	values := make([]string, 0, len(rules))
	args := make([]any, 0, 1+len(rules)*2)
	args = append(args, "p")
	for _, rule := range rules {
		if rule.V1 == "" || rule.V2 == "" {
			continue
		}
		values = append(values, "(?, ?)")
		args = append(args, rule.V1, rule.V2)
	}
	if len(values) == 0 {
		return nil
	}

	sql := "DELETE FROM " + quoteTable(DefaultRuleTable) + " WHERE ptype = ? AND (v1, v2) IN (" + strings.Join(values, ",") + ")"
	return db.WithContext(ctx).Exec(sql, args...).Error
}

func ensureRuleTable(ctx context.Context, db *database.Database, table string) error {
	if db == nil || db.PGX == nil {
		return nil
	}
	_, err := db.PGX.Exec(ctx, ruleTableDDL(table))
	return err
}

func ruleTableDDL(table string) string {
	if strings.TrimSpace(table) == "" {
		table = DefaultRuleTable
	}
	return `CREATE TABLE IF NOT EXISTS ` + quoteTable(table) + ` (
		id SERIAL PRIMARY KEY,
		ptype VARCHAR(100) NOT NULL DEFAULT '',
		v0 VARCHAR(100) NOT NULL DEFAULT '',
		v1 VARCHAR(100) NOT NULL DEFAULT '',
		v2 VARCHAR(100) NOT NULL DEFAULT '',
		v3 VARCHAR(100) NOT NULL DEFAULT '',
		v4 VARCHAR(100) NOT NULL DEFAULT '',
		v5 VARCHAR(100) NOT NULL DEFAULT '',
		UNIQUE (ptype, v0, v1, v2, v3, v4, v5)
	)`
}

func applyRuleFilter(db *gorm.DB, filter RuleFilter) *gorm.DB {
	if filter.PType != "" {
		db = db.Where("ptype = ?", filter.PType)
	}
	if filter.V0 != "" {
		db = db.Where("v0 = ?", filter.V0)
	}
	if values := compactStrings(filter.V0In); len(values) > 0 {
		db = db.Where("v0 IN ?", values)
	}
	if filter.V1 != "" {
		db = db.Where("v1 = ?", filter.V1)
	}
	if filter.V1Prefix != "" {
		db = db.Where("v1 LIKE ? ESCAPE '\\'", escapeLike(filter.V1Prefix)+"%")
	}
	if filter.V2 != "" {
		db = db.Where("v2 = ?", filter.V2)
	}
	if filter.V3 != "" {
		db = db.Where("v3 = ?", filter.V3)
	}
	if filter.V4 != "" {
		db = db.Where("v4 = ?", filter.V4)
	}
	if filter.V5 != "" {
		db = db.Where("v5 = ?", filter.V5)
	}
	return db
}

func ruleKey(rule Rule) string {
	return strings.Join([]string{rule.V1, rule.V2, rule.V3, rule.V4, rule.V5}, "\x00")
}

func compactStrings(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}
