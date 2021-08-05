package tags

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type TagLister struct {
	keychain *DockerConfigKeychain
}

func NewTagLister(keychain *DockerConfigKeychain) (*TagLister, error) {
	return &TagLister{
		keychain: keychain,
	}, nil
}

func (t *TagLister) ListTags(image string, keychain *DockerConfigKeychain) ([]string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, err
	}

	mergedKeychain := MergeKeychains([]*DockerConfigKeychain{t.keychain, keychain}...)

	tags, err := remote.List(ref.Context(), remote.WithAuthFromKeychain(mergedKeychain))
	if err != nil {
		return nil, err
	}
	return tags, nil
}

func (t *TagLister) GetTagOfImage(image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", err
	}

	return ref.Identifier(), nil
}
