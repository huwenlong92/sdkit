package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/huwenlong92/sdkit/pkg/storage"
	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

func init() {
	Register()
}

func Register() {
	storage.RegisterDriver("s3", func(cfg core.Config) (core.Handler, error) {
		return NewFromConfig(cfg, false)
	})
	storage.RegisterDriver("minio", func(cfg core.Config) (core.Handler, error) {
		return NewFromConfig(cfg, true)
	})
	storage.RegisterDriver("r2", func(cfg core.Config) (core.Handler, error) {
		return NewR2FromConfig(cfg)
	})
}

type Driver struct {
	cfg     Config
	svc     *awss3.Client
	presign *awss3.PresignClient
	creds   aws.CredentialsProvider
	path    bool
}

type Config struct {
	Bucket        string
	Endpoint      string
	EndpointInner string
	CDNURL        string
	Region        string
	AccessKey     string
	SecretKey     string
}

func New(cfg Config, minio bool) (*Driver, error) {
	creds := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""))
	awsCfg := aws.Config{
		Region:      cfg.Region,
		Credentials: creds,
	}
	client := awss3.NewFromConfig(awsCfg, func(opts *awss3.Options) {
		if endpoint := s3Endpoint(cfg); endpoint != "" {
			opts.BaseEndpoint = aws.String(endpoint)
		}
		opts.UsePathStyle = minio
	})
	return &Driver{
		cfg:     cfg,
		svc:     client,
		presign: awss3.NewPresignClient(client),
		creds:   creds,
		path:    minio,
	}, nil
}

func NewFromConfig(cfg core.Config, minio bool) (*Driver, error) {
	policy := cfg.Policy
	return New(Config{
		Bucket:        firstNonEmpty(policy.Bucket, cfg.DriverString("s3", "bucket")),
		Endpoint:      firstNonEmpty(policy.Endpoint, cfg.DriverString("s3", "endpoint")),
		EndpointInner: firstNonEmpty(policy.EndpointInner, cfg.DriverString("s3", "endpoint_inner")),
		CDNURL:        firstNonEmpty(policy.CDNURL, cfg.DriverString("s3", "cdn_url")),
		Region:        firstNonEmpty(policy.Region, cfg.DriverString("s3", "region")),
		AccessKey:     firstNonEmpty(policy.AccessKey, cfg.DriverString("s3", "access_key")),
		SecretKey:     firstNonEmpty(policy.SecretKey, cfg.DriverString("s3", "secret_key")),
	}, minio)
}

