package storage

import (
	"context"
	"io"
	"net/http"

	pkgfs "github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

const (
	OperationUpload   = pkgfs.OperationUpload
	OperationDownload = pkgfs.OperationDownload
	OperationGet      = pkgfs.OperationGet
	OperationDelete   = pkgfs.OperationDelete
	OperationList     = pkgfs.OperationList
	OperationSource   = pkgfs.OperationSource
	OperationToken    = pkgfs.OperationToken

	UploadModeLocalChunk   = core.UploadModeLocalChunk
	UploadModeDirectPut    = core.UploadModeDirectPut
	UploadModeMultipartPut = core.UploadModeMultipartPut

	HookBeforeUpload        = pkgfs.HookBeforeUpload
	HookAfterUpload         = pkgfs.HookAfterUpload
	HookAfterUploadFailed   = pkgfs.HookAfterUploadFailed
	HookBeforeDownload      = pkgfs.HookBeforeDownload
	HookAfterDownload       = pkgfs.HookAfterDownload
	HookAfterDownloadFailed = pkgfs.HookAfterDownloadFailed
	HookBeforeGet           = pkgfs.HookBeforeGet
	HookAfterGet            = pkgfs.HookAfterGet
	HookAfterGetFailed      = pkgfs.HookAfterGetFailed
	HookBeforeDelete        = pkgfs.HookBeforeDelete
	HookAfterDelete         = pkgfs.HookAfterDelete
	HookAfterDeleteFailed   = pkgfs.HookAfterDeleteFailed
	HookBeforeList          = pkgfs.HookBeforeList
	HookAfterList           = pkgfs.HookAfterList
	HookAfterListFailed     = pkgfs.HookAfterListFailed
	HookBeforeSource        = pkgfs.HookBeforeSource
	HookAfterSource         = pkgfs.HookAfterSource
	HookAfterSourceFailed   = pkgfs.HookAfterSourceFailed
	HookBeforeToken         = pkgfs.HookBeforeToken
	HookAfterToken          = pkgfs.HookAfterToken
	HookAfterTokenFailed    = pkgfs.HookAfterTokenFailed
)

var (
	ErrFileTooBig             = core.ErrFileTooBig
	ErrFileExists             = core.ErrFileExists
	ErrFileNotFound           = core.ErrFileNotFound
	ErrNameInvalid            = core.ErrNameInvalid
	ErrUnknownDriver          = core.ErrUnknownDriver
	ErrSourceExpired          = core.ErrSourceExpired
	ErrSourceSignatureInvalid = core.ErrSourceSignatureInvalid
	ErrSourceSecretRequired   = core.ErrSourceSecretRequired
)

type (
	FileHeader       = core.FileHeader
	FileInfo         = core.FileInfo
	FileStream       = core.FileStream
	Handler          = core.Handler
	Object           = core.Object
	UploadCredential = core.UploadCredential

	Event             = pkgfs.Event
	Hook              = pkgfs.Hook
	HookOption        = pkgfs.HookOption
	UploadResult      = pkgfs.UploadResult
	GetResult         = pkgfs.GetResult
	DownloadResult    = pkgfs.DownloadResult
	DeleteResult      = pkgfs.DeleteResult
	ListResult        = pkgfs.ListResult
	SourceResult      = pkgfs.SourceResult
	TokenResult       = pkgfs.TokenResult
	UploadInitRequest = pkgfs.UploadInitRequest
	UploadChunkResult = pkgfs.UploadChunkResult
	UploadStatus      = pkgfs.UploadStatus
	UploadSession     = pkgfs.UploadSession
	ImageInfo         = pkgfs.ImageInfo
	ImageLimit        = pkgfs.ImageLimit
	CropRect          = pkgfs.CropRect
)

func NewFileStream(reader io.Reader, info FileInfo) *FileStream {
	return core.NewFileStream(reader, info)
}

func SourceHandler(fs *FileSystem, secret string) http.Handler {
	return pkgfs.SourceHandler(fs, secret)
}

func BeforeUpload(hook Hook) HookOption {
	return pkgfs.BeforeUpload(hook)
}

func AfterUpload(hook Hook) HookOption {
	return pkgfs.AfterUpload(hook)
}

func AfterUploadFailed(hook Hook) HookOption {
	return pkgfs.AfterUploadFailed(hook)
}

func BeforeDownload(hook Hook) HookOption {
	return pkgfs.BeforeDownload(hook)
}

func AfterDownload(hook Hook) HookOption {
	return pkgfs.AfterDownload(hook)
}

func AfterDownloadFailed(hook Hook) HookOption {
	return pkgfs.AfterDownloadFailed(hook)
}

func BeforeGet(hook Hook) HookOption {
	return pkgfs.BeforeGet(hook)
}

func AfterGet(hook Hook) HookOption {
	return pkgfs.AfterGet(hook)
}

func AfterGetFailed(hook Hook) HookOption {
	return pkgfs.AfterGetFailed(hook)
}

func BeforeDelete(hook Hook) HookOption {
	return pkgfs.BeforeDelete(hook)
}

func AfterDelete(hook Hook) HookOption {
	return pkgfs.AfterDelete(hook)
}

func AfterDeleteFailed(hook Hook) HookOption {
	return pkgfs.AfterDeleteFailed(hook)
}

func BeforeList(hook Hook) HookOption {
	return pkgfs.BeforeList(hook)
}

func AfterList(hook Hook) HookOption {
	return pkgfs.AfterList(hook)
}

func AfterListFailed(hook Hook) HookOption {
	return pkgfs.AfterListFailed(hook)
}

func BeforeSource(hook Hook) HookOption {
	return pkgfs.BeforeSource(hook)
}

func AfterSource(hook Hook) HookOption {
	return pkgfs.AfterSource(hook)
}

func AfterSourceFailed(hook Hook) HookOption {
	return pkgfs.AfterSourceFailed(hook)
}

func BeforeToken(hook Hook) HookOption {
	return pkgfs.BeforeToken(hook)
}

func AfterToken(hook Hook) HookOption {
	return pkgfs.AfterToken(hook)
}

func AfterTokenFailed(hook Hook) HookOption {
	return pkgfs.AfterTokenFailed(hook)
}

func HookMetadata(key string, value any) HookOption {
	return pkgfs.HookMetadata(key, value)
}

func HookValidateFile(ctx context.Context, event Event) error {
	return pkgfs.HookValidateFile(ctx, event)
}

func DecodeImageInfo(reader io.Reader) (ImageInfo, error) {
	return pkgfs.DecodeImageInfo(reader)
}

func ValidateImageInfo(info ImageInfo, limit ImageLimit) error {
	return pkgfs.ValidateImageInfo(info, limit)
}

func CropImage(reader io.Reader, writer io.Writer, rect CropRect, format string, quality int) error {
	return pkgfs.CropImage(reader, writer, rect, format, quality)
}
