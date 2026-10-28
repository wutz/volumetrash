# volumetrash

Kubernetes operator that purges trashed project volumes once their retention has
passed. It is the expiry half of the volume trash: forgeai moves a volume to the
trash, and this operator deletes it when its expiry comes.

## How it works

A volume (PersistentVolumeClaim) is in the trash while it carries the label
`fileserver.wutz.dev/ignore=true`. The file server does not serve such a volume.

| Field | Meaning |
| --- | --- |
| label `fileserver.wutz.dev/ignore=true` | The volume is in the trash. |
| label `forgeai.wutz.dev/trash=true` | Legacy trash label from before FileServer 0.3.0; moved to the ignore label. |
| annotation `forgeai.wutz.dev/trashed-at` | When the volume was trashed (RFC3339). |
| annotation `forgeai.wutz.dev/expire-at` | When the volume is purged (RFC3339). |

For a trashed volume the operator:

1. Sets `expire-at` from the retention if the volume has none, for example one
   labelled with `kubectl`.
2. Requeues until `expire-at` has passed.
3. Postpones the purge, and rechecks in 10 minutes, while a pod that has not
   finished mounts the volume. A pod that finishes wakes the purge early.
4. Deletes the volume, with its UID and resourceVersion as preconditions, so a
   volume restored or changed in the meantime is kept.

### Retention

The retention of a volume is the `trashRetention` of its project when the project
sets one. Otherwise it is the platform default:

- The project is found from the `project.wutz.dev/project` label on the namespace,
  and its `spec.trashRetention` is read from the `Project` (`project.wutz.dev`).
- The platform default is the `trashRetention` of the platform config, the
  ConfigMap `forgeai-platform` in `forgeai-system`, key `config.yaml`. It is read
  on each purge schedule, so a change applies to the next one. It defaults to 7d.

Only `trashRetention` is read from the platform config. Other keys are ignored.

## Install

The operator needs the `Project` CRD, installed by the project operator.

```bash
helm install volumetrash oci://ghcr.io/wutz/charts/volumetrash --version 0.1.0 \
  --namespace volumetrash-system --create-namespace
```

Values that matter:

| Value | Default | Meaning |
| --- | --- | --- |
| `platform.namespace` | `forgeai-system` | Namespace of the platform ConfigMap. |
| `platform.configMap` | `forgeai-platform` | Name of the platform ConfigMap. |
| `leaderElection` | `false` | Enable leader election for more than one replica. |
| `image.tag` | chart `appVersion` | Operator image tag. |

The operator needs cluster-wide access to PersistentVolumeClaims, Pods, Namespaces
and Projects, and read access to the platform ConfigMap in its namespace.

## Development

```bash
make test        # unit tests and envtest suite
make lint        # golangci-lint
make helm-lint   # chart lint
make run         # run against the current kubeconfig context
```

The envtest suite needs the Kubernetes test binaries, which `make test` downloads.

## Release

Pushing a `v*` tag builds and pushes the image `ghcr.io/wutz/volumetrash` with ko,
and packages and pushes the chart to `oci://ghcr.io/wutz/charts`. The chart version
and `appVersion` come from the tag, so the chart and image versions match.
