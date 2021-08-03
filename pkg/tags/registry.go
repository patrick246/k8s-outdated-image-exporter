package tags

import (
	"fmt"
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

func (t *TagLister) ListTagsWithCreds(image string, creds []string) ([]string, error) {
	var tags []string
	for _, cred := range creds {
		dockerConfigKeychain, err := ReadRegistryCredentialsFromString(cred)
		if err != nil {
			return nil, err
		}

		ref, err := name.ParseReference(image)
		if err != nil {
			return nil, err
		}
		scopedTags, err := remote.List(ref.Context(), remote.WithAuthFromKeychain(dockerConfigKeychain))
		if err != nil {
			//TO DO
			fmt.Println(err)
		}
		tags = append(tags, scopedTags...)
	}

	return tags, nil
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
