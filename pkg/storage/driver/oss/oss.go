package oss

import (
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage/core"

	alioss "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type Driver struct {
	cfg    Config
	bucket *alioss.Bucket
}

type Config struct {
	Bucket        string
	Endpoint      string
	EndpointInner string
	PublicURL     string
	CDNURL        string
	AccessKeyID   string
	AccessSecret  string
}

func New(cfg Config) (*Driver, error) {
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
		PublicURL:     firstNonEmpty(policy.PublicURL, cfg.DriverString("oss", "public_url")),
		CDNURL:        firstNonEmpty(policy.CDNURL, cfg.DriverString("oss", "cdn_url")),
		AccessKeyID:   firstNonEmpty(policy.AccessKey, cfg.DriverString("oss", "access_key_id")),
		AccessSecret:  firstNonEmpty(policy.SecretKey, cfg.DriverString("oss", "access_secret")),
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
	return d.bucket.PutObject(info.Path, file)
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
	if publicURL := core.JoinPublicURL(publicBaseURL(d.cfg.PublicURL, d.cfg.CDNURL), path); publicURL != "" {
		return publicURL, nil
	}
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

func publicBaseURL(publicURL, cdnURL string) string {
	if cdnURL != "" {
		return cdnURL
	}
	return publicURL
}
