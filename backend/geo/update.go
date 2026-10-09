package geo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Updater keeps the City and ASN databases on disk fresh and hot-swaps them into a Swappable.
//
// An update downloads both files next to their final paths as ".part", opens
// and checks them, and only then renames them into place and swaps them in. A
// failure at any step leaves the files on disk and the databases in use as
// they were. Renaming does not disturb the running process: the old databases
// stay mapped to their old files until the swap closes them.
type Updater struct {
	CityPath, ASNPath string
	Source            Source
	// Target receives the new databases. Nil only writes the files (the fetch-geoip command).
	Target *Swappable
	Logf   func(format string, args ...any)

	// MaxAge is how old the City file may get before it is refreshed. The
	// publishers release monthly, so the default is a little under a month.
	MaxAge time.Duration
	// CheckEvery is how often Run looks at whether a refresh is due.
	CheckEvery time.Duration
	// RetryMissing is how soon Run retries while there is no database at all.
	RetryMissing time.Duration
	Now          func() time.Time

	mu sync.Mutex
}

// NewUpdater returns an Updater with the default schedule.
func NewUpdater(cityPath, asnPath string, src Source, target *Swappable) *Updater {
	return &Updater{
		CityPath: cityPath, ASNPath: asnPath, Source: src, Target: target,
		Logf:         func(string, ...any) {},
		MaxAge:       30 * 24 * time.Hour,
		CheckEvery:   24 * time.Hour,
		RetryMissing: time.Hour,
		Now:          time.Now,
	}
}

func nonEmpty(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Size() > 0
}

// Present reports whether both database files exist.
func (u *Updater) Present() bool { return nonEmpty(u.CityPath) && nonEmpty(u.ASNPath) }

// Due reports whether the files are missing or older than MaxAge.
func (u *Updater) Due() bool {
	if !u.Present() {
		return true
	}
	st, err := os.Stat(u.CityPath)
	return err != nil || u.Now().Sub(st.ModTime()) > u.MaxAge
}

// Run updates whatever is due now, then checks every CheckEvery until ctx ends.
// Failures are logged and retried; they never stop the loop.
func (u *Updater) Run(ctx context.Context) {
	for {
		wait := u.CheckEvery
		if u.Due() || (u.Target != nil && !u.Target.Info().Available) {
			if _, err := u.Update(ctx, false); err != nil {
				if ctx.Err() != nil {
					return
				}
				if u.Target != nil && !u.Target.Info().Available {
					u.Logf("geoip: download failed, running without locations; retrying in %s: %v", u.RetryMissing, err)
					wait = u.RetryMissing
				} else {
					u.Logf("geoip: refresh failed, keeping the current databases; retrying in %s: %v", u.CheckEvery, err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Update downloads both databases and installs them. It reports whether it did:
// unless force is set, a download that is not newer than the databases in use is
// dropped (and the files' age reset so it is not tried again until the next cycle).
func (u *Updater) Update(ctx context.Context, force bool) (bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.Logf("geoip: downloading %s databases", u.Source.Name())
	editions := []struct {
		e     Edition
		final string
	}{{EditionCity, u.CityPath}, {EditionASN, u.ASNPath}}
	for _, ed := range editions {
		if err := os.MkdirAll(filepath.Dir(ed.final), 0o755); err != nil {
			return false, err
		}
		part := ed.final + ".part"
		defer os.Remove(part)
		if err := u.Source.Fetch(ctx, ed.e, part); err != nil {
			return false, err
		}
	}
	next, err := OpenMaxMind(u.CityPath+".part", u.ASNPath+".part")
	if err != nil {
		return false, fmt.Errorf("downloaded databases are unusable: %w", err)
	}
	if err := next.check(); err != nil {
		next.Close()
		return false, fmt.Errorf("downloaded databases are unusable: %w", err)
	}
	info := next.Info()
	if !force && u.Target != nil {
		if cur := u.Target.Info(); cur.Available && !info.BuildTime().After(cur.BuildTime()) {
			next.Close()
			now := u.Now()
			os.Chtimes(u.CityPath, now, now)
			u.Logf("geoip: already up to date (%s, built %s)", info.Provider, info.Updated)
			return false, nil
		}
	}
	for _, ed := range editions {
		if err := os.Rename(ed.final+".part", ed.final); err != nil {
			next.Close()
			return false, err
		}
	}
	if u.Target != nil {
		u.Target.Swap(next)
	} else {
		next.Close()
	}
	u.Logf("geoip: installed %s databases built %s", info.Provider, info.Updated)
	return true, nil
}

// check rejects files that open but are not the City and ASN databases.
func (m *MaxMind) check() error {
	if t := m.city.Metadata().DatabaseType; !strings.Contains(t, "City") {
		return fmt.Errorf("city database has type %q", t)
	}
	if m.asn != nil {
		if t := m.asn.Metadata().DatabaseType; !strings.Contains(t, "ASN") {
			return fmt.Errorf("asn database has type %q", t)
		}
	}
	if m.city.Metadata().BuildEpoch == 0 {
		return errors.New("city database has no build date")
	}
	return nil
}
