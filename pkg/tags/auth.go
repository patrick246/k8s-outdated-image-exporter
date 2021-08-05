package tags

import (
	"encoding/json"
	"github.com/google/go-containerregistry/pkg/authn"
	coreV1 "k8s.io/api/core/v1"
	"log"
	"os"
)

type DockerConfigKeychain struct {
	authConfigs map[string]authn.AuthConfig
}

type dockerConfigJson struct {
	AuthConfig map[string]authn.AuthConfig `json:"auths"`
}

func (d DockerConfigKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	authConfig, ok := d.authConfigs[resource.RegistryStr()]
	if !ok {
		return authn.Anonymous, nil
	}
	return authn.FromConfig(authConfig), nil
}

func MergeKeychains(keychains ...*DockerConfigKeychain) *DockerConfigKeychain {
	authConfigs := map[string]authn.AuthConfig{}

	for _, keychain := range keychains {
		for registryName, auth := range keychain.authConfigs {
			authConfigs[registryName] = auth
		}
	}

	return &DockerConfigKeychain{authConfigs: authConfigs}
}

func ReadRegistryCredentialsFromFile(path string) (*DockerConfigKeychain, error) {
	file, err := os.Open(path)
	if err != nil {
		return &DockerConfigKeychain{}, err
	}

	var config dockerConfigJson
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return &DockerConfigKeychain{}, err
	}

	return &DockerConfigKeychain{authConfigs: config.AuthConfig}, nil
}

func RegistryCredentialsFromSecrets(secrets []*coreV1.Secret) *DockerConfigKeychain {
	var configs []dockerConfigJson
	for _, secret := range secrets {
		var decoded []byte
		var ok bool
		switch secret.Type {
		case coreV1.SecretTypeDockercfg:
			decoded, ok = secret.Data[coreV1.DockerConfigKey]
			if !ok {
				log.Printf("expected secret %s/%s to contain key %s, key was not present", secret.Namespace, secret.Name, coreV1.DockerConfigKey)
				continue
			}
		case coreV1.SecretTypeDockerConfigJson:
			decoded, ok = secret.Data[coreV1.DockerConfigJsonKey]
			if !ok {
				log.Printf("expected secret %s/%s to contain key %s, key was not present", secret.Namespace, secret.Name, coreV1.DockerConfigJsonKey)
				continue
			}
		default:
			log.Printf("unknown secret type %s for secret %s/%s", secret.Type, secret.GetNamespace(), secret.GetName())
			continue
		}
		var config dockerConfigJson
		err := json.Unmarshal(decoded, &config)
		if err != nil {
			log.Printf("error decoding secret %s/%s, invalid json as pull secret: %s", secret.Namespace, secret.Name, err.Error())
			continue
		}

		configs = append(configs, config)
	}

	mergedConfig := map[string]authn.AuthConfig{}
	for _, config := range configs {
		for registryName, auth := range config.AuthConfig {
			mergedConfig[registryName] = auth
		}
	}

	return &DockerConfigKeychain{
		authConfigs: mergedConfig,
	}
}
