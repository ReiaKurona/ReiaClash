package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseLayouts(t *testing.T) {
	windows := []byte("MZ-versioned-test-kernel")
	linux := append([]byte{0x7f, 'E', 'L', 'F'}, []byte("linux-test-kernel")...)
	for _, tt := range []struct {
		name, asset, binary, entry string
		data                       []byte
	}{
		{"versioned Windows ZIP", "release.zip", "mihomo.exe", "mihomo-windows-amd64-v1.99.exe", windows},
		{"nested Windows ZIP", "release.zip", "mihomo.exe", "release/bin/renamed-core.exe", windows},
		{"nested Linux ZIP", "release.zip", "mihomo", "usr/local/bin/mihomo-linux-amd64", linux},
		{"Linux TAR GZIP", "release.tar.gz", "mihomo", "usr/bin/mihomo-linux-amd64-v1.99", linux},
		{"Linux TGZ", "release.tgz", "mihomo", "opt/core/mihomo", linux},
		{"Linux raw GZIP", "release.gz", "mihomo", "", linux},
	} {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			source := filepath.Join(directory, tt.asset)
			output := filepath.Join(directory, "extracted")
			var archive bytes.Buffer
			if filepath.Ext(tt.asset) == ".zip" {
				writer := zip.NewWriter(&archive)
				for name, data := range map[string][]byte{"etc/systemd/system/mihomo.service": []byte("[Service]\nExecStart=/usr/bin/mihomo"), "etc/mihomo/config.yaml": []byte("mode: rule"), tt.entry: tt.data} {
					entry, err := writer.Create(name)
					if err != nil {
						t.Fatal(err)
					}
					entry.Write(data)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				gzipWriter := gzip.NewWriter(&archive)
				if tt.entry == "" {
					gzipWriter.Write(tt.data)
				} else {
					writer := tar.NewWriter(gzipWriter)
					for name, data := range map[string][]byte{"lib/systemd/system/mihomo.service": []byte("[Service]"), "etc/mihomo/config.yaml": []byte("mode: rule"), tt.entry: tt.data} {
						if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
							t.Fatal(err)
						}
						writer.Write(data)
					}
					if err := writer.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if err := gzipWriter.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(source, archive.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if err := extractBinary(source, tt.asset, tt.binary, output); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.data) {
				t.Fatalf("wrong extracted payload %q", got)
			}
			if _, err := os.Stat(filepath.Join(directory, "etc")); !os.IsNotExist(err) {
				t.Fatal("package directories must not be installed")
			}
		})
	}
}

func TestArchiveRejectsAmbiguousOrMissingKernel(t *testing.T) {
	for _, names := range [][]string{{"README.md"}, {"bin/mihomo-a.exe", "bin/mihomo-b.exe"}} {
		directory := t.TempDir()
		source := filepath.Join(directory, "bad.zip")
		output := filepath.Join(directory, "kernel")
		var data bytes.Buffer
		writer := zip.NewWriter(&data)
		for _, name := range names {
			entry, _ := writer.Create(name)
			if name == "README.md" {
				entry.Write([]byte("documentation"))
			} else {
				entry.Write([]byte("MZ-test"))
			}
		}
		writer.Close()
		os.WriteFile(source, data.Bytes(), 0600)
		if err := extractBinary(source, "bad.zip", "mihomo.exe", output); err == nil {
			t.Fatal("invalid archive accepted")
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("failed update left an output binary")
		}
	}
}
