package baidupan

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/remotefs"
)

type ParsedShare struct {
	URL      string
	Password string
	Feature  string
}

func (f *FileSystem) StageShare(ctx context.Context, req remotefs.StageRequest) (remotefs.StageResult, error) {
	if err := f.ready(ctx); err != nil {
		return remotefs.StageResult{}, err
	}
	details, err := ParseShare(req.Share)
	if err != nil {
		return remotefs.StageResult{}, err
	}
	destination, err := normalizeRemotePath(req.Destination.Path, true)
	if err != nil {
		return remotefs.StageResult{}, err
	}
	if err := f.ensureAuthenticated(ctx); err != nil {
		return remotefs.StageResult{}, err
	}
	unlock, err := f.lockSession(ctx)
	if err != nil {
		return remotefs.StageResult{}, err
	}
	defer unlock()
	mkdirOutput, err := f.outputLocked(ctx, []string{"mkdir", destination})
	if err != nil {
		return remotefs.StageResult{}, err
	}
	if standard, summary, failed := classifyOutput("stage share", mkdirOutput); failed && !errors.Is(standard, remotefs.ErrConflict) {
		return remotefs.StageResult{}, &remotefs.Error{Operation: "stage share", Driver: DriverName, Err: standard, Summary: summary}
	}
	cdOutput, err := f.outputLocked(ctx, []string{"cd", destination})
	if err != nil {
		return remotefs.StageResult{}, err
	}
	if standard, summary, failed := classifyOutput("stage share", cdOutput); failed {
		return remotefs.StageResult{}, &remotefs.Error{Operation: "stage share", Driver: DriverName, Err: standard, Summary: summary}
	}
	args := []string{"transfer", details.URL}
	transferInput := "cd " + destination + "\ntransfer " + details.URL
	if details.Password != "" {
		if !safeInteractiveValue(details.Password) {
			return remotefs.StageResult{}, &remotefs.Error{Operation: "stage share", Driver: DriverName, Err: ErrSharePasswordInvalid, Summary: "share password contains unsupported characters"}
		}
		transferInput += " " + details.Password
	}
	transferInput += "\nquit\n"
	output, err := f.outputWithCommandLocked(ctx, Command{
		Name: f.runtime.cfg.BinaryPath, Args: args, Env: f.commandEnv(), Input: transferInput,
		Interactive: true, OutputLimit: f.runtime.cfg.OutputLimit, MergeStderr: true,
	})
	if err != nil {
		return remotefs.StageResult{}, err
	}
	baseResult := remotefs.StageResult{Reference: remotefs.Reference{Path: destination}}
	if standard, summary, failed := classifyOutput("stage share", output); failed {
		if standard == remotefs.ErrConflict {
			baseResult.Duplicate = true
		}
		return baseResult, &remotefs.Error{Operation: "stage share", Driver: DriverName, Err: standard, Summary: summary}
	}
	match := transferNamePattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return baseResult, errorForOutput("stage share", output, f.cfg.BDUSS, f.cfg.STOKEN, details.Password)
	}
	name := strings.TrimSpace(match[1])
	if name == "" || name == "." || name == ".." || path.Base(name) != name || strings.Contains(name, `\`) || strings.ContainsRune(name, '\x00') {
		return baseResult, &remotefs.Error{Operation: "stage share", Driver: DriverName, Err: remotefs.ErrPermissionDenied, Summary: "provider result name is unsafe"}
	}
	baseResult.Name = name
	baseResult.Reference.Path = path.Join(destination, name)
	return baseResult, nil
}

func ParseShare(req remotefs.ShareRequest) (ParsedShare, error) {
	raw := strings.TrimSpace(req.URL)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return ParsedShare{}, &remotefs.Error{Operation: "parse share", Driver: DriverName, Err: ErrShareInvalid, Summary: "share URL is invalid"}
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "pan.baidu.com" {
		return ParsedShare{}, &remotefs.Error{Operation: "parse share", Driver: DriverName, Err: ErrShareInvalid, Summary: "share URL host is unsupported"}
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) != 2 || segments[0] != "s" {
		return ParsedShare{}, &remotefs.Error{Operation: "parse share", Driver: DriverName, Err: ErrShareInvalid, Summary: "share URL path is invalid"}
	}
	feature := segments[1]
	if !validShareFeature(feature) {
		return ParsedShare{}, &remotefs.Error{Operation: "parse share", Driver: DriverName, Err: ErrShareInvalid, Summary: "share feature is invalid"}
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		password = strings.TrimSpace(parsed.Query().Get("pwd"))
	}
	if password != "" && len(password) != 4 {
		return ParsedShare{}, &remotefs.Error{Operation: "parse share", Driver: DriverName, Err: ErrSharePasswordInvalid, Summary: "share password must contain four characters"}
	}
	query := parsed.Query()
	query.Del("pwd")
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return ParsedShare{URL: parsed.String(), Password: password, Feature: feature}, nil
}

func validShareFeature(feature string) bool {
	if !strings.HasPrefix(feature, "1") || len(feature) < 8 || len(feature) > 23 {
		return false
	}
	for _, character := range feature {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
