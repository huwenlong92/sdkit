package local

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

type Driver struct {
	cfg Config
	dir string
}

type Config struct {
	Dir       string
	CDNURL    string
	Endpoint  string
	SecretKey string
}

func New(dir string) *Driver {
	return NewWithConfig(Config{Dir: dir})
}

func NewWithConfig(cfg Config) *Driver {
	dir := cfg.Dir
	abs, _ := filepath.Abs(dir)
	return &Driver{cfg: cfg, dir: abs}
}

func NewFromConfig(cfg core.Config) *Driver {
	policy := cfg.Policy
	return NewWithConfig(Config{
		Dir:       firstNonEmpty(policy.LocalDir, cfg.DriverString("local", "dir")),
		CDNURL:    firstNonEmpty(policy.CDNURL, cfg.DriverString("local", "cdn_url")),
		Endpoint:  firstNonEmpty(policy.Endpoint, cfg.DriverString("local", "endpoint")),
		SecretKey: firstNonEmpty(policy.SecretKey, cfg.DriverString("local", "secret_key")),
	})
}

func (d *Driver) TempDir() string {
	return filepath.Join(d.dir, ".chunks")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (d *Driver) fullPath(path string) string {
	return filepath.Join(d.dir, filepath.FromSlash(path))
}

func (d *Driver) Put(file core.FileHeader) error {
	info := file.Info()
	dst := d.fullPath(info.Path)

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	return err
}

func (d *Driver) Get(path string) (io.ReadCloser, error) {
	return os.Open(d.fullPath(path))
}

func (d *Driver) Delete(paths ...string) error {
	for _, p := range paths {
		if err := os.Remove(d.fullPath(p)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (d *Driver) List(path string) ([]core.Object, error) {
	root := d.fullPath(path)
	var objects []core.Object

	err := filepath.Walk(root, func(p string, fi fs.FileInfo, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		objects = append(objects, core.Object{
			Name:    fi.Name(),
			Path:    filepath.ToSlash(rel),
			Size:    fi.Size(),
			IsDir:   fi.IsDir(),
			ModTime: fi.ModTime(),
		})
		if fi.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return objects, err
}

func (d *Driver) Source(path string, ttl time.Duration) (string, error) {
	if ttl > 0 {
		return core.SignSourceURL(d.cfg.Endpoint, path, d.cfg.SecretKey, ttl, time.Now())
	}
	if objectURL := core.JoinObjectURL(d.cfg.CDNURL, path); objectURL != "" {
		return objectURL, nil
	}
	return d.fullPath(path), nil
}

func (d *Driver) Token(info core.FileInfo, ttl time.Duration) (*core.UploadCredential, error) {
	return &core.UploadCredential{
		Mode:      core.UploadModeLocalChunk,
		Gateway:   "local",
		Path:      info.Path,
		ChunkSize: 5 << 20, // 5MB
	}, nil
}
