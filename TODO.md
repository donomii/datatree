# TODO

No required completion work remains for the current Datatree implementation. The project builds, runs, and passes its self-test.

Completed 2026-07-10: read-only kubernetes browsing via `kubernetes_driver` (kubectl restricted to get, api-resources, config get-contexts), with a virtual `kubernetes` root entry, fake-kubectl self-test coverage, and a read-only invocation audit.

Completed 2026-07-11: `json_driver` browses JSON documents (and kubernetes manifests) as hierarchical data, and dedicated kubernetes object drivers present curated views: `k8s_pod_driver`, `k8s_workload_driver`, `k8s_service_driver`, `k8s_configmap_driver`, and `k8s_node_driver`, each with a `[manifest]` child keeping the full manifest browsable.

Completed 2026-07-20: the browser uploads a selected file through `/api/upload`, mounts the selected driver through `/api/load`, and keeps each result as an expandable tree. Dedicated image, PDF, HTML, and CSV preview drivers expose image dimensions, PDF metadata and links, HTML titles and links, and structured CSV rows.

Completed 2026-07-20: kubernetes discovery accepts an explicit kubeconfig selection, sorts and deduplicates every context, namespace, resource-type, and object listing, bounds result counts and kubectl output, routes every represented core kind to its curated driver, and preserves manifest-read failures as visible error leaves. The isolated self-test covers these behaviors without contacting or changing a cluster.

Completed 2026-07-20: inactive driver sessions are cleaned up after the configured idle timeout, and shared driver protocol, range, JSON, filesystem, and presenter support lives in `drivers_go/driverlib`.

Completed 2026-07-25: read-only Git repository browsing detects working trees and exposes commits, committed files, staged changes, unstaged changes, untracked files, and conflicts, with exact historical, index, and working-tree bytes behind file rows.

Completed 2026-07-25: the installer smoke test verifies relocated installed driver and web UI fallback paths in the foreground, including installed root and JSON driver execution.
