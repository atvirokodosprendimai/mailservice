# Immutable Image Deploy Spec

## Problem

Production deploy currently pulls:

- `ghcr.io/atvirokodosprendimai/mailservice-api:latest`
- `ghcr.io/atvirokodosprendimai/mailservice-mailreceive:latest`

That creates a race:

1. merge lands on `main`
2. Docker publish workflow starts
3. Hetzner deploy workflow starts
4. deploy pulls `latest` before the new image is published

Result:
- infra and runtime config are current
- application image may still be old

## Goal

Deploy the exact application artifact that belongs to the commit being released.

## Decision

Use the existing GHCR SHA tags already produced by the Docker workflow:

- `ghcr.io/atvirokodosprendimai/mailservice-api:sha-<commit>`
- `ghcr.io/atvirokodosprendimai/mailservice-mailreceive:sha-<commit>`

The deploy workflow must:

1. derive the expected tag from `github.sha`
2. wait until both GHCR tags exist
3. write those image references into `production.env`
4. let compose pull those exact immutable tags

## Scope

In scope:
- deploy workflow
- production compose file
- runtime env template
- docs

Out of scope:
- replacing GHCR
- changing the Docker build workflow tag format
- rollback UX beyond documenting that immutable tags make rollback possible

## Required Changes

### Compose

`compose.tunnel.yml.example` should use:

- `image: ${API_IMAGE:-ghcr.io/atvirokodosprendimai/mailservice-api:latest}`
- `image: ${MAILRECEIVE_IMAGE:-ghcr.io/atvirokodosprendimai/mailservice-mailreceive:latest}`

That keeps local/dev compatibility while allowing production to inject exact tags.

### Deploy workflow

Before SSH deployment:

1. compute:
   - `API_IMAGE=ghcr.io/...-api:sha-${GITHUB_SHA}`
   - `MAILRECEIVE_IMAGE=ghcr.io/...-mailreceive:sha-${GITHUB_SHA}`
2. authenticate to GHCR
3. retry until both image manifests exist
4. write `API_IMAGE` and `MAILRECEIVE_IMAGE` into `production.env`

### Docs

Document:
- production deploy is commit-pinned
- mutable `latest` is no longer the production source of truth
- rollback can use a previously published SHA tag

## Acceptance Test

Given a merge to `main`,
when the Docker publish and deploy workflows overlap,
then deploy must still wait for and pull the exact `sha-<commit>` images instead of an older `latest` image.
