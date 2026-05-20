package s3

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage/core"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awss3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type Driver struct {
	cfg    Config
	svc    *awss3.S3
	upload *s3manager.Uploader
}

type Config struct {
	Bucket        string
	Endpoint      string
	EndpointInner string
	PublicURL     string
	CDNURL        string
	Region        string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
}

func New(cfg Config, minio bool) (*Driver, error) {
	endpoint := cfg.Endpoint
	if cfg.EndpointInner != "" {
		endpoint = cfg.EndpointInner
	}
	sess, err := session.NewSession(&aws.Config{
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(cfg.Region),
		Credentials:      credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, ""),
		S3ForcePathStyle: aws.Bool(minio),
		DisableSSL:       aws.Bool(!cfg.UseSSL),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 session: %w", err)
	}
	return &Driver{
		cfg:    cfg,
		svc:    awss3.New(sess),
		upload: s3manager.NewUploader(sess),
	}, nil
}

func NewFromConfig(cfg core.Config, minio bool) (*Driver, error) {
	policy := cfg.Policy
	return New(Config{
		Bucket:        firstNonEmpty(policy.Bucket, cfg.DriverString("s3", "bucket")),
		Endpoint:      firstNonEmpty(policy.Endpoint, cfg.DriverString("s3", "endpoint")),
		EndpointInner: firstNonEmpty(policy.EndpointInner, cfg.DriverString("s3", "endpoint_inner")),
		PublicURL:     firstNonEmpty(policy.PublicURL, cfg.DriverString("s3", "public_url")),
		CDNURL:        firstNonEmpty(policy.CDNURL, cfg.DriverString("s3", "cdn_url")),
		Region:        firstNonEmpty(policy.Region, cfg.DriverString("s3", "region")),
		AccessKey:     firstNonEmpty(policy.AccessKey, cfg.DriverString("s3", "access_key")),
		SecretKey:     firstNonEmpty(policy.SecretKey, cfg.DriverString("s3", "secret_key")),
		UseSSL:        policy.UseSSL || cfg.DriverBool("s3", "use_ssl"),
	}, minio)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (d *Driver) bucket() *string { return aws.String(d.cfg.Bucket) }

func (d *Driver) Put(file core.FileHeader) error {
	info := file.Info()
	_, err := d.upload.Upload(&s3manager.UploadInput{
		Bucket:      d.bucket(),
		Key:         aws.String(info.Path),
		Body:        file,
		ContentType: aws.String(info.MIMEType),
	})
	return err
}

func (d *Driver) Get(path string) (io.ReadCloser, error) {
	out, err := d.svc.GetObject(&awss3.GetObjectInput{
		Bucket: d.bucket(),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (d *Driver) Delete(paths ...string) error {
	objs := make([]*awss3.ObjectIdentifier, len(paths))
	for i, p := range paths {
		objs[i] = &awss3.ObjectIdentifier{Key: aws.String(p)}
	}
	_, err := d.svc.DeleteObjects(&awss3.DeleteObjectsInput{
		Bucket: d.bucket(),
		Delete: &awss3.Delete{Objects: objs},
	})
	return err
}

func (d *Driver) List(dir string) ([]core.Object, error) {
	prefix := dir
	if prefix != "" && !bytes.HasSuffix([]byte(prefix), []byte("/")) {
		prefix += "/"
	}

	out, err := d.svc.ListObjectsV2(&awss3.ListObjectsV2Input{
		Bucket: d.bucket(),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, err
	}

	var objects []core.Object
	for _, obj := range out.Contents {
		objects = append(objects, core.Object{
			Name:    path.Base(*obj.Key),
			Path:    *obj.Key,
			Size:    *obj.Size,
			IsDir:   bytes.HasSuffix([]byte(*obj.Key), []byte("/")),
			ModTime: *obj.LastModified,
		})
	}
	return objects, nil
}

func (d *Driver) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) {
	chunkSize := int64(5 << 20) // 5MB

	// 小文件：单次 PUT presigned URL
	if info.Size <= chunkSize || info.Size == 0 {
		req, _ := d.svc.PutObjectRequest(&awss3.PutObjectInput{
			Bucket: d.bucket(),
			Key:    aws.String(info.Path),
		})
		url, err := req.Presign(ttl)
		if err != nil {
			return nil, err
		}
		return &core.UploadCredential{
			Mode:       core.UploadModeDirectPut,
			Gateway:    "s3",
			Path:       info.Path,
			ChunkSize:  chunkSize,
			ChunkNum:   1,
			UploadURLs: []string{url},
		}, nil
	}

	// 大文件：MultipartUpload
	expires := time.Now().Add(ttl)
	res, err := d.svc.CreateMultipartUpload(&awss3.CreateMultipartUploadInput{
		Bucket:      d.bucket(),
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
		req, _ := d.svc.UploadPartRequest(&awss3.UploadPartInput{
			Bucket:     d.bucket(),
			Key:        aws.String(info.Path),
			PartNumber: aws.Int64(int64(i + 1)),
			UploadId:   res.UploadId,
		})
		urls[i], err = req.Presign(ttl)
		if err != nil {
			return nil, err
		}
	}

	completeReq, _ := d.svc.CompleteMultipartUploadRequest(&awss3.CompleteMultipartUploadInput{
		Bucket:   d.bucket(),
		Key:      aws.String(info.Path),
		UploadId: res.UploadId,
	})
	completeURL, _ := completeReq.Presign(ttl)

	return &core.UploadCredential{
		Mode:        core.UploadModeMultipartPut,
		Gateway:     "s3",
		UploadID:    *res.UploadId,
		Path:        info.Path,
		ChunkSize:   chunkSize,
		ChunkNum:    chunkNum,
		UploadURLs:  urls,
		CompleteURL: completeURL,
	}, nil
}

func (d *Driver) Source(path string, ttl time.Duration) (string, error) {
	if publicURL := core.JoinPublicURL(publicBaseURL(d.cfg.PublicURL, d.cfg.CDNURL), path); publicURL != "" {
		return publicURL, nil
	}
	req, _ := d.svc.GetObjectRequest(&awss3.GetObjectInput{
		Bucket: d.bucket(),
		Key:    aws.String(path),
	})
	if ttl > 0 {
		return req.Presign(ttl)
	}
	return req.Presign(7 * 24 * time.Hour)
}

func publicBaseURL(publicURL, cdnURL string) string {
	if cdnURL != "" {
		return cdnURL
	}
	return publicURL
}
