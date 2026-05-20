package cos

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage/core"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type Driver struct {
	cfg    Config
	client *cos.Client
}

type Config struct {
	Bucket        string
	Endpoint      string
	EndpointInner string
	PublicURL     string
	CDNURL        string
	SecretID      string
	SecretKey     string
}

func New(cfg Config) (*Driver, error) {
	endpoint := cfg.Endpoint
	if cfg.EndpointInner != "" {
		endpoint = cfg.EndpointInner
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("cos endpoint: %w", err)
	}

	d := &Driver{
		cfg: cfg,
		client: cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
			Transport: &cos.AuthorizationTransport{
				SecretID:  cfg.SecretID,
				SecretKey: cfg.SecretKey,
			},
			Timeout: 10 * time.Minute,
		}),
	}
	return d, nil
}

func NewFromConfig(cfg core.Config) (*Driver, error) {
	policy := cfg.Policy
	return New(Config{
		Bucket:        firstNonEmpty(policy.Bucket, cfg.DriverString("cos", "bucket")),
		Endpoint:      firstNonEmpty(policy.Endpoint, cfg.DriverString("cos", "endpoint")),
		EndpointInner: firstNonEmpty(policy.EndpointInner, cfg.DriverString("cos", "endpoint_inner")),
		PublicURL:     firstNonEmpty(policy.PublicURL, cfg.DriverString("cos", "public_url")),
		CDNURL:        firstNonEmpty(policy.CDNURL, cfg.DriverString("cos", "cdn_url")),
		SecretID:      firstNonEmpty(policy.AccessKey, cfg.DriverString("cos", "secret_id")),
		SecretKey:     firstNonEmpty(policy.SecretKey, cfg.DriverString("cos", "secret_key")),
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

	// 小文件单次上传
	if info.Size <= 25<<20 { // 25MB
		_, err := d.client.Object.Put(context.Background(), info.Path, io.LimitReader(file, info.Size), nil)
		return err
	}

	// 大文件分片上传
	initRes, _, err := d.client.Object.InitiateMultipartUpload(context.Background(), info.Path, nil)
	if err != nil {
		return err
	}
	uploadID := initRes.UploadID

	chunkSize := int64(25 << 20) // 25MB
	chunkNum := int(info.Size / chunkSize)
	if info.Size%chunkSize != 0 {
		chunkNum++
	}

	parts := make([]cos.Object, chunkNum)
	for i := 0; i < chunkNum; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize
		if end > info.Size {
			end = info.Size
		}

		resp, err := d.client.Object.UploadPart(context.Background(), info.Path, uploadID, i+1,
			io.LimitReader(file, end-start), &cos.ObjectUploadPartOptions{
				ContentLength: end - start,
			})
		if err != nil {
			d.client.Object.AbortMultipartUpload(context.Background(), info.Path, uploadID)
			return fmt.Errorf("上传分片 %d 失败: %w", i, err)
		}
		parts[i] = cos.Object{PartNumber: i + 1, ETag: resp.Header.Get("ETag")}
	}

	_, _, err = d.client.Object.CompleteMultipartUpload(context.Background(), info.Path, uploadID, &cos.CompleteMultipartUploadOptions{
		Parts: parts,
	})
	return err
}

func (d *Driver) Get(path string) (io.ReadCloser, error) {
	resp, err := d.client.Object.Get(context.Background(), path, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (d *Driver) Delete(paths ...string) error {
	for _, p := range paths {
		_, err := d.client.Object.Delete(context.Background(), p)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) List(dir string) ([]core.Object, error) {
	res, _, err := d.client.Bucket.Get(context.Background(), &cos.BucketGetOptions{
		Prefix:    dir,
		Delimiter: "/",
	})
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
	for _, o := range res.Contents {
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
	base, _ := url.Parse(d.cfg.Endpoint)
	base.Path = path
	return base.String(), nil
}

func (d *Driver) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) {
	chunkSize := int64(25 << 20)

	// 小文件：单次 PUT presigned URL
	if info.Size <= chunkSize || info.Size == 0 {
		rawURL, err := d.client.Object.GetPresignedURL(context.Background(), http.MethodPut, info.Path,
			d.cfg.SecretID, d.cfg.SecretKey, ttl, nil)
		if err != nil {
			return nil, err
		}
		return &core.UploadCredential{
			Mode:       core.UploadModeDirectPut,
			Gateway:    "cos",
			Path:       info.Path,
			ChunkSize:  chunkSize,
			ChunkNum:   1,
			UploadURLs: []string{rawURL.String()},
		}, nil
	}

	// 大文件：MultipartUpload presigned URLs
	initRes, _, err := d.client.Object.InitiateMultipartUpload(context.Background(), info.Path, nil)
	if err != nil {
		return nil, fmt.Errorf("cos 创建分片上传失败: %w", err)
	}

	chunkNum := int(info.Size / chunkSize)
	if info.Size%chunkSize != 0 {
		chunkNum++
	}

	urls := make([]string, chunkNum)
	for i := 0; i < chunkNum; i++ {
		query := url.Values{}
		query.Set("partNumber", strconv.Itoa(i+1))
		query.Set("uploadId", initRes.UploadID)

		rawURL, err := d.client.Object.GetPresignedURL(context.Background(), http.MethodPut, info.Path,
			d.cfg.SecretID, d.cfg.SecretKey, ttl, &cos.PresignedURLOptions{Query: &query})
		if err != nil {
			return nil, err
		}
		urls[i] = rawURL.String()
	}

	completeURL, err := d.client.Object.GetPresignedURL(context.Background(), http.MethodPost, info.Path,
		d.cfg.SecretID, d.cfg.SecretKey, ttl, &cos.PresignedURLOptions{
			Query: &url.Values{"uploadId": []string{initRes.UploadID}},
		})
	if err != nil {
		return nil, err
	}

	return &core.UploadCredential{
		Mode:        core.UploadModeMultipartPut,
		Gateway:     "cos",
		UploadID:    initRes.UploadID,
		Path:        info.Path,
		ChunkSize:   chunkSize,
		ChunkNum:    chunkNum,
		UploadURLs:  urls,
		CompleteURL: completeURL.String(),
	}, nil
}

func publicBaseURL(publicURL, cdnURL string) string {
	if cdnURL != "" {
		return cdnURL
	}
	return publicURL
}
