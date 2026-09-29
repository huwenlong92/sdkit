//go:build sdkit_storage_oss

package oss

import (
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"

	alioss "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func init() {
	Register()
}

func Register() {
	storage.RegisterDriver("oss", func(cfg core.Config) (core.Handler, error) {
		return NewFromConfig(cfg)
	})
}

type Driver struct {
	cfg    Config
	bucket *alioss.Bucket
}

type Config struct {
	Bucket        string
	Endpoint      string
	EndpointInner string
	CDNURL        string
	AccessKeyID   string
	AccessSecret  string
	ChunkSize     int64
}

func New(cfg Config) (*Driver, error) {
	if cfg.ChunkSize < 100<<10 {
		cfg.ChunkSize = 5 << 20
	}
	endpoint := cfg.Endpoint
	if cfg.EndpointInner != "" {
		endpoint = cfg.EndpointInner
	}

	client, err := alioss.New(endpoint, cfg.AccessKeyID, cfg.AccessSecret)
	if err != nil {
		return nil, fmt.Errorf("oss client: %w", err)
	}

	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("oss bucket: %w", err)
	}

	return &Driver{cfg: cfg, bucket: bucket}, nil
}

func NewFromConfig(cfg core.Config) (*Driver, error) {
	policy := cfg.Policy
	return New(Config{
		Bucket:        firstNonEmpty(policy.Bucket, cfg.DriverString("oss", "bucket")),
		Endpoint:      firstNonEmpty(policy.Endpoint, cfg.DriverString("oss", "endpoint")),
		EndpointInner: firstNonEmpty(policy.EndpointInner, cfg.DriverString("oss", "endpoint_inner")),
		CDNURL:        firstNonEmpty(policy.CDNURL, cfg.DriverString("oss", "cdn_url")),
		AccessKeyID:   firstNonEmpty(policy.AccessKey, cfg.DriverString("oss", "access_key_id")),
		AccessSecret:  firstNonEmpty(policy.SecretKey, cfg.DriverString("oss", "access_secret")),
		ChunkSize:     cfg.ChunkSize,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (d *Driver) Put(file core.FileHeader) error {
	info := file.Info()
	if info.Size <= d.cfg.ChunkSize {
		if err := d.bucket.PutObject(info.Path, file); err != nil {
			return err
		}
		if info.Progress != nil {
			info.Progress(info.Size, info.Size)
		}
		return nil
	}
	imur, err := d.bucket.InitiateMultipartUpload(info.Path)
	if err != nil {
		return fmt.Errorf("oss initiate multipart upload: %w", err)
	}
	completed := false
	defer func() {
		if !completed {
			_ = d.bucket.AbortMultipartUpload(imur)
		}
	}()
	chunkSize := ossMultipartPartSize(info.Size, d.cfg.ChunkSize)
	parts := make([]alioss.UploadPart, 0, int((info.Size+chunkSize-1)/chunkSize))
	var uploaded int64
	for partNumber, remaining := 1, info.Size; remaining > 0; partNumber++ {
		size := min(chunkSize, remaining)
		part, err := d.bucket.UploadPart(imur, io.LimitReader(file, size), size, partNumber)
		if err != nil {
			return fmt.Errorf("oss upload multipart part %d: %w", partNumber, err)
		}
		parts = append(parts, part)
		remaining -= size
		uploaded += size
		if info.Progress != nil {
			info.Progress(uploaded, info.Size)
		}
	}
	if _, err := d.bucket.CompleteMultipartUpload(imur, parts); err != nil {
		return fmt.Errorf("oss complete multipart upload: %w", err)
	}
	completed = true
	return nil
}

func (d *Driver) ManagesUploadProgress() {}

func ossMultipartPartSize(total int64, configured int64) int64 {
	partSize := max(configured, int64(100<<10))
	const maxParts = int64(10_000)
	if required := (total + maxParts - 1) / maxParts; required > partSize {
		const alignment = int64(1 << 20)
		partSize = ((required + alignment - 1) / alignment) * alignment
	}
	return partSize
}

func (d *Driver) Get(path string) (io.ReadCloser, error) {
	body, err := d.bucket.GetObject(path)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (d *Driver) Delete(paths ...string) error {
	_, err := d.bucket.DeleteObjects(paths, alioss.DeleteObjectsQuiet(true))
	return err
}

func (d *Driver) List(dir string) ([]core.Object, error) {
	prefix := dir
	if !strings.HasSuffix(prefix, "/") && prefix != "" {
		prefix += "/"
	}

	res, err := d.bucket.ListObjects(alioss.Prefix(prefix), alioss.Delimiter("/"))
	if err != nil {
		return nil, err
	}

	var objects []core.Object
	for _, cp := range res.CommonPrefixes {
		objects = append(objects, core.Object{
			Name:  path.Base(cp),
			Path:  cp,
			IsDir: true,
		})
	}
	for _, o := range res.Objects {
		objects = append(objects, core.Object{
			Name: path.Base(o.Key),
			Path: o.Key,
			Size: o.Size,
		})
	}
	return objects, nil
}

func (d *Driver) Source(path string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		if objectURL := core.JoinObjectURL(d.cfg.CDNURL, path); objectURL != "" {
			return objectURL, nil
		}
	}
	ttl = core.NormalizeSourceTTL(ttl)
	urlStr, err := d.bucket.SignURL(path, alioss.HTTPGet, int64(ttl.Seconds()))
	if err != nil {
		return "", err
	}
	// SDK 返回的 URL 可能不包含 Bucket，用 endpoint 重建完整 URL
	if !strings.HasPrefix(urlStr, "http") {
		return fmt.Sprintf("https://%s.%s/%s", d.cfg.Bucket, d.cfg.Endpoint, path), nil
	}
	return urlStr, nil
}

func (d *Driver) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) {
	urlStr, err := d.bucket.SignURL(info.Path, alioss.HTTPPut, int64(ttl.Seconds()))
	if err != nil {
		return nil, err
	}
	return &core.UploadCredential{
		Mode:       core.UploadModeDirectPut,
		Gateway:    "oss",
		Path:       info.Path,
		ChunkNum:   1,
		UploadURLs: []string{urlStr},
	}, nil
}
