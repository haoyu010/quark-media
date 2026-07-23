//go:build ignore
// +build ignore

package organize

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// copyFileSafe 复制文件到目标位置（不删除源文件）
func copyFileSafe(src, dst string, overwrite bool) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return nil
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source is a directory: %s", src)
	}
	if dstInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return nil
		}
		if !overwrite {
			return fmt.Errorf("目标文件已存在: %s", dst)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".organizer-copy-*")
	if err != nil {
		return fmt.Errorf("create temp target: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy target: %w", err)
	}
	if err := tmp.Chmod(srcInfo.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod target: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync target: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close target: %w", err)
	}

	if err := installTarget(tmpPath, dst, overwrite); err != nil {
		return err
	}
	cleanup = false
	_ = os.Remove(tmpPath)

	return nil
}

// hardlinkFile 创建硬连接。
// 当源/目标位于不同挂载点时，Linux 会返回 EXDEV（invalid cross-device link）。
// Docker 中即使路径同在 /home 下，只要对应两个 volume/bind mount，也会触发该错误；
// 此时退化为 copy，保留“源文件不删除、目标可独立存在”的整理语义。
func hardlinkFile(src, dst string, overwrite bool) error {
	return hardlinkFileWithLink(src, dst, overwrite, os.Link)
}

func hardlinkFileWithLink(src, dst string, overwrite bool, link func(string, string) error) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return nil
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source is a directory: %s", src)
	}
	if dstInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return nil
		}
		if !overwrite {
			return fmt.Errorf("目标文件已存在: %s", dst)
		}
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove existing target: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	if err := link(src, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return copyFileSafe(src, dst, overwrite)
		}
		return fmt.Errorf("create hardlink: %w", err)
	}

	return nil
}

// softlinkFile 创建软连接/符号链接（可以跨文件系统，但删除源文件会导致目标失效）
func softlinkFile(src, dst string, overwrite bool) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return nil
	}

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if _, err := os.Lstat(dst); err == nil {
		if !overwrite {
			return fmt.Errorf("目标文件已存在: %s", dst)
		}
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove existing target: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}

	return nil
}

// moveFileSafe 移动文件（同文件系统原子移动，跨文件系统复制+删除）
func moveFileSafe(src, dst string, overwrite bool) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return nil
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source is a directory: %s", src)
	}
	if dstInfo, err := os.Stat(dst); err == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return nil
		}
		if !overwrite {
			return fmt.Errorf("目标文件已存在: %s", dst)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// os.Rename 在 Linux 上原生覆盖目标；Windows 上若 overwrite=true，先 Remove 再 Rename
	if overwrite {
		_ = os.Remove(dst)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	return copyThenRemove(src, dst, srcInfo, overwrite)
}

func copyThenRemove(src, dst string, srcInfo os.FileInfo, overwrite bool) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".organizer-move-*")
	if err != nil {
		return fmt.Errorf("create temp target: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy target: %w", err)
	}
	if err := tmp.Chmod(srcInfo.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod target: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync target: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close target: %w", err)
	}

	if err := installTarget(tmpPath, dst, overwrite); err != nil {
		return err
	}
	cleanup = false
	_ = os.Remove(tmpPath)

	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove source after copy: %w", err)
	}
	return nil
}

// installTarget 用 tmpPath 替换 dst，按 overwrite 决定是否允许覆盖
func installTarget(tmpPath, dst string, overwrite bool) error {
	if overwrite {
		// Rename 在 Linux/macOS 原生覆盖；Windows 不行，需先 Remove
		if err := os.Rename(tmpPath, dst); err == nil {
			return nil
		}
		_ = os.Remove(dst)
		if err := os.Rename(tmpPath, dst); err != nil {
			return fmt.Errorf("install target (overwrite): %w", err)
		}
		return nil
	}
	if err := os.Link(tmpPath, dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			return fmt.Errorf("目标文件已存在: %s", dst)
		}
		return fmt.Errorf("install target: %w", err)
	}
	return nil
}

// performFileOperation 执行文件操作（根据模式选择）
func performFileOperation(src, dst string, mode FileOperationMode, overwrite bool) error {
	switch mode {
	case FileOpMove:
		return moveFileSafe(src, dst, overwrite)
	case FileOpCopy:
		return copyFileSafe(src, dst, overwrite)
	case FileOpHardlink:
		return hardlinkFile(src, dst, overwrite)
	case FileOpSoftlink:
		return softlinkFile(src, dst, overwrite)
	default:
		return fmt.Errorf("unsupported file operation mode: %d", mode)
	}
}
