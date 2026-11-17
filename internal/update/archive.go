package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
)

func extractBinary(archivePath, assetName, binaryName, outputPath string) (err error) {
	defer func() {
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()
	if strings.HasSuffix(strings.ToLower(assetName), ".zip") {
		archive, err := zip.OpenReader(archivePath)
		if err != nil {
			return err
		}
		defer archive.Close()
		var selected *zip.File
		score, ambiguous := 0, false
		for _, file := range archive.File {
			if !file.Mode().IsRegular() {
				continue
			}
			reader, err := file.Open()
			if err != nil {
				return err
			}
			header := make([]byte, 4)
			n, _ := io.ReadFull(reader, header)
			reader.Close()
			if !executableHeader(header[:n], binaryName) {
				continue
			}
			rank := candidateScore(file.Name, binaryName)
			if rank > score {
				selected, score, ambiguous = file, rank, false
			} else if rank == score {
				ambiguous = true
			}
		}
		if selected == nil {
			return fmt.Errorf("发布 ZIP 中找不到当前平台可执行内核（不要求固定文件名 %s）", binaryName)
		}
		if ambiguous {
			return fmt.Errorf("发布 ZIP 中存在多个同优先级内核候选文件")
		}
		if selected.UncompressedSize64 > 150<<20 {
			return fmt.Errorf("mihomo 二进制超过 150 MiB 限制")
		}
		reader, err := selected.Open()
		if err != nil {
			return err
		}
		defer reader.Close()
		return writeLimited(outputPath, reader, 150<<20)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	var input io.Reader = file
	lower := strings.ToLower(assetName)
	if !strings.HasSuffix(lower, ".tar") {
		reader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer reader.Close()
		input = reader
	}
	buffered := bufio.NewReader(input)
	header, _ := buffered.Peek(512)
	if strings.HasSuffix(lower, ".tar") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") || len(header) >= 262 && string(header[257:262]) == "ustar" {
		return extractTar(buffered, binaryName, outputPath)
	}
	if !executableHeader(header, binaryName) {
		return fmt.Errorf("GZIP 内容不是可识别的内核可执行文件")
	}
	return writeLimited(outputPath, buffered, 150<<20)
}

// File signatures skip service units, configs and documentation at any depth.
func executableHeader(header []byte, binaryName string) bool {
	if strings.HasSuffix(strings.ToLower(binaryName), ".exe") {
		return bytes.HasPrefix(header, []byte("MZ"))
	}
	return bytes.HasPrefix(header, []byte{0x7f, 'E', 'L', 'F'}) || bytes.HasPrefix(header, []byte{0xcf, 0xfa, 0xed, 0xfe}) || bytes.HasPrefix(header, []byte{0xfe, 0xed, 0xfa, 0xcf}) || bytes.HasPrefix(header, []byte{0xca, 0xfe, 0xba, 0xbe})
}

func candidateScore(name, binaryName string) int {
	base := strings.ToLower(path.Base(strings.ReplaceAll(name, "\\", "/")))
	if base == strings.ToLower(binaryName) {
		return 3
	}
	if strings.HasPrefix(base, "mihomo-") || strings.HasPrefix(base, "mihomo_") {
		return 2
	}
	return 1
}

func extractTar(input io.Reader, binaryName, outputPath string) error {
	reader := tar.NewReader(io.LimitReader(input, 512<<20))
	score, ambiguous := 0, false
	for {
		entry, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeRegA {
			continue
		}
		header := make([]byte, 4)
		n, _ := io.ReadFull(reader, header)
		if !executableHeader(header[:n], binaryName) {
			continue
		}
		rank := candidateScore(entry.Name, binaryName)
		if rank == score {
			ambiguous = true
			continue
		}
		if rank < score {
			continue
		}
		if entry.Size > 150<<20 {
			return fmt.Errorf("mihomo 二进制超过 150 MiB 限制")
		}
		if err := writeLimited(outputPath, io.MultiReader(bytes.NewReader(header[:n]), reader), 150<<20); err != nil {
			return err
		}
		score, ambiguous = rank, false
	}
	if score == 0 {
		return fmt.Errorf("发布 TAR 中找不到内核可执行文件（已跳过服务单元和配置目录）")
	}
	if ambiguous {
		return fmt.Errorf("发布 TAR 中存在多个同优先级内核候选文件")
	}
	return nil
}

func writeLimited(path string, reader io.Reader, maximum int64) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(reader, maximum+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written == 0 || written > maximum {
		_ = os.Remove(path)
		return fmt.Errorf("解压后的 mihomo 内核超过大小限制")
	}
	return nil
}

func binarySuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func trustedHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "github.com" || host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}
