package buildinfo

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Version 由发布构建写入，源码构建保持 dev 便于识别。
var Version = "dev"

// ReleaseSelfCheck 由发布构建设置为 required；源码构建会在同目录存在校验文件时自动检查。
var ReleaseSelfCheck = "auto"

const checksumFileName = "baize-mcp.sha256"

// VerifyExecutable 检查发布包随附的可执行文件校验值，避免用户运行损坏或不完整的文件。
func VerifyExecutable() error {
	required := false
	switch ReleaseSelfCheck {
	case "auto":
	case "required":
		required = true
	default:
		return errors.New("invalid release integrity check mode")
	}

	executable, err := os.Executable()
	if err != nil {
		if required {
			return errors.New("unable to locate executable for integrity check")
		}
		return nil
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		if required {
			return errors.New("unable to resolve executable for integrity check")
		}
		return nil
	}

	return verifyExecutable(executable, filepath.Join(filepath.Dir(executable), checksumFileName), required)
}

func verifyExecutable(executablePath, checksumPath string, required bool) error {
	metadata, err := os.ReadFile(checksumPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("release integrity metadata is missing beside the executable")
		}
		return errors.New("unable to read release integrity metadata")
	}

	expected, err := parseChecksum(metadata, filepath.Base(executablePath))
	if err != nil {
		return err
	}

	file, err := os.Open(executablePath)
	if err != nil {
		return errors.New("unable to open executable for integrity check")
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return errors.New("unable to calculate executable checksum")
	}
	actual := hash.Sum(nil)
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return errors.New("executable checksum does not match its release integrity metadata")
	}
	return nil
}

func parseChecksum(metadata []byte, executableName string) ([]byte, error) {
	fields := strings.Fields(string(metadata))
	if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
		return nil, errors.New("release integrity metadata is invalid")
	}
	expected, err := hex.DecodeString(fields[0])
	if err != nil || filepath.Base(strings.TrimPrefix(fields[1], "*")) != executableName {
		return nil, errors.New("release integrity metadata is invalid")
	}
	return expected, nil
}
