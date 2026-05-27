package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const StateDir = "/var/lib/runc-rootfs-persist"

type Info struct {
	MergedDir string `json:"mergedDir"`
	UpperDir  string `json:"upperDir"`
}

func Dir() string {
	return StateDir
}

func EnsureDir() error {
	return os.MkdirAll(StateDir, 0755)
}

func Path(containerID string) string {
	return filepath.Join(StateDir, containerID+".json")
}

func Read(containerID string) (*Info, error) {
	data, err := os.ReadFile(Path(containerID))
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &info, nil
}

func Write(containerID string, info *Info) error {
	if err := EnsureDir(); err != nil {
		return err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(Path(containerID), data, 0644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func Remove(containerID string) error {
	return os.Remove(Path(containerID))
}
