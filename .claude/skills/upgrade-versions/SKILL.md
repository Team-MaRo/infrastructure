---
name: upgrade-versions
description: Find and upgrade outdated versions anywhere in this repo - chart ranges, upstream manifest URLs, image tags and overrides, k3s, tool pins - e.g. "what's outdated", "upgrade traefik", "image audit". Holds the method; where versions live is found by its script, how each component upgrades stays in its README.
argument-hint: "[component]"
---

# Upgrading versions

A version here is never listed in one place: `scripts/inventory.sh` finds every location by its
shape, and each component's README holds what is particular to it. This skill is the method
around both. One component at a time, each its own commit.

## 1. Inventory

```sh
.claude/skills/upgrade-versions/scripts/inventory.sh [filter]
```

The filter (the argument, e.g. `traefik`) matches the file path or the image/chart name; a full
run takes about a minute. Every row: `file:line` (`cluster/<namespaces>` for images only a chart
or operator sets), kind, name, current, running, newest on the same line (major.minor), newest
overall, flag. RUNNING lists every tag of that image name running anywhere in the cluster, not
only this location's. Read the flags:

- `behind-patch` - a fix release; usually one commit, still read its notes.
- `behind-minor` / `behind-major` - a deliberate upgrade with its own plan (step 2 onward).
- `override` - an image tag set above what its chart ships. Upgrade it like an image; once the
  chart's own version is at least as new, the upgrade is **removing** the override.
- `floating` - Keel rolls it out (`kubernetes/components/keel/README.md`); a new *line* (`3.15`
  -> `3.16`, `9` -> `10`) is still a deliberate tag change.
- `unrecognised` / `lookup-failed` - look it up by hand; a shape the script missed is fixed in
  the script, not worked around.

Running and git differ for a reason worth one sentence each: a chart range that already took a
patch, k3s upgraded by system-upgrade-controller past `k3s_version` (that pin only bootstraps),
Keel's digest. Anything else is drift - say so before upgrading.

Present the table's non-`ok` rows grouped by urgency (advisories first) when asked "what's
outdated"; for one component go straight on.

## 2. Read before changing

- The component's `README.md` (and its "Upgrading" part) - the rules, couplings and runbooks for
  that component live there, per AGENTS.md.
- The release notes of **every** version between current and target, and the project's security
  advisories, against how this repo uses it (its values, its CRDs, the features named in its
  README). A sweep over several minors goes to a subagent on a cheaper model with that list of
  uses; it returns only what touches them and a verdict.
- Whether a chart range can bring it at all: a project without patch releases (only minors), a
  chart whose `appVersion` lags its app, a k3s-bundled component - the inventory's "newest on
  the same line" shows it.

Step done when every skipped release's breaking changes, migrations and advisories are either
irrelevant to this repo (with the reason) or part of the change.

## 3. Change, with every copy

- The version itself, in the place the inventory named.
- **Every other copy of the old version string**: `git grep -n '<old version>'` repo-wide, and
  for an image the old tag without its suffix. Copies are how couplings show up without a list -
  a test image in a script, an interpreter pin that must match the app (`go.mod` of the new
  release says which), a second image of the same app, a README line. Each hit is changed,
  or kept with a reason (history: a dated test result stays).
- An override above a chart gets a comment: why, and "remove once the chart ships at least this".
- The component's README in the same change (AGENTS.md): what changed and why, dated.

## 4. Render the diff

The rendered difference must be only the intended change:

```sh
kubectl kustomize kubernetes/components/<app>                          # Kustomize components
helm template <app> <chart> --repo <repoURL> --version <resolved> -n <ns> -f kubernetes/components/<app>/values.yaml
```

Render old and new (old values from `git show HEAD:<path>`) and diff. A chart minor or major:
also diff the chart's `values.yaml` between versions and check CRDs (served/stored versions,
conversion webhooks). Tests that cover the component run now (the repo's hooks run on commit).

## 5. Commit, push, verify

Present the change per file and the message (`fix: upgrade <app> to <version> for its security
fixes`, or `feat:` for a move to a floating tag); commit and push only on the admin's word, per
the commit rules. A data-holding component (database, OpenBao) gets a fresh backup or snapshot
first, asked for.

After the push, wait for the result rather than polling blindly (deploy waits cap at 2 minutes,
then check): pods on the new version and Ready without restarts, Argo CD Synced/Healthy, the
component's own check from its README, and whatever depends on it (logins after an identity or
database change). A user-facing check the admin has to do (a login) is asked for by name.

Then the roadmap's audit item, and the next component from step 1.
