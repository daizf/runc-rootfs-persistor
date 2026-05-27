package overlay

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

type Overlay struct {
	LowerDir  string
	UpperDir  string
	WorkDir   string
	MergedDir string
}

func New(lowerDir, basePath, subPath string) *Overlay {
	return &Overlay{
		LowerDir:  lowerDir,
		UpperDir:  fmt.Sprintf("%s/%s/upper", basePath, subPath),
		WorkDir:   fmt.Sprintf("%s/%s/work", basePath, subPath),
		MergedDir: fmt.Sprintf("%s/%s/merged", basePath, subPath),
	}
}

func (o *Overlay) Setup() error {
	for _, dir := range []string{o.UpperDir, o.WorkDir, o.MergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	mounted, err := IsMounted(o.MergedDir)
	if err != nil {
		return fmt.Errorf("check mount %s: %w", o.MergedDir, err)
	}
	if mounted {
		return nil
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		o.LowerDir, o.UpperDir, o.WorkDir)

	if err := syscall.Mount("overlay", o.MergedDir, "overlay", 0, opts); err != nil {
		return fmt.Errorf("mount overlay on %s: %w", o.MergedDir, err)
	}

	return nil
}

func (o *Overlay) Teardown() error {
	return Teardown(o.MergedDir)
}

func Teardown(mergedDir string) error {
	mounted, err := IsMounted(mergedDir)
	if err != nil {
		return fmt.Errorf("check mount %s: %w", mergedDir, err)
	}
	if !mounted {
		return nil
	}

	if err := syscall.Unmount(mergedDir, 0); err != nil {
		return fmt.Errorf("umount %s: %w", mergedDir, err)
	}

	return nil
}

func IsMounted(path string) (bool, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false, fmt.Errorf("read /proc/mounts: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == path {
			return true, nil
		}
	}
	return false, nil
}
