package baidupan

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/execx"
	"github.com/huwenlong92/sdkit/pkg/remotefs"
)

var (
	lsEntryPattern      = regexp.MustCompile(`^\s*\d+\s+(\S+)\s+(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+(.+?)\s*$`)
	metaHeaderPattern   = regexp.MustCompile(`^\s*\[\d+\]\s+-\s+\[(.+)]\s+-+\s*$`)
	metaFieldPattern    = regexp.MustCompile(`^\s*(类型|目录路径|目录名称|文件路径|文件名称|文件大小|md5[^\s]*.*|app_id|fs_id|创建日期|修改日期|是否含有子目录)\s+(.+?)\s*$`)
	progressPattern     = regexp.MustCompile(`↓\s*([\d.]+\s*[KMGTPE]?i?B)/([\d.]+\s*[KMGTPE]?i?B)\s+([\d.]+\s*[KMGTPE]?i?B)/s`)
	quotaPattern        = regexp.MustCompile(`(?i)总空间\s*[:：]\s*([\d.]+\s*[KMGTPE]?i?B).*?已用空间\s*[:：]\s*([\d.]+\s*[KMGTPE]?i?B)`)
	savePathPattern     = regexp.MustCompile(`下载完成,\s*保存位置[:：]\s*(.+?)\s*$`)
	transferNamePattern = regexp.MustCompile(`保存了(.+?)到当前目录`)
)

type listedEntry struct {
	name  string
	path  string
	isDir bool
}

type metaEntry struct {
	path       string
	name       string
	isDir      bool
	size       int64
	fsID       string
	appID      string
	md5        string
	createdAt  time.Time
	modifiedAt time.Time
}

type providerIdentity struct {
	UID  string
	Name string
}

