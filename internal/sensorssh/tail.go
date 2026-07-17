package sensorssh

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"time"
)

const defaultPollInterval = 500 * time.Millisecond

// TailFile, path'i satir satir takip eder ve her tam satir icin handle cagirir.
// Poll tabanlidir (harici bagimlilik yok) ve log rotasyonuna dayaniklidir:
//   - rotate (path yeni bir inode'a isaret eder) -> yeni dosya bastan okunur,
//   - truncate (dosya offset'in altina kuculur) -> bastan okunur,
//   - dosya henuz yoksa olusana kadar poll edilir.
//
// fromStart=false ise mevcut icerik atlanir, yalnizca yeni satirlar okunur
// (canli takip). ctx iptal edilince nil doner.
func TailFile(ctx context.Context, path string, pollInterval time.Duration, fromStart bool, handle func(string)) error {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	var (
		f       *os.File
		info    os.FileInfo
		offset  int64
		partial []byte
	)
	closeFile := func() {
		if f != nil {
			_ = f.Close()
			f = nil
		}
	}
	defer closeFile()

	openFile := func() error {
		nf, err := os.Open(path) //nolint:gosec // path operator tarafindan verilir (AUTH_LOG_PATH)
		if err != nil {
			return err
		}
		fi, err := nf.Stat()
		if err != nil {
			_ = nf.Close()
			return err
		}
		f, info, partial = nf, fi, nil
		offset = 0
		if !fromStart {
			offset = fi.Size()
		}
		return nil
	}

	buf := make([]byte, 32*1024)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		if f == nil {
			if err := openFile(); err != nil {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					continue
				}
			}
			// Rotate sonrasi yeniden aciliyorsa yeni dosyayi bastan oku.
			fromStart = true
		}

		n, err := f.ReadAt(buf, offset)
		if n > 0 {
			offset += int64(n)
			partial = append(partial, buf[:n]...)
			for {
				i := bytes.IndexByte(partial, '\n')
				if i < 0 {
					break
				}
				handle(string(bytes.TrimRight(partial[:i], "\r")))
				partial = partial[i+1:]
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			closeFile()
			continue
		}

		if rotatedOrTruncated(path, info, offset) {
			closeFile()
			continue
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// rotatedOrTruncated, path'in artik acik dosyadan farkli bir inode'a isaret
// ettigini (rotate) veya dosyanin offset'in altina kuculdugunu (truncate)
// ya da tamamen kayboldugunu bildirir; bu durumlarda dosya yeniden acilmali.
func rotatedOrTruncated(path string, openInfo os.FileInfo, offset int64) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return true
	}
	if !os.SameFile(openInfo, fi) {
		return true
	}
	return fi.Size() < offset
}
