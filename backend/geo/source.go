package geo

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// Edition is one of the two databases the backend reads.
type Edition string

const (
	EditionCity Edition = "City"
	EditionASN  Edition = "ASN"
)

// maxDatabaseBytes bounds what a download may expand to. The largest databases
// are a few hundred MB; this only stops a runaway or hostile response.
const maxDatabaseBytes = 2 << 30

// Source downloads databases from one publisher.
type Source interface {
	// Name is the publisher, for logs.
	Name() string
	// Fetch writes the edition's .mmdb file to dst.
	Fetch(ctx context.Context, e Edition, dst string) error
}

// NewSource returns MaxMind GeoLite2 when licenseKey is set, else keyless DB-IP Lite.
func NewSource(licenseKey string) Source {
	client := &http.Client{Timeout: 15 * time.Minute}
	if licenseKey != "" {
		return &MaxMindSource{Client: client, BaseURL: "https://download.maxmind.com/app/geoip_download", LicenseKey: licenseKey}
	}
	return &DBIPSource{Client: client, BaseURL: "https://download.db-ip.com/free", Now: time.Now}
}

// httpStatusError is a download that the server answered with a non-200 status.
type httpStatusError struct{ status int }

func (e httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.status) }

// get requests u and hands the body to read. Errors never include u, which may
// carry a license key.
func get(ctx context.Context, c *http.Client, u string, read func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errors.New("bad download URL")
	}
	res, err := c.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return httpStatusError{res.StatusCode}
	}
	return read(res.Body)
}

// writeFile copies r to dst, refusing anything larger than maxDatabaseBytes.
func writeFile(dst string, r io.Reader) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxDatabaseBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxDatabaseBytes {
		err = errors.New("download is implausibly large")
	}
	if err == nil && n == 0 {
		err = errors.New("download is empty")
	}
	return err
}

// DBIPSource fetches DB-IP Lite, published monthly under CC BY 4.0.
type DBIPSource struct {
	Client  *http.Client
	BaseURL string
	Now     func() time.Time
}

func (*DBIPSource) Name() string { return "DB-IP Lite" }

func (s *DBIPSource) Fetch(ctx context.Context, e Edition, dst string) error {
	now := s.Now().UTC()
	// Early in a month the new file may not be published yet: fall back to last month's.
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	var lastErr error
	for _, month := range []time.Time{first, first.AddDate(0, -1, 0)} {
		u := fmt.Sprintf("%s/dbip-%s-lite-%s.mmdb.gz", s.BaseURL, strings.ToLower(string(e)), month.Format("2006-01"))
		err := get(ctx, s.Client, u, func(body io.Reader) error {
			gz, err := gzip.NewReader(body)
			if err != nil {
				return fmt.Errorf("not a gzip file: %w", err)
			}
			defer gz.Close()
			return writeFile(dst, gz)
		})
		var status httpStatusError
		if errors.As(err, &status) {
			lastErr = err
			continue
		}
		if err != nil {
			return fmt.Errorf("download DB-IP %s database: %w", e, err)
		}
		return nil
	}
	return fmt.Errorf("download DB-IP %s database: %w (DB-IP may have changed its URLs)", e, lastErr)
}

// MaxMindSource fetches GeoLite2, which needs a free license key.
type MaxMindSource struct {
	Client     *http.Client
	BaseURL    string
	LicenseKey string
}

func (*MaxMindSource) Name() string { return "MaxMind GeoLite2" }

func (s *MaxMindSource) Fetch(ctx context.Context, e Edition, dst string) error {
	q := url.Values{"edition_id": {"GeoLite2-" + string(e)}, "license_key": {s.LicenseKey}, "suffix": {"tar.gz"}}
	err := get(ctx, s.Client, s.BaseURL+"?"+q.Encode(), func(body io.Reader) error {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return fmt.Errorf("not a gzip file: %w", err)
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				return errors.New("archive has no .mmdb file")
			}
			if err != nil {
				return fmt.Errorf("read archive: %w", err)
			}
			if h.Typeflag == tar.TypeReg && path.Ext(h.Name) == ".mmdb" {
				return writeFile(dst, tr)
			}
		}
	})
	if err != nil {
		var status httpStatusError
		if errors.As(err, &status) && (status.status == http.StatusUnauthorized || status.status == http.StatusForbidden) {
			return fmt.Errorf("download MaxMind %s database: %w (check MAXMIND_LICENSE_KEY)", e, err)
		}
		return fmt.Errorf("download MaxMind %s database: %w", e, err)
	}
	return nil
}
