package tilt

import (
	"crypto"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
)

const TiltPodTemplateHashLabel = "tilt.dev/pod-template-hash"

type PodTemplateSpecHash string

func HashPodTemplateSpec(spec *v1.PodTemplateSpec) (PodTemplateSpecHash, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return "", errors.Wrap(err, "serializing spec to json")
	}

	h := crypto.SHA1.New()
	_, err = h.Write(data)
	if err != nil {
		return "", errors.Wrap(err, "writing to hash")
	}
	return PodTemplateSpecHash(fmt.Sprintf("%x", h.Sum(nil)[:10])), nil
}
