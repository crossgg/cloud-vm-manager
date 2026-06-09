# Update and Release

## Docker Runtime Update

The Docker deployment uses two mounted directories:

```yaml
volumes:
  - ./config:/app/config
  - ./runtime:/app/runtime
```

`/app/config` stores user configuration and keys.

`/app/runtime` stores the binary installed by the in-app updater. This keeps update artifacts out of the config directory.

Container startup order:

1. If `/app/runtime/cloud-vm-manager` exists and is executable, run it.
2. Otherwise run the image bundled `/app/cloud-vm-manager`.

When the web UI applies an update, the server downloads the matching GitHub Release archive, verifies `checksums.txt`, extracts the binary to `/app/runtime/cloud-vm-manager`, and exits. Docker restarts the container because `restart: unless-stopped` is set.

## Release Artifacts

The release workflow creates these archives:

```text
cloud-vm-manager_windows_amd64.zip
cloud-vm-manager_linux_amd64.tar.gz
cloud-vm-manager_linux_arm64.tar.gz
cloud-vm-manager_linux_armv7.tar.gz
checksums.txt
```

Each archive contains:

```text
cloud-vm-manager / cloud-vm-manager.exe
README-update.txt
```

GitHub also provides source code archives automatically on every Release.

## Creating a Release

Push a tag:

```bash
git tag v1.2.3
git push origin v1.2.3
```

Or run the `Release` workflow manually and provide a version such as `v1.2.3`.

The workflow also pushes Docker images:

```text
ghcr.io/<owner>/cloud-vm-manager:latest
ghcr.io/<owner>/cloud-vm-manager:v1.2.3
```

## Updater Repository

By default, the updater checks:

```text
crossgg/cloud-vm-manager
```

Override this with:

```text
UPDATE_GITHUB_REPO=owner/repo
```

For private repositories, use a public Release download path or extend the updater with a GitHub token.

## Download Proxy

The update page supports a download proxy for GitHub Release assets.

Built-in options:

```text
Direct GitHub
https://gh-proxy.com/
Custom
```

Release API requests, Release asset downloads, and `checksums.txt` downloads are proxied when a proxy is selected.

Set a default proxy with:

```text
UPDATE_DOWNLOAD_PROXY=https://gh-proxy.com/
```

The web UI can override the default for a single update operation.
