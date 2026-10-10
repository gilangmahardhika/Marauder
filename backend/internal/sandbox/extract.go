package sandbox

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

func ExtractArchive(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader || hdr.Typeflag == tar.TypeXHeader {
			continue
		}
		name, err := cleanEntry(hdr.Name)
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			if linkEscapes(name, hdr.Linkname) {
				return fmt.Errorf("archive link %s escapes the export", hdr.Name)
			}
		}
		target := path.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(path.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			continue
		default:
			return fmt.Errorf("archive entry %s has type %q", hdr.Name, hdr.Typeflag)
		}
	}
}

func cleanEntry(name string) (string, error) {
	if name == "" || strings.Contains(name, "\x00") || strings.HasPrefix(name, "/") || path.IsAbs(name) {
		return "", fmt.Errorf("archive entry %s is absolute", name)
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("archive entry %s escapes the export", name)
	}
	return clean, nil
}

func linkEscapes(name, target string) bool {
	if target == "" || strings.HasPrefix(target, "/") || path.IsAbs(target) {
		return true
	}
	joined := path.Clean(path.Join(path.Dir(name), target))
	return joined == ".." || strings.HasPrefix(joined, "../")
}
