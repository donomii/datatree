#!/bin/sh

# Local listen address. Port 0 asks macOS to select an available port; set a fixed port only when another service is not using it.
DATATREE_ADDR="${DATATREE_ADDR:-127.0.0.1:0}"
# Optional kubeconfig path or path list. Empty uses kubectl's normal selection.
DATATREE_KUBECONFIG="${DATATREE_KUBECONFIG:-}"
# Maximum unique entries shown at each kubernetes tree level; 0 shows all entries, while a positive limit adds an explanatory error row when exceeded.
DATATREE_KUBERNETES_MAX_RESULTS="${DATATREE_KUBERNETES_MAX_RESULTS:-0}"
# Maximum bytes retained from each kubectl output stream; larger output produces an explanatory error row.
DATATREE_KUBERNETES_MAX_OUTPUT_BYTES="${DATATREE_KUBERNETES_MAX_OUTPUT_BYTES:-8388608}"
# GitHub CLI executable used for pull request browsing. Change this when gh is not on PATH or a specific installation is required.
DATATREE_GH="${DATATREE_GH:-gh}"
# Maximum bytes retained from each GitHub CLI output stream; larger output produces an explanatory error row.
DATATREE_GITHUB_MAX_OUTPUT_BYTES="${DATATREE_GITHUB_MAX_OUTPUT_BYTES:-8388608}"
# AWS CLI executable used for profile, Resource Explorer, region, CloudFormation, and Cloud Control browsing. Change this when aws is not on PATH or a specific installation is required.
DATATREE_AWS="${DATATREE_AWS:-aws}"
# Maximum bytes retained from each AWS CLI output stream; larger output produces an explanatory error row.
DATATREE_AWS_MAX_OUTPUT_BYTES="${DATATREE_AWS_MAX_OUTPUT_BYTES:-8388608}"
# macOS system_profiler executable used for the local hardware and USB trees. Change this only when system_profiler is installed elsewhere or a test substitute is required.
DATATREE_SYSTEM_PROFILER="${DATATREE_SYSTEM_PROFILER:-/usr/sbin/system_profiler}"
# Maximum bytes retained from each system_profiler output stream; larger output produces an explanatory error row.
DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES="${DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES:-8388608}"
# Maximum time allowed for each system_profiler command; a stalled command produces an explanatory error row.
DATATREE_SYSTEM_PROFILER_TIMEOUT="${DATATREE_SYSTEM_PROFILER_TIMEOUT:-30s}"
# Git executable used for repository browsing. Change this when git is not on PATH or a specific installation is required.
DATATREE_GIT="${DATATREE_GIT:-git}"
# Maximum rows shown in each Git repository view; extra rows produce an explanatory error row.
DATATREE_GIT_MAX_RESULTS="${DATATREE_GIT_MAX_RESULTS:-500}"
# Maximum bytes retained from each Git command; larger output produces an explanatory error row.
DATATREE_GIT_MAX_OUTPUT_BYTES="${DATATREE_GIT_MAX_OUTPUT_BYTES:-8388608}"
export DATATREE_ADDR DATATREE_KUBECONFIG DATATREE_KUBERNETES_MAX_RESULTS DATATREE_KUBERNETES_MAX_OUTPUT_BYTES DATATREE_GH DATATREE_GITHUB_MAX_OUTPUT_BYTES DATATREE_AWS DATATREE_AWS_MAX_OUTPUT_BYTES DATATREE_SYSTEM_PROFILER DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES DATATREE_SYSTEM_PROFILER_TIMEOUT DATATREE_GIT DATATREE_GIT_MAX_RESULTS DATATREE_GIT_MAX_OUTPUT_BYTES

# Build Datatree
./build.sh

# run
./datatree "$@"