func parseListOutput(directory string, output string) ([]listedEntry, error) {
	var entries []listedEntry
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", "\n"), "\n") {
		match := lsEntryPattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		name := strings.TrimSpace(match[3])
		isDir := strings.HasSuffix(name, "/")
		name = strings.TrimSuffix(name, "/")
		if name == "" || name == "." || name == ".." || path.Base(name) != name || strings.Contains(name, `\`) || strings.ContainsRune(name, '\x00') {
			return nil, fmt.Errorf("list output contained an unsafe child name")
		}
		if _, err := parseBaiduTime(match[2]); err != nil {
			return nil, fmt.Errorf("list output contained an invalid timestamp")
		}
		childPath := path.Join(directory, name)
		if path.Dir(childPath) != path.Clean(directory) {
			return nil, fmt.Errorf("list output contained a non-child path")
		}
		entries = append(entries, listedEntry{
			name:  name,
			path:  childPath,
			isDir: isDir,
		})
	}
	if len(entries) == 0 && !strings.Contains(output, "文件总数: 0, 目录总数: 0") {
		return nil, fmt.Errorf("list output did not contain recognizable entries")
	}
	return entries, nil
}

func parseMetaOutput(output string) ([]metaEntry, error) {
	var (
		entries []metaEntry
		current *metaEntry
	)
	flush := func() error {
		if current == nil {
			return nil
		}
		if current.path == "" || current.name == "" || current.fsID == "" || current.createdAt.IsZero() || current.modifiedAt.IsZero() {
			return fmt.Errorf("metadata block is incomplete")
		}
		entries = append(entries, *current)
		current = nil
		return nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", "\n"), "\n") {
		if match := metaHeaderPattern.FindStringSubmatch(line); len(match) == 2 {
			if err := flush(); err != nil {
				return nil, err
			}
			current = &metaEntry{path: strings.TrimSpace(match[1])}
			continue
		}
		if current == nil {
			continue
		}
		match := metaFieldPattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		key := strings.TrimSpace(match[1])
		value := strings.TrimSpace(match[2])
		switch {
		case key == "类型":
			current.isDir = value == "目录"
		case key == "目录路径" || key == "文件路径":
			current.path = value
		case key == "目录名称" || key == "文件名称":
			current.name = value
		case key == "文件大小":
			raw := strings.TrimSpace(strings.SplitN(value, ",", 2)[0])
			size, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid metadata file size")
			}
			current.size = size
		case strings.HasPrefix(key, "md5"):
			current.md5 = value
		case key == "app_id":
			current.appID = value
		case key == "fs_id":
			current.fsID = value
		case key == "创建日期":
			parsed, err := parseBaiduTime(value)
			if err != nil {
				return nil, fmt.Errorf("invalid metadata creation time")
			}
			current.createdAt = parsed
		case key == "修改日期":
			parsed, err := parseBaiduTime(value)
			if err != nil {
				return nil, fmt.Errorf("invalid metadata modification time")
			}
			current.modifiedAt = parsed
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("metadata output did not contain a recognizable block")
	}
	return entries, nil
}

func parseBaiduTime(value string) (time.Time, error) {
	location := time.FixedZone("CST", 8*60*60)
	return time.ParseInLocation("2006-01-02 15:04:05", value, location)
}

func parseWho(output string) (providerIdentity, bool) {
	uidPattern := regexp.MustCompile(`uid:\s*(\d+)`)
	namePattern := regexp.MustCompile(`用户名:\s*([^,]*)`)
	uidMatch := uidPattern.FindStringSubmatch(output)
	nameMatch := namePattern.FindStringSubmatch(output)
	if len(uidMatch) != 2 || len(nameMatch) != 2 {
		return providerIdentity{}, false
	}
	uid, _ := strconv.ParseInt(uidMatch[1], 10, 64)
	name := strings.TrimSpace(nameMatch[1])
	if uid <= 0 || name == "" {
		return providerIdentity{}, false
	}
	return providerIdentity{UID: uidMatch[1], Name: name}, true
}

func parseProgressLine(line string) (remotefs.Progress, bool) {
	match := progressPattern.FindStringSubmatch(line)
	if len(match) != 4 {
		return remotefs.Progress{}, false
	}
	transferred, err := parseByteSize(match[1])
	if err != nil {
		return remotefs.Progress{}, false
	}
	total, err := parseByteSize(match[2])
	if err != nil {
		return remotefs.Progress{}, false
	}
	speed, err := parseByteSize(match[3])
	if err != nil {
		return remotefs.Progress{}, false
	}
	return remotefs.Progress{
		Phase:            remotefs.ProgressTransferring,
		TransferredBytes: transferred,
		TotalBytes:       total,
		BytesPerSecond:   float64(speed),
	}, true
}

func parseQuotaOutput(output string) (remotefs.Quota, error) {
	match := quotaPattern.FindStringSubmatch(strings.ReplaceAll(output, "\r", "\n"))
	if len(match) != 3 {
		return remotefs.Quota{}, fmt.Errorf("quota output did not contain recognizable capacity values")
	}
	total, err := parseByteSize(match[1])
	if err != nil {
		return remotefs.Quota{}, fmt.Errorf("invalid total quota: %w", err)
	}
	used, err := parseByteSize(match[2])
	if err != nil {
		return remotefs.Quota{}, fmt.Errorf("invalid used quota: %w", err)
	}
	if total <= 0 || used < 0 || used > total {
		return remotefs.Quota{}, fmt.Errorf("quota output contained inconsistent capacity values")
	}
	return remotefs.Quota{TotalBytes: total, UsedBytes: used}, nil
}

func parseByteSize(value string) (int64, error) {
	value = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	match := regexp.MustCompile(`^([\d.]+)([KMGTPE]?I?B)$`).FindStringSubmatch(value)
	if len(match) != 3 {
		return 0, fmt.Errorf("invalid byte size")
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, err
	}
	powers := map[string]int{"B": 0, "KB": 1, "KIB": 1, "MB": 2, "MIB": 2, "GB": 3, "GIB": 3, "TB": 4, "TIB": 4, "PB": 5, "PIB": 5, "EB": 6, "EIB": 6}
	power, ok := powers[match[2]]
	if !ok {
		return 0, fmt.Errorf("invalid byte unit")
	}
	multiplier := float64(int64(1) << (10 * power))
	return int64(number * multiplier), nil
}

func classifyOutput(operation string, output string) (error, string, bool) {
	normalized := strings.ToLower(strings.Join(strings.Fields(output), " "))
	if normalized == "" {
		return nil, "", false
	}
	checks := []struct {
		markers []string
		err     error
		summary string
	}{
		{[]string{"提取码错误", "访问密码错误"}, ErrSharePasswordInvalid, "share password was rejected"},
		{[]string{"分享已删除", "分享已取消", "页面已过期", "该分享已删除"}, ErrShareExpired, "share has expired or was removed"},
		{[]string{"链接地址或提取码非法", "链接错误没找到文件", "分享信息无效"}, ErrShareInvalid, "share link is invalid"},
		{[]string{"文件重复", "已存在同名文件", "文件已存在"}, remotefs.ErrConflict, "remote entry already exists"},
		{[]string{"超出配额", "配额不足", "容量已满"}, remotefs.ErrQuotaExceeded, "remote quota is exhausted"},
		{[]string{"请求过于频繁", "操作过于频繁", "限流", "rate limit", "http 429"}, remotefs.ErrRateLimited, "provider rate limit was reached"},
		{[]string{"31066", "文件或目录不存在", "文件不存在"}, remotefs.ErrNotFound, "remote path was not found"},
		{[]string{"未登录", "帐号无效", "账号无效", "登录信息有误", "请重新登录", "cookie无效"}, remotefs.ErrUnauthenticated, "account session is not authenticated"},
		{[]string{"permission denied", "没有权限", "无权访问"}, remotefs.ErrPermissionDenied, "provider permission was denied"},
		{[]string{"网络错误", "network error", "timeout", "temporary"}, remotefs.ErrTemporary, "provider request failed temporarily"},
	}
	for _, check := range checks {
		for _, marker := range check.markers {
			if strings.Contains(normalized, strings.ToLower(marker)) {
				return check.err, check.summary, true
			}
		}
	}
	if strings.Contains(normalized, "失败") || strings.Contains(normalized, "遇到错误") || strings.Contains(normalized, "操作错误") {
		return remotefs.ErrTemporary, operation + " returned a provider failure", true
	}
	return nil, "", false
}

func errorForOutput(operation string, output string, secrets ...string) error {
	standard, summary, classified := classifyOutput(operation, output)
	if classified {
		return &remotefs.Error{Operation: operation, Driver: DriverName, Err: standard, Summary: summary}
	}
	return &remotefs.Error{
		Operation: operation,
		Driver:    DriverName,
		Err:       remotefs.ErrTemporary,
		Summary:   "unrecognized provider output: " + redact(output, secrets...),
	}
}

func redact(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	value = regexp.MustCompile(`(?i)(bduss|stoken|cookie)(=|:)[^\s,;]+`).ReplaceAllString(value, "$1$2[REDACTED]")
	return remotefs.SafeSummary(value)
}

func joinStandardError(standard error, err error) error {
	if err == nil {
		return standard
	}
	return errors.Join(standard, err)
}

func safeRunnerError(err error) error {
	var exitErr *execx.ExitError
	if !errors.As(err, &exitErr) {
		return err
	}
	if exitErr.Err != nil {
		return exitErr.Err
	}
	return errors.New("command exited unsuccessfully")
}
