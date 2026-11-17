# Solovey Backend

Solovey Backend is the Panel control plane library used by Solovey UI. It owns authenticated HTTP adapters, Panel administrator security, browser sessions and CSRF, settings and encrypted values, audit storage, SQLite admission and migration coordination, and data lifecycle operations.

The Product composes the library into its existing executable. Component lifecycle, network configuration, privileged operations, frontend assets and signed Product updates remain with their semantic owners. Data lifecycle receives narrow installed-owner, backup and resource-admission ports; it uses the existing Panel database authority.

The module path is `github.com/MalenkiySolovey/solovey-backend`. Its accepted shared interfaces are `solovey-host v0.10.0` and `solovey-contracts v1.0.0`. Consumers select an immutable accepted module version. Repository separation does not require another daemon or listener.

For source qualification, use Go 1.26.11 with cgo enabled:

```sh
GOWORK=off GOFLAGS=-mod=readonly go test -race -count=1 -timeout=15m ./...
GOWORK=off GOFLAGS=-mod=readonly go vet ./...
node tools/verify-source.mjs
```

Independent CI runs ordinary tests, race tests, vet, module checks and source provenance on Linux and Windows. Product integration tests separately cover the actual frontend bundle, ComponentHost, providers, profiles and Backup/Restore composition.

Source exports retain GPL v3 licensing and the original Solovey UI attribution. See LICENSE, NOTICE and SOURCE_PROVENANCE.json for the source identity and file hashes.
