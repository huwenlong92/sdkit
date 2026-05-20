package core

import (
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Namer 文件名生成器，支持模板变量
// {uuid} / {randomkey16} / {randomkey8} / {timestamp} / {datetime}
// {date} / {year} / {month} / {day} / {hour} / {minute} / {second}
// {originname} / {ext}
type Namer struct {
	DirRule  string // 目录规则
	FileRule string // 文件名规则
}

var DefaultNamer = &Namer{
	DirRule:  "{date}",
	FileRule: "{uuid}{ext}",
}

// Generate 根据原始文件名生成存储路径
func (n *Namer) Generate(originName string) string {
	ext := filepath.Ext(originName)
	now := time.Now()

	repl := map[string]string{
		"{randomkey16}": randStr(16),
		"{randomkey8}":  randStr(8),
		"{timestamp}":   strconv.FormatInt(now.Unix(), 10),
		"{uuid}":        strings.ReplaceAll(uuid.New().String(), "-", ""),
		"{datetime}":    now.Format("20060102150405"),
		"{date}":        now.Format("20060102"),
		"{year}":        now.Format("2006"),
		"{month}":       now.Format("01"),
		"{day}":         now.Format("02"),
		"{hour}":        now.Format("15"),
		"{minute}":      now.Format("04"),
		"{second}":      now.Format("05"),
		"{originname}":  strings.TrimSuffix(originName, ext),
		"{ext}":         ext,
	}

	dir := n.DirRule
	name := n.FileRule
	for k, v := range repl {
		dir = strings.ReplaceAll(dir, k, v)
		name = strings.ReplaceAll(name, k, v)
	}
	return filepath.Join(dir, name)
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randStr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
