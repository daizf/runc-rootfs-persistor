package config

import "encoding/json"

const (
	AnnotationEnabled       = "eki.rootfs-persist.enabled"
	AnnotationVolumeMapping = "eki.rootfs-persist.volume-mapping"
)

type VolumeMapping struct {
	ContainerName string `json:"containerName"`
	MountPath     string `json:"mountPath"`
	SubPath       string `json:"subPath"`
}

func ParseVolumeMapping(raw string) ([]VolumeMapping, error) {
	var mappings []VolumeMapping
	if err := json.Unmarshal([]byte(raw), &mappings); err != nil {
		return nil, err
	}
	return mappings, nil
}

func FindMapping(mappings []VolumeMapping, containerName string) *VolumeMapping {
	for i := range mappings {
		if mappings[i].ContainerName == containerName {
			return &mappings[i]
		}
	}
	return nil
}
