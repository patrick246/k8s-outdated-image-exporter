package tags

import (
	"encoding/json"
	"github.com/google/go-containerregistry/pkg/authn"
	"os"
	"strings"
)

type DockerConfigKeychain struct {
	authConfigs map[string]authn.AuthConfig
}

func (d DockerConfigKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	authConfig, ok := d.authConfigs[resource.RegistryStr()]
	if !ok {
		return authn.Anonymous, nil
	}
	return authn.FromConfig(authConfig), nil
}

func ReadRegistryCredentialsFromString(cred string) (*DockerConfigKeychain, error) {
	var config struct {
		AuthConfig map[string]authn.AuthConfig `json:"auths"`
	}
	err := json.NewDecoder(strings.NewReader(cred)).Decode(&config)
	if err != nil {
		return &DockerConfigKeychain{}, err
	}

	return &DockerConfigKeychain{authConfigs: config.AuthConfig}, nil
}

func ReadRegistryCredentialsFromFile(path string) (*DockerConfigKeychain, error) {
	file, err := os.Open(path)
	if err != nil {
		return &DockerConfigKeychain{}, err
	}

	var config struct {
		AuthConfig map[string]authn.AuthConfig `json:"auths"`
	}
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return &DockerConfigKeychain{}, err
	}

	return &DockerConfigKeychain{authConfigs: config.AuthConfig}, nil
}
