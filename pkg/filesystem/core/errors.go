package core

import "errors"

var (
	ErrFileTooBig    = errors.New("文件过大")
	ErrFileExists    = errors.New("文件已存在")
	ErrFileNotFound  = errors.New("文件不存在")
	ErrNameInvalid   = errors.New("文件名不合法")
	ErrUnknownDriver = errors.New("未知存储驱动")
)
