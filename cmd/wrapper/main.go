package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/example/runc-rootfs-persist/pkg/config"
	"github.com/example/runc-rootfs-persist/pkg/overlay"
	"github.com/example/runc-rootfs-persist/pkg/state"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const realRunc = "runc"

var log = logrus.WithField("component", "runc-rootfs-persist")

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

	if err := ensureWritable(); err != nil {
		log.Fatal(err)
	}

	args := os.Args[1:]
	action := findAction(args)

	switch action {
	case "create":
		if err := handleCreate(args); err != nil {
			log.Fatalf("create error: %v", err)
		}
	case "delete":
		if err := handleDelete(args); err != nil {
			log.WithError(err).Warn("delete cleanup error, continuing")
		}
	}

	execRunc(args...)
}

func ensureWritable() error {
	for _, dir := range []string{state.StateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func findAction(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "" {
			continue
		}
		if a[0] == '-' {
			kvFlags := map[string]bool{
				"--root": true, "--log": true, "--log-format": true,
				"--bundle": true, "--pid-file": true, "--process": true,
			}
			if kvFlags[a] && i+1 < len(args) {
				i++
			}
			continue
		}
		if i+1 < len(args) {
			return a
		}
	}
	for _, a := range args {
		if a != "" && a[0] != '-' {
			return a
		}
	}
	return ""
}

func handleCreate(args []string) error {
	bundlePath := flagValue(args, "--bundle")
	if bundlePath == "" {
		return errors.New("--bundle flag not found")
	}
	log.WithField("bundle", bundlePath).Debug("handleCreate started")

	containerID := firstNonFlag(args)
	log.WithFields(logrus.Fields{"containerID": containerID, "args": args}).Debug("parsed args")

	configPath := filepath.Join(bundlePath, "config.json")
	spec, err := readSpec(configPath)
	if err != nil {
		return fmt.Errorf("read OCI spec: %w", err)
	}

	enabled := spec.Annotations[config.AnnotationEnabled]
	log.WithField("annotation-enabled", enabled).Debug("checked annotation")
	if enabled != "true" {
		return nil
	}

	rawMapping := spec.Annotations[config.AnnotationVolumeMapping]
	log.WithField("annotation-mapping", rawMapping != "").Debug("checked mapping annotation")
	if rawMapping == "" {
		return fmt.Errorf("annotation %s is true but %s is missing",
			config.AnnotationEnabled, config.AnnotationVolumeMapping)
	}

	mappings, err := config.ParseVolumeMapping(rawMapping)
	if err != nil {
		return fmt.Errorf("parse volume mapping: %w", err)
	}

	cName := containerName(spec)
	if cName == "" {
		cName = spec.Annotations["io.kubernetes.cri.container-name"]
	}
	log.WithField("containerName", cName).Debug("resolved container name")

	mapping := config.FindMapping(mappings, cName)
	if mapping == nil {
		log.WithField("containerName", cName).Info("no mapping for container, passing through")
		return nil
	}

	pvHostPath := ""
	for _, m := range spec.Mounts {
		if m.Destination == mapping.MountPath {
			pvHostPath = m.Source
			break
		}
	}
	if pvHostPath == "" {
		return fmt.Errorf("mount path %q not found in OCI spec mounts", mapping.MountPath)
	}
	log.WithField("pvHostPath", pvHostPath).Debug("found PV mount")

	originalRoot := spec.Root.Path
	if !filepath.IsAbs(originalRoot) {
		originalRoot = filepath.Join(bundlePath, originalRoot)
	}
	log.WithField("originalRoot", originalRoot).Debug("resolved root path")

	subPath := mapping.SubPath
	if subPath == "" {
		subPath = cName
	}

	ov := overlay.New(originalRoot, pvHostPath, subPath)
	if err := ov.Setup(); err != nil {
		return fmt.Errorf("setup overlay: %w", err)
	}
	log.WithField("mergedDir", ov.MergedDir).Debug("overlay setup done")

	spec.Root.Path = ov.MergedDir

	if err := writeSpec(configPath, spec); err != nil {
		return fmt.Errorf("write OCI spec: %w", err)
	}

	if err := state.Write(containerID, &state.Info{
		MergedDir: ov.MergedDir,
		UpperDir:  ov.UpperDir,
	}); err != nil {
		log.WithError(err).Warn("failed to write state file")
	}

	log.WithFields(logrus.Fields{
		"container":  containerID,
		"lower":      originalRoot,
		"merged":     ov.MergedDir,
		"pvHostPath": pvHostPath,
	}).Info("overlay rootfs persistence enabled")

	return nil
}

func handleDelete(args []string) error {
	containerID := firstNonFlag(args)
	if containerID == "" {
		return nil
	}

	info, err := state.Read(containerID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read state: %w", err)
	}

	if err := overlay.Teardown(info.MergedDir); err != nil {
		log.WithError(err).WithField("merged", info.MergedDir).Warn("teardown overlay")
	}

	if err := state.Remove(containerID); err != nil {
		log.WithError(err).Warn("remove state file")
	}

	log.WithField("container", containerID).Info("overlay rootfs cleanup complete")
	return nil
}

func execRunc(args ...string) {
	bin, err := exec.LookPath(realRunc)
	if err != nil {
		bin = realRunc
	}

	log.WithField("args", fmt.Sprintf("%v", args)).Debug("exec runc")

	if err := syscall.Exec(bin, append([]string{bin}, args...), os.Environ()); err != nil {
		log.Fatalf("exec runc: %v", err)
	}
}

func readSpec(path string) (*specs.Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var spec specs.Spec
	if err := json.NewDecoder(f).Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode config.json: %w", err)
	}
	return &spec, nil
}

func writeSpec(path string, spec *specs.Spec) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(spec)
}

func containerName(spec *specs.Spec) string {
	return spec.Annotations["io.kubernetes.cri.container-name"]
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func firstNonFlag(args []string) string {
	for i := len(args) - 1; i >= 0; i-- {
		a := args[i]
		if a != "" && a[0] != '-' {
			return a
		}
	}
	return ""
}

var _ = io.Discard