func NewR2FromConfig(cfg core.Config) (*Driver, error) {
	policy := cfg.Policy
	endpoint := firstNonEmpty(policy.Endpoint, cfg.DriverString("r2", "endpoint"))
	if endpoint == "" {
		if accountID := cfg.DriverString("r2", "account_id"); accountID != "" {
			endpoint = "https://" + accountID + ".r2.cloudflarestorage.com"
		}
	}
	return New(Config{
		Bucket:        firstNonEmpty(policy.Bucket, cfg.DriverString("r2", "bucket")),
		Endpoint:      endpoint,
		EndpointInner: firstNonEmpty(policy.EndpointInner, cfg.DriverString("r2", "endpoint_inner")),
		CDNURL:        firstNonEmpty(policy.CDNURL, cfg.DriverString("r2", "cdn_url")),
		Region:        firstNonEmpty(policy.Region, cfg.DriverString("r2", "region"), "auto"),
		AccessKey:     firstNonEmpty(policy.AccessKey, cfg.DriverString("r2", "access_key"), cfg.DriverString("r2", "access_key_id")),
		SecretKey:     firstNonEmpty(policy.SecretKey, cfg.DriverString("r2", "secret_key"), cfg.DriverString("r2", "access_secret")),
	}, true)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func s3Endpoint(cfg Config) string {
	endpoint := firstNonEmpty(cfg.EndpointInner, cfg.Endpoint)
	if endpoint == "" || strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "https://" + endpoint
}

func (d *Driver) Put(file core.FileHeader) error {
	info := file.Info()
	_, err := d.svc.PutObject(context.Background(), &awss3.PutObjectInput{
		Bucket:      aws.String(d.cfg.Bucket),
		Key:         aws.String(info.Path),
		Body:        file,
		ContentType: aws.String(info.MIMEType),
	})
	return err
}

func (d *Driver) Get(path string) (io.ReadCloser, error) {
	out, err := d.svc.GetObject(context.Background(), &awss3.GetObjectInput{
		Bucket: aws.String(d.cfg.Bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (d *Driver) Delete(paths ...string) error {
	objs := make([]s3types.ObjectIdentifier, len(paths))
	for i, p := range paths {
		objs[i] = s3types.ObjectIdentifier{Key: aws.String(p)}
	}
	_, err := d.svc.DeleteObjects(context.Background(), &awss3.DeleteObjectsInput{
		Bucket: aws.String(d.cfg.Bucket),
		Delete: &s3types.Delete{Objects: objs},
	})
	return err
}

func (d *Driver) List(dir string) ([]core.Object, error) {
	prefix := dir
	if prefix != "" && !bytes.HasSuffix([]byte(prefix), []byte("/")) {
		prefix += "/"
	}

	out, err := d.svc.ListObjectsV2(context.Background(), &awss3.ListObjectsV2Input{
		Bucket: aws.String(d.cfg.Bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, err
	}

	var objects []core.Object
	for _, obj := range out.Contents {
		key := aws.ToString(obj.Key)
		objects = append(objects, core.Object{
			Name:    path.Base(key),
			Path:    key,
			Size:    aws.ToInt64(obj.Size),
			IsDir:   bytes.HasSuffix([]byte(key), []byte("/")),
			ModTime: aws.ToTime(obj.LastModified),
		})
	}
	return objects, nil
}

func (d *Driver) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) {
	chunkSize := int64(5 << 20) // 5MB

	// 小文件：单次 PUT presigned URL
	if info.Size <= chunkSize || info.Size == 0 {
		req, err := d.presign.PresignPutObject(context.Background(), &awss3.PutObjectInput{
			Bucket: aws.String(d.cfg.Bucket),
			Key:    aws.String(info.Path),
		}, func(opts *awss3.PresignOptions) {
			opts.Expires = ttl
		})
		if err != nil {
			return nil, err
		}
		return &core.UploadCredential{
			Mode:       core.UploadModeDirectPut,
			Gateway:    "s3",
			Path:       info.Path,
			ChunkSize:  chunkSize,
			ChunkNum:   1,
			UploadURLs: []string{req.URL},
		}, nil
	}

	// 大文件：MultipartUpload
	expires := time.Now().Add(ttl)
	res, err := d.svc.CreateMultipartUpload(context.Background(), &awss3.CreateMultipartUploadInput{
		Bucket:      aws.String(d.cfg.Bucket),
		Key:         aws.String(info.Path),
		Expires:     &expires,
		ContentType: aws.String(info.MIMEType),
	})
	if err != nil {
		return nil, fmt.Errorf("创建分片上传失败: %w", err)
	}

	chunkNum := int(info.Size / chunkSize)
	if info.Size%chunkSize != 0 {
		chunkNum++
	}

	urls := make([]string, chunkNum)
	for i := 0; i < chunkNum; i++ {
		req, err := d.presign.PresignUploadPart(context.Background(), &awss3.UploadPartInput{
			Bucket:     aws.String(d.cfg.Bucket),
			Key:        aws.String(info.Path),
			PartNumber: aws.Int32(int32(i + 1)),
			UploadId:   res.UploadId,
		}, func(opts *awss3.PresignOptions) {
			opts.Expires = ttl
		})
		if err != nil {
			return nil, err
		}
		urls[i] = req.URL
	}

	completeURL, err := d.presignCompleteMultipartUpload(context.Background(), info.Path, aws.ToString(res.UploadId), ttl)
	if err != nil {
		return nil, err
	}

	return &core.UploadCredential{
		Mode:        core.UploadModeMultipartPut,
		Gateway:     "s3",
		UploadID:    aws.ToString(res.UploadId),
		Path:        info.Path,
		ChunkSize:   chunkSize,
		ChunkNum:    chunkNum,
		UploadURLs:  urls,
		CompleteURL: completeURL,
	}, nil
}

func (d *Driver) Source(path string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		if objectURL := core.JoinObjectURL(d.cfg.CDNURL, path); objectURL != "" {
			return objectURL, nil
		}
	}
	ttl = core.NormalizeSourceTTL(ttl)
	req, err := d.presign.PresignGetObject(context.Background(), &awss3.GetObjectInput{
		Bucket: aws.String(d.cfg.Bucket),
		Key:    aws.String(path),
	}, func(opts *awss3.PresignOptions) {
		opts.Expires = ttl
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (d *Driver) presignCompleteMultipartUpload(ctx context.Context, objectPath string, uploadID string, ttl time.Duration) (string, error) {
	if uploadID == "" {
		return "", fmt.Errorf("s3 multipart upload id is empty")
	}
	u, err := d.objectURL(objectPath)
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("uploadId", uploadID)
	query.Set("X-Amz-Expires", strconv.FormatInt(int64(ttl/time.Second), 10))
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return "", err
	}
	creds, err := d.creds.Retrieve(ctx)
	if err != nil {
		return "", err
	}
	signer := v4.NewSigner()
	signedURL, _, err := signer.PresignHTTP(ctx, creds, req, "UNSIGNED-PAYLOAD", "s3", d.cfg.Region, time.Now())
	if err != nil {
		return "", err
	}
	return signedURL, nil
}

func (d *Driver) objectURL(objectPath string) (*url.URL, error) {
	base := s3Endpoint(d.cfg)
	if base == "" {
		region := d.cfg.Region
		if region == "" {
			region = "us-east-1"
		}
		base = "https://s3." + region + ".amazonaws.com"
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("s3 endpoint is invalid: %q", base)
	}
	keyPath := escapeObjectPath(objectPath)
	if d.path {
		u.Path = joinURLPath(u.Path, d.cfg.Bucket, keyPath)
		return u, nil
	}
	u.Host = d.cfg.Bucket + "." + u.Host
	u.Path = joinURLPath(u.Path, keyPath)
	return u, nil
}

func joinURLPath(parts ...string) string {
	joined := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			joined = append(joined, part)
		}
	}
	return "/" + strings.Join(joined, "/")
}

func escapeObjectPath(objectPath string) string {
	parts := strings.Split(objectPath, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
