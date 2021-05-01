# k8s-outdated-image-exporter
Checks the image tag of pods if there is a newer semver tag in the registry.

## Metrics
 - pod_image_outdated - Exports by how many major, minor or patch versions an image in a podspec is outdated
    - namespace: The kubernetes namespace of the pod
    - pod: The name of the pod
    - type: major/minor/patch, shows the difference to the latest, versioned image tag. If there are two new major versions, the metric with type=major will be 2, the other two will be 0.
    
## Building
```bash
# Binary
CGO_ENABLED=0 go build -o ./bin -ldflags="-extldflags=-static" ./cmd/...

# Docker Image
docker build -t <your-image> .
```

## Deployment
Example Kubernetes manifests are in the `deployments/` folder. You can also use these as `kustomization` base.

## Configuration
`-image-check-interval duration` \
How often to check for new image versions. Configuring this to a lower interval will eat up your registry request quota faster. (default 1h) 

`-in-cluster` \
Controls if the in-cluster connection configuration method should be used. (default true)

`-listen-addr string` \
The address to listen on for metrics requests (default ":8080")

`-registry-credentials path` \
Path to a file containing registry credentials. This is the same format as K8s imagePullSecret contents (default "~/.docker/config.json")