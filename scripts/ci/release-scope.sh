#!/usr/bin/env bash

# shellcheck disable=SC2034
release_scope() {
  repository="$1"
  tag="$2"
  commit="$3"
  [[ "$repository" == park285/iris-client-go ]] || {
    echo "release bundle: unexpected repository" >&2; return 1;
  }
  [[ "$commit" =~ ^[0-9a-f]{40}$ ]] || {
    echo "release bundle: commit must be a full lowercase Git SHA" >&2; return 1;
  }
  case "$tag" in
    v[1-5].*)
      module_dir=.
      module_prefix=''
      module_path="github.com/$repository"
      if [[ "$tag" != v1.* ]]; then module_path+="/${tag%%.*}"; fi
      version="$tag"
      artifact_name="iris-client-go-$version"
      ;;
    adapters/valkeydedup/v1.*)
      module_dir=adapters/valkeydedup
      module_prefix="$module_dir/"
      module_path="github.com/$repository/$module_dir"
      version="${tag#"$module_prefix"}"
      artifact_name="iris-client-go-valkeydedup-$version"
      ;;
    *) echo "release bundle: unsupported module/tag pair" >&2; return 1 ;;
  esac
  [[ "$version" =~ ^v[1-5]\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
    echo "release bundle: tag must be strict stable semver" >&2; return 1;
  }
  archive="$artifact_name.tar.gz"
  sbom="$artifact_name.spdx.json"
}

release_publish_scope() {
  case "$2" in
    adapters/valkeydedup/v1.*) release_scope "$@" ;;
    *) echo "release bundle: unsupported publication tag" >&2; return 1 ;;
  esac
}

release_source_identity() {
  [[ "$(git rev-parse --verify HEAD)" == "$commit" &&
     "$(git rev-parse --verify "refs/tags/${tag}^{commit}")" == "$commit" ]] || {
    echo "release bundle: checkout or tag target does not match requested commit" >&2; return 1;
  }
  [[ "${GITHUB_REF:-refs/tags/$tag}" == "refs/tags/$tag" && "${GITHUB_SHA:-$commit}" == "$commit" ]] || {
    echo "release bundle: GitHub ref or SHA does not match requested tag target" >&2; return 1;
  }
  local main_commit
  main_commit="$(git show-ref --verify --hash refs/remotes/origin/main)" || {
    echo "release bundle: canonical origin/main authority is unavailable" >&2; return 1;
  }
  git merge-base --is-ancestor "$commit" "$main_commit" || {
    echo "release bundle: tag target is not contained in origin/main" >&2; return 1;
  }
}

release_archive() {
  local paths=() adapter_entry
  if [[ "$module_dir" == . ]]; then
    adapter_entry="$(git ls-tree "$commit" -- adapters/valkeydedup)" || return 1
    if [[ -n "$adapter_entry" ]]; then
      paths=(-- . ':(top,exclude)adapters/valkeydedup')
    fi
  else
    paths=(-- "$module_dir")
  fi
  git archive --format=tar --prefix="$artifact_name/" "$commit" "${paths[@]}"
}
