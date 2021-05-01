package tags

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type TagLister struct {
	keychain authn.Keychain
}

func NewTagLister(keychain authn.Keychain) (*TagLister, error) {
	return &TagLister{
		keychain: keychain,
	}, nil
}

func (t *TagLister) ListTags(image string) ([]string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, err
	}

	tags, err := remote.List(ref.Context(), remote.WithAuthFromKeychain(t.keychain))
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
