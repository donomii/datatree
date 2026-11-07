Datatree Specification

Standalone repository: https://github.com/donomii/datatree.

Content browsing update:

Restart recovery: server-issued handles for resources rooted in the filesystem/location browser contain an authenticated, encrypted reconstruction route. The route records version 1, the original root directory, and each selected driver, display name, internal child key and kind. It contains no cached file bytes or provider credentials. A private 32-byte AES-GCM key is created once at the operating system user configuration directory under datatree/recovery.key (on macOS: HOME/Library/Application Support/datatree/recovery.key), with file permission 0600 and a private datatree directory. Invalid, public, non-regular or malformed key files are rejected instead of replaced. Handle tokens begin hdl-v1.; fresh non-recoverable upload and resource IDs use random UUIDs, so restart cannot accidentally map an old ID to a different resource.

Scalar selectors retain logical field identity, not displayed values: JSON fields and array elements have stable reserved internal keys, including valid empty field names, and scalar-root values use a reserved scalar key. Recovery can therefore read an updated value at the same field without keeping its previous contents in the route. The -test self-test also launches an internal -test-recovery-worker subprocess, passing only a private key-file path and the original handle over standard input, and verifies the recreated file bytes from a completely fresh process. This mode is solely the restart test worker, not a launcher or user configuration option.

When an API request uses a valid handle whose runtime driver is absent, Datatree decrypts the route, starts an isolated root session using the original root, and replays its read-only driver layers. It matches both resource identity and kind, preferring the original internal key and allowing a unique matching name after listing reordering. Missing or ambiguous resources and provider permission/authentication failures produce specific reconnect errors. Reconstructed sessions receive unique runtime namespaces so recovering one branch cannot destroy another. The original handle becomes an alias for the newly recreated child; its descendants and further filters receive equally recoverable handles. Reads retry one reconstruction after a vanished driver or failed provider read. Expansion, selected filters, downloads and action-target lookup all resolve handles this way. Reconstruction never executes or replays mutations. Idle session cleanup can remove runtime handles without invalidating authenticated reconstruction tokens.

Recreated provider processes reuse the configured CLI accounts and credential stores. AWS normal credential/role refresh and Kubernetes exec-credential refresh remain with their CLIs. An expired configured AWS SSO session triggers one exact sso login for that profile and retries the failed read. Expired stored GitHub credentials trigger one auth refresh for the existing host/account without adding or resetting scopes, followed by one read retry. Environment-supplied GitHub tokens are not replaced. Login flows are bounded to two minutes and may require the provider's browser sign-in or MFA; failures explain the provider sign-in requirement, rather than reporting an invalid handle. Passwords and access tokens are not stored in Datatree's recovery state. Permission denials do not trigger login. Upload bytes remain memory-only: expired uploads and handles issued by versions without recovery information require a fresh upload or reload rather than invented data.

The default dynamic loopback listener remembers its selected address in datatree/listener beside the recovery key. A normal restart tries that same port, preserving the browser origin. If that port is occupied it selects and saves another available local port and prints/opens the new URL; explicitly configured fixed addresses remain authoritative. Malformed saved listener data is rejected. Tests use private temporary recovery keys, fake provider CLIs and real driver subprocesses to recover S3-to-ZIP-to-ZIP-to-text after constructing a completely new router and loading its key from disk, and again after idle-driver removal. Additional tests cover token tampering, changed keys, invalid state preservation, origin reuse, occupied ports and authentication renewal/retry.

All-driver byte contract: every byte filter (ZIP, text, JSON, CSV, HTML, PDF, image, diff, and the five Kubernetes presenters) retains its exact complete input in a readable [source bytes] child of kind raw. That child offers download and explicit filter selection, but does not auto-expand, so recursive expansion cannot create a cycle. Rejected input remains available beside the visible format error. Presenter values and extracted URLs retain their actual value bytes under unique internal keys, rather than bytes reconstructed from labels. The shared Resource record has an implementation-only optional byte payload, never serialized; the harness binds it to the ordinary range-serving map and advertises readable. Navigation and action descriptors remain separate from file or document bytes.

Directory/root files, Git HEAD/index/worktree blobs, S3 objects, Kubernetes manifests and ConfigMap/Secret entries all serve actual payloads through the same range protocol. Git file bytes are independent of the metadata output-size limit and failures use the protocol error field rather than text substituted for file data. ConfigMap and decoded Secret keys use the same filename-based classification as disk files, including ZIP, with UTF-8 fallback for unknown filenames. Hardware/USB data-node handles serve actual JSON; their default presenter accepts those bytes, and JSON or other filters can mount on the same handle. Hardware views retain complete source JSON and scalar bytes. Provider query views expose original command-result bytes alongside their navigation rows, including GitHub responses, AWS service responses and Kubernetes listings.

Range responses contain base64 data, an integer size matching the decoded byte count, and an optional error string. A read failure sets error instead of supplying invented file bytes. Errors are forwarded through the host to the next filter and shown as error rows, or as an HTTP data-read failure. Content is never interpreted as an error merely because it resembles an error-shaped JSON document. Missing/unreadable filesystem children produce explicit errors, distinct from legitimate zero-byte files.

The no-argument XSH launchers build.xsh, run.xsh, test.xsh, demo.xsh and install.xsh work from any directory. run.xsh builds and opens the location browser. test.xsh runs all Go tests and the executable's -test self-test. install.xsh installs to HOME/.local/bin/datatree. Existing shell scripts remain as legacy launchers.

Opening an S3 bucket in the service catalog, Resource Explorer, or Cloud Control view lists objects as ordinary files with original names, actual sizes and the same extension-based kinds as disk files. Their handles lazily serve actual bytes through the normal range protocol, never AWS mount-state JSON. ZIP, JSON, text, CSV and other filters mount directly on those handles, without an intermediate AWS object wrapper. Prefixes remain AWS browsing views. Numbered internal keys distinguish reserved controls and duplicate labels. The first bucket page retains actions, and listings preserve the 1000-object continuation view. Permission and validation failures are visible errors.

S3 provider states carry opaque UTF-8 keys up to 1024 bytes and pagination tokens up to 8192 bytes. Object descriptors require a key and no token; only the provider resolves these descriptors. No key is interpreted as a local path or shell command. Only exact read-only list-objects-v2 and remote-object-to-stdout copy commands are permitted; uploads, recursive copies, local destinations and arbitrary flags are rejected. DATATREE_AWS_MAX_OUTPUT_BYTES (default 8388608) bounds metadata and stderr, not object bytes. Finite range responses retain only requested bytes while draining the CLI stream; whole-document filters receive all bytes. Whole-document processing remains subject to available memory, as it is for disk files.

AWS containers expose data queries beside the complete response JSON: DynamoDB tables to items; CloudWatch groups to streams to log events; Route 53 zones to DNS records; ECR repositories to images; backup vaults to recovery points; Lambda functions to configuration and code-download location; ECS clusters to tasks/services to details; EKS clusters to cluster data; Step Functions machines to definitions and executions to event histories; EventBridge rules to targets; Secrets Manager entries to values; KMS keys to details; SNS topics to attributes and subscriptions; SQS queues to attributes; API Gateway APIs to paths; certificates to details; load balancers to listeners; CloudFormation stacks to resources and templates. Compiled argument templates validate each identifier and retain a parent identifier when needed. CLI automatic pagination reads all results under the output byte bound; exceeding that bound produces an explicit error.

IAM roles, users and groups expose inline policy names followed by their policy documents; customer-managed policies expose versions followed by their documents. Access Analyzer exposes findings, GuardDuty exposes finding identifiers followed by complete finding data, and AWS Config rules expose compliance results. CloudWatch event pages expose a forward-token continuation; the final stable token ends pagination.

SQS message reception changes visibility, so queues explicitly explain why read-only browsing cannot receive messages. RDS and ElastiCache entries explain that records require a database connection and credentials, beyond the AWS management APIs. EKS configuration does not grant workload access; configured Kubernetes contexts remain available through the Kubernetes driver. Lambda code bytes are not fetched from its returned remote location. Image-layer, backup-file and database-protocol access are not implemented by these data queries.

Every byte-backed child offers an explained Filter selector and Apply filter button, defaulting to its ordinary kind (ZIP for archives, text for unknown files). The selected filter interprets the same bytes regardless of provider, extension or nesting depth, replacing that resource's current child view. Its children can be filtered again. JSON scalar leaves preserve exact serialized bytes, advertise readable=true in the driver and HTTP Resource records, and offer downloads and filter controls. Metadata without byte payloads has no filter selector. ZIP entries use unique internal keys, preserving duplicate names independently. Corrupt archives and unreadable entries produce visible errors rather than silently empty listings.

Text, HTML source, binary, JSON and CSV content has 64 KiB byte pages with Previous bytes and Next bytes controls and a zero-based range readout. All bytes can be reached without an 8 MiB preview cutoff. UTF-8 text uses text nodes without script execution; binary and split UTF-8 byte pages show exact hexadecimal bytes. Images and PDFs use complete bytes and local blob URLs revoked on collapse or upload close. Upload-root previews read the selected File; descendants use the same handle protocol as disk and S3 files. The executable self-test exercises real subprocess composition through S3 ZIP, nested ZIP, text, twelve additional ZIP layers, an extensionless ZIP, JSON scalar bytes, empty files, and a 9,000,000-byte object using only a fake AWS CLI.

1. Summary

Datatree is a read-only browser for nested data containers. A host process manages standalone driver programs. Each driver understands one kind of container and exposes that container's children as resources. The host can then mount another driver on any expandable resource, allowing directory-in-directory, ZIP-in-directory, ZIP-in-ZIP, text-in-ZIP, and similar nested browsing.

The system has three user-facing surfaces:

The web UI at the local HTTP URL printed when Datatree starts, for browsing common filesystem locations or a selected directory.

The HTTP API for uploading bytes, loading drivers, expanding child handles, unloading driver sessions, and downloading raw resource bytes.

The shell scripts used to build, run, test, demo, and install the program.

Browsing is read-only. Drivers may read local files and uploaded bytes, but no driver writes changes back to those sources. The kubernetes driver reads cluster data through the kubectl command line tool and is restricted to a fixed allowlist of read-only kubectl subcommands (get, api-resources, and config get-contexts), so it can never modify a cluster. The Git driver reads repositories through a fixed allowlist of non-mutating Git subcommands, disables optional Git locks, and never stages, commits, switches, resets, or otherwise changes repository state. GitHub and AWS changes happen only after the user activates a visible action control and confirms state-changing operations. Same-origin action endpoints resolve opaque server handles to validated targets and permit only exact allowlisted gh or aws command shapes. Unsupported flags, credential writes, arbitrary service commands, and shell execution are refused before execution.

Datatree builds as one executable containing the HTTP host, every driver implementation, and the browser UI. Each driver session remains a separate process for protocol isolation, but the host starts another mode of its own executable instead of a separate driver binary. DATATREE_ADDR overrides the listen address. Without it, Datatree reuses its saved loopback port when available, otherwise port 0 selects and saves an available local port. DATATREE_KUBECTL overrides the kubectl executable used by the kubernetes drivers, which defaults to kubectl found on the PATH. DATATREE_KUBECONFIG selects the kubeconfig path or path list used for discovery; empty preserves kubectl's normal KUBECONFIG and home-directory selection. DATATREE_KUBERNETES_MAX_RESULTS optionally bounds each discovery result when set to a positive value; its default of 0 shows every result. DATATREE_KUBERNETES_MAX_OUTPUT_BYTES bounds each kubectl output stream to 8388608 bytes by default. DATATREE_GH overrides the GitHub CLI executable used by GitHub pull request browsing, which defaults to gh found on the PATH. DATATREE_GITHUB_MAX_OUTPUT_BYTES bounds each GitHub CLI output stream to 8388608 bytes by default. DATATREE_AWS overrides the AWS CLI executable used by AWS browsing and actions, which defaults to aws found on the PATH. DATATREE_AWS_MAX_OUTPUT_BYTES bounds each AWS CLI output stream to 8388608 bytes by default. DATATREE_SYSTEM_PROFILER selects the macOS system_profiler executable used for local hardware and USB browsing, defaulting to /usr/sbin/system_profiler. DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES bounds each system_profiler output stream to 8388608 bytes by default, and DATATREE_SYSTEM_PROFILER_TIMEOUT limits each command to 30s by default. DATATREE_GIT overrides the Git executable used by Git browsing, which defaults to git found on the PATH. DATATREE_GIT_MAX_RESULTS and DATATREE_GIT_MAX_OUTPUT_BYTES bound each Git view to 500 rows and each Git metadata output stream to 8388608 bytes by default; file payloads are not limited by that setting.

2. User Interactions

2.1 Build and run

The recommended macOS installation uses the third-party Homebrew tap:

```
brew tap donomii/tap https://gitlab.com/donomii/homebrew-tap.git
brew install --cask datatree
```

The tap cask downloads a checksummed archive containing the arm64 macOS Datatree executable and links it into Homebrew's binary directory. The release executable is built locally from the Datatree project directory with CGO disabled and tested before packaging. The cask does not invoke Homebrew's source-build environment, so installation does not require current Apple Command Line Tools. The executable is not signed or notarized. The cask explicitly removes the macOS quarantine attribute from the checksummed Datatree executable after installation so Gatekeeper does not block it. Updating the cask requires a new immutable versioned binary archive and matching SHA-256 checksum.

The user runs ./run.sh with an optional directory argument.

The script compiles one ./datatree executable containing the host, drivers, and browser UI, then starts it on the saved loopback port when available, or an available local port selected by the operating system. When a directory argument is present, Datatree opens that directory. Without a directory argument, Datatree opens a location browser containing HOME, /, and readable mounted-volume directories it can discover. Relative directory arguments are resolved from the directory where the user invoked the command. A missing path, non-directory path, or more than one argument stops startup with a complete error. After binding the listening socket, the host prints the complete browser URL, including the selected port, and opens that URL in the default macOS browser.

The user opens the printed http://127.0.0.1:PORT URL in a browser. Opening web/index.html directly with a file URL cannot reach the HTTP API; in that mode the page disables its controls and displays instructions to start Datatree with ./run.sh and use the printed HTTP URL.

When the host starts normally, it loads root_driver after a short delay. root_driver lists the location browser when no directory was selected, or non-hidden entries from the selected directory otherwise. The browser calls GET /api/list until root resources are available.

Browser API requests must come from the Datatree host's own origin. The host returns HTTP 403 for a foreign Origin or browser fetch metadata marking the API request cross-site or same-site but cross-origin. Direct HTTP clients without browser origin metadata can use the API. Browser navigation to the main page remains available from links.

When listening on a loopback address, the host accepts requests addressed to that IP or localhost at the actual listening port. Other authorities receive HTTP 403, including when a domain resolves to the loopback address.

Run ./run-gallery.sh from the directory to browse, or supply a directory argument, to open the 3D exhibit on this Mac. The launcher builds the host, forces a dynamic loopback port, and sets DATATREE_GALLERY_ASSETS to the trusted portfolio source directory. An empty DATATREE_GALLERY_ASSETS disables the exhibit in a regular launch. A configured exhibit requires an explicit browse directory and a loopback listener. The host serves the embedded exhibit and three named assets from the configured directory on the API origin.

The exhibit starts with the sample archive. Browse live Datatree fetches the root listing from the host. Select a plaque or its matching button to expand a directory, ZIP, JSON, text, image, PDF, HTML, or CSV resource using its opaque handle. Other resources show their kind and size and action rows are omitted. The exhibit keeps the current view on request failure and shows the error. Up and the page controls navigate the tree, and Restore sample returns to the bundled archive. Local ZIP and snapshot imports stay in browser memory.

Point a VR controller at **Live Datatree** and select it to browse the host's directory from inside the exhibit. The plaque changes to **Sample archive** when browsing live or imported data. Select it to return to the bundled archive. The exhibit displays progress and errors for live requests and branch opening on a 3D plaque as well as on the page. Another navigation action supersedes an in-flight request.

The headset session requests hand tracking when available. Tracked hands appear
as moving joints and links. Aim and pinch to select the same plaques as a
controller trigger. Each beam brightens and ends at the plaque it can select.
Untracked joints and links disappear, and an untracked aim cannot select.

Focus the scene to walk with WASD, look with the arrow keys, and select the
plaque under the center dot with Enter. A standard gamepad uses the left stick
to walk, the right stick to look, and its bottom face button to select.
In VR, the left motion controller stick moves relative to the viewer's heading.
The right stick turns 30 degrees once per tilt around the viewer's position.
Return it toward center to turn again. A standard gamepad can also move,
turn, and select in VR when the browser exposes one. Virtual walking stays on
the exhibit floor and clear of the display base.

2.2 Location and directory browser

**Save visible tree as JSON** downloads `datatree-snapshot.json` from the directory browser. It captures the names, kinds and byte sizes shown by the driver, plus children of currently expanded rows. Action rows, handles, and raw file bytes are omitted. No additional driver requests occur. The JSON has integer `datatree_snapshot` set to 1 and a `resources` list. Each resource has `name` (string), `kind` (string), `size` (integer, negative when unknown), and optional `children` (resource list). Exports are limited to 12 levels, 5,000 resources, and 4 MiB. Names displayed by a driver may include values.

The browser shows a Directory Browser heading, a loading message while root resources are not ready, and a tree of resources after GET /api/list succeeds. In no-argument mode, HOME expands to the user's home directory, / expands to the filesystem root, VOLUME: NAME entries expand to readable directories discovered directly under /Volumes, and MOUNT: PATH entries expand to readable directories discovered directly under /mnt or /media. Missing or unreadable mount roots are ignored. An explicit directory argument continues to show that directory's entries directly.

When a kubectl executable can be resolved, the tree includes a virtual resource named kubernetes of kind kubernetes. Expanding it lists the contexts from the selected kubeconfig, and deeper expansions let the user select a context, a namespace or cluster scope, a resource type, and an object as described in section 6.5.

When a GitHub CLI executable can be resolved, the tree begins with a virtual resource named github of kind github. Expanding it shows open pull requests authored by the authenticated user and open pull requests requesting that user's review. Expanding a pull request shows [actions], [CI/CD runs], and [diff] children followed by its complete selected gh pr view JSON fields. Expanding [actions] shows controls appropriate to the current pull request state. Expanding [CI/CD runs] shows recent workflow runs for the pull request head branch; each run expands to its jobs, metadata, and state-appropriate actions. Expanding [diff] shows one row per changed file. Expanding a changed file shows the unified diff as readable metadata, hunk, context, removed, and added rows with old and new line numbers.

When an AWS CLI executable can be resolved, the tree includes a virtual resource named aws of kind aws. Expanding it reads local profile names without contacting AWS. A profile shows its non-secret region, output, role, source-profile, credential-source, and SSO configuration when present; configuration actions for region and output; and a regions-and-resources branch. Region expansion contacts the selected AWS profile and offers a service catalog, Resource Explorer search, and Cloud Control resource types. The service catalog has curated read-only views for EC2, S3, IAM, Access Analyzer, Security Hub, GuardDuty, CloudTrail, AWS Config, Lambda, RDS, ECS, EKS, DynamoDB, SQS, SNS, CloudFormation, CloudWatch, API Gateway, ElastiCache, load balancers, Route 53, Secrets Manager metadata, KMS, ECR, AWS Backup, Step Functions, EventBridge, Auto Scaling, Certificate Manager, WAF, Organizations, and Systems Manager. Generic service responses remain recursively browseable JSON. EC2 instances, security groups, S3 buckets, IAM roles, and customer-managed IAM policies become named resources with detail and appropriate action branches. Resource Explorer shows every indexed result returned by the selected Region's view. Cloud Control types list supported AWS resource types; each type lists its resources and a create control, and each resource shows current properties plus update and delete controls. AWS access keys, secret keys, session tokens, secret values, and arbitrary aws commands are never displayed or accepted.

When the macOS system_profiler executable can be resolved, the browser begins with hardware and usb virtual resources in both location-browser and explicitly selected-directory modes. Virtual resources use hidden protocol routing keys, so real children named hardware, usb, kubernetes, github, or aws remain independently browseable beside virtual resources with the same display names. Expanding hardware shows the local machine's hardware overview as readable facts. Expanding usb lists the machine's USB buses, then hubs and devices at their actual nesting depth, with each selected object's properties below it. Duplicate system-profiler names receive stable numeric suffixes so every bus and device remains expandable. These trees read the local machine only, never contact a network service, and do not change hardware or system settings.

Each row shows:

An icon indicating a directory, kubernetes, error, or plain resource.

The resource name.

The resource kind.

The resource size in bytes.

An expand button when the kind can be mounted by another driver. Its arrow points right (`▸`) while the node is closed and down (`▾`) while the node is open. Its tooltip changes from Expand to Collapse with the same state. Ordinary clicks, recursive expansion, and bookmark restoration all update the same arrow state. An open node containing an empty result or visible expansion error remains marked open until the user collapses it.

A recursive-expand button when the kind can be mounted by another driver. It expands the selected node itself, then expands every expandable descendant in depth-first order. If the selected node is already expanded, it leaves that node open and recursively expands its existing descendants. Leaf nodes remain unchanged, and one descendant failure remains visible on that row without preventing later sibling nodes from expanding.

A link button for each expandable resource in the root tree. Activating it writes a URL fragment containing the resource names and kinds from the root to that row, places the URL in the address bar, and copies it when the browser clipboard is available. Loading such a URL validates at most 64 path segments, reloads fresh resources and handles, then expands the matching path in order. Uploaded trees have no link button because their retained bytes do not survive a page reload. A status readout explains copy or restoration success and failures.

A download link for file-like resources that can return raw bytes.

When the user clicks an expand button, the browser sends POST /api/autoLoad with the resource handle and kind. The host selects the correct driver, loads it on that handle, and returns child resources. The browser inserts a nested tree under the clicked row.

If an expanded resource has no child resources, the browser displays "No child resources" under that row.

If expansion fails, the browser displays a row-level error message containing the failed resource name and the server response.

The browser treats url, kv, and error resources as leaves. They are displayed but not expanded and not downloaded. A kv resource's name carries a "key: value" fact from a curated view, and an error resource's name carries a complete failure message, so a kubectl or cluster failure is readable directly in the tree. Expandable container kinds (directory, kubernetes, and the k8s object kinds) show no download link; json resources show both an expand button and a download link for their raw bytes.

Rows of kind kubernetes offer an expand button but no download link, matching the treatment of directories.

Rows of kind git use a branch icon, offer expand and recursive-expand buttons, and have no download link. A directory containing a .git directory or file is shown as kind git. When the selected directory is inside a Git work tree, the root listing begins with a virtual . row of kind git for opening the current repository view directly. Expanding it shows the repository path, current branch, HEAD commit or unborn state, active Git limits, and expandable sections for commits, files committed at HEAD, all uncommitted changes, staged changes, unstaged changes, untracked files, and conflicted files. The all-uncommitted section labels every row with all applicable states; the specific state section heading supplies its context, so its child rows say added, modified, deleted, type changed, untracked, or the complete conflict status without repeating the section name. File rows retain their normal text, JSON, image, PDF, HTML, CSV, archive, or file kind, so their selected Git version can be previewed or downloaded through the existing controls. Deleted files and submodule entries are informational leaves because they have no ordinary file bytes at that view.

Rows of kind github offer expand and recursive-expand buttons and have no download link. GitHub scalar details are kv leaves; nested arrays and objects use json rows and therefore continue through json_driver to arbitrary depth. A changed file in [diff] has kind diff, offers expand and recursive-expand buttons, and has no download link. Its diff-meta, diff-hunk, diff-context, diff-delete, and diff-add children are non-expandable leaves with no download links. Added rows use a green background, removed rows use a red background, hunk headings use a blue background, metadata uses a neutral grey background, and context rows preserve the ordinary code background. Diff code preserves spacing and wraps when required by the available width.

GitHub action kinds render controls instead of expand or download controls. Comment, review-comment, and request-changes controls have labelled text areas limited to 65536 bytes and reject blank text. Merge has a labelled merge-method selector with squash selected by default and merge, squash, and rebase as the only choices. Close, reopen, merge, approve, request changes, cancel, and rerun actions display a confirmation before sending. Each control has a tooltip explaining its effect and a status readout reporting progress, the complete failure, or GitHub's response. Successful actions do not synthesize new state; collapsing and re-expanding the containing GitHub view reads the current server state.

Rows of kind aws offer expand and recursive-expand buttons and no download link. AWS action kinds render labelled controls instead of expansion or download controls. Create accepts a non-empty desired-state JSON object. Update accepts a non-empty RFC 6902 JSON patch array. Security-group ingress authorization and revocation accept a non-empty array of EC2 IP permission objects. S3 bucket policy, IAM trust policy, and IAM managed-policy version controls accept complete non-empty JSON policy objects. S3 versioning offers Enabled and Suspended, with Enabled selected by default. Set default region accepts a validated AWS Region name. Set output format offers json, text, table, yaml, yaml-stream, and off, with json selected by default. Cloud Control delete, S3 bucket-policy delete, and EC2 instance start, stop, reboot, and terminate have no input. Every AWS action displays a confirmation, tooltip, progress and result readout. Cloud Control create, update, and delete are asynchronous and may return an in-progress request token; Datatree displays that response and refreshes only when the user collapses and re-expands the view.

Rows of kind hardware use a computer icon and rows of kind usb use a plug icon. Both kinds offer expand and recursive-expand buttons and no download link. Their scalar properties are kv leaves.

2.3 Downloading data

For file-like rows, the browser provides a download link that calls GET /api/data with the resource handle, offset 0, and length -1.

The host asks the driver that owns the handle to serve the requested child range. The response body is the raw resource bytes with content type application/octet-stream.

2.4 Browser and HTTP upload workflow

Above the home-directory tree, the browser shows a file chooser labelled "Choose a file to inspect", a driver selector labelled "Preview using", an "Upload and preview" button, a status readout, and the active per-file, total retained-memory, and idle-expiry limits. The file chooser selects one local file; the original file is never changed. The driver selector lists the upload-compatible drivers returned by GET /api/drivers and initially selects zip_driver. Internal drivers that require a mount state or filesystem path are not offered. The user selects the driver that understands the chosen file type. Hovering the limits readout explains that inactive uploads may be evicted and that closing a preview releases its retained data.

Clicking "Upload and preview" with no file or no driver displays an explanation beside the controls. With both selected, the browser posts the file bytes to /api/upload, posts the returned resource id and selected driver to /api/load, and displays the resulting resources in a new tree above earlier upload results. Each uploaded tree supports the same expand and download controls as the home-directory tree and has a "Close FILE" button. Closing a preview stops its driver session, removes the session's child handles, releases its uploaded bytes immediately, removes its tree, and reports the release in the status readout. The status readout otherwise reports upload progress, driver loading, the resulting driver session, or the complete server error.

API clients may upload raw bytes with POST /api/upload. The request body is the raw blob. The response contains res_id and size.

API clients may load a driver on an uploaded blob with POST /api/load. The request body contains res_id and driver. The response contains resources, driver, and session.

API clients may load a driver on a child handle with POST /api/loadChild. The request body contains handle and may contain driver or kind. If driver is empty, the host uses the same kind mapping as /api/autoLoad.

API clients may stop a running driver session with POST /api/unload. The request body contains session. The response contains session and stopped.

An upload body may contain at most DATATREE_MAX_UPLOAD_BYTES bytes, 67108864 by default. Datatree never reads more than one byte beyond that limit and rejects a larger body with HTTP 413. All retained uploads together may contain at most DATATREE_MAX_RETAINED_UPLOAD_BYTES bytes, 268435456 by default. Before accepting an upload that would cross the total limit, Datatree removes least-recently-used uploads that have no active driver session. If active uploads occupy too much of the limit, Datatree rejects the new upload with HTTP 507 and reports the requested, retained, and allowed byte counts.

An inactive upload expires when it has not been loaded or served for DATATREE_UPLOAD_IDLE_TIMEOUT, 30m by default. Setting the duration to 0 disables time-based expiry; the total retained-memory limit still applies. Active driver sessions protect their uploaded bytes from expiry and capacity eviction. An idle driver cleanup or explicit unload releases the session's uploaded bytes and child handles immediately. DATATREE_MAX_UPLOAD_BYTES must be a positive byte count no larger than DATATREE_MAX_RETAINED_UPLOAD_BYTES. DATATREE_MAX_RETAINED_UPLOAD_BYTES must be positive. DATATREE_UPLOAD_IDLE_TIMEOUT must be a non-negative Go duration. Invalid settings stop startup with an error identifying the received and expected values.

3. Driver Protocol

Drivers are built-in modes of the Datatree executable. The host starts its own executable in the selected driver mode, and the resulting driver process communicates with the host through JSON objects over stdin and stdout, one object per line.

Driver-to-host range request:

```
{ "cmd": "range", "offset": 0, "length": -1, "child": "optional/name" }
```

offset is the first byte requested.

length is the number of bytes requested. -1 means "through the end".

child is optional during driver initialization and required when a driver is serving one of its children.

Host-to-driver range response:

```
{ "data": "<base64>", "size": 2048 }
```

data is base64-encoded raw bytes.

size is the decoded byte count.

Driver-to-host resources message:

```
{ "resources": [ { "name": "docs/readme.txt", "kind": "text", "size": 2048 } ] }
```

A driver must emit exactly one resources message during initialization. If it finds nothing, it emits:

```
{ "resources": null }
```

After emitting resources, non-leaf drivers keep running and answer range requests for their child resources. Leaf drivers may exit after emitting resources.

4. Data Types

Resource:

name: string. The display name or child identifier.

kind: string. The resource classification used for driver routing.

size: integer. The resource size in bytes. Directories and containers use 0. A Git object whose size is deliberately not fetched during listing uses -1, which the browser displays as "size read on demand" instead of an incorrect byte count.

key: string, optional. An opaque child identifier used only by the driver protocol. When absent, name is the child identifier. The host never exposes key in its HTTP resource response. root_driver assigns NUL-prefixed reserved keys to virtual resources so they cannot collide with filesystem names.

handle: string, optional. Present in host HTTP responses. It is not part of the driver protocol. The browser and API clients use it when expanding or downloading a resource.

Range request:

cmd: string. Must be "range".

offset: integer. Start byte.

length: integer. Byte count, or -1 for the rest of the resource.

child: string, optional. The child resource key when one was emitted, otherwise its name.

Range response:

data: string. Base64-encoded bytes.

size: integer. Decoded byte count.

Child handle:

hid: string. Host-generated handle id.

child_name: string. Name passed back to the owning driver when serving bytes.

size: integer. Resource size in bytes.

kind: string. Resource kind.

driver_name: string. Running driver session that owns the child.

Store:

blobs: uploaded resource id to uploaded blob containing the byte array and last-used time.

blobBytes: total retained uploaded byte count.

maxUploadBytes: maximum byte count accepted for one upload.

maxRetainedUploadBytes: maximum total retained uploaded byte count.

uploadIdleTimeout: how long an inactive upload may remain unused before expiry; zero disables time-based expiry.

driverBlobIDs: driver session to uploaded resource id. Membership protects the upload from eviction.

handles: handle id to child handle.

drivers: session name to running driver process.

rootResources: resource list shown by GET /api/list.

Kubernetes mount state:

context: string, optional. The kubeconfig context to read. Empty means the kubernetes root, which lists contexts.

namespace: string, optional. The namespace whose resource types and objects are being browsed.

cluster: boolean, optional. True when browsing cluster-scoped resource types instead of a namespace. Mutually exclusive with namespace.

type: string, optional. The resource type (as printed by kubectl api-resources -o name) whose objects are being browsed. Requires a namespace or cluster scope.

The mount state is serialized as a JSON object. It is the byte payload a parent driver serves for an expandable kubernetes child and the payload a newly mounted kubernetes_driver requests from its parent. JSON is used instead of a delimited path because context names may contain any character, including slashes (for example EKS context ARNs).

Git mount state:

repository: string. Absolute top-level path of the working tree.

view: string. One of repository, commits, commit, committed, uncommitted, staged, unstaged, untracked, or conflicted.

revision: string, optional. The full 40- or 64-character hexadecimal object id required by the commit view.

The initial payload from a directory driver is the absolute repository path. Deeper Git children carry the mount state as JSON. A mount state is limited to 16384 bytes and its repository path is limited to 4096 bytes with no NUL.

GitHub mount state:

view: string. One of root, authored, review-requested, pull-request, diff, actions, runs, run, or run-actions.

url: string, required by pull-request, diff, and actions. A non-empty HTTPS pull request URL with a host and path, no whitespace, and at most 4096 bytes.

repository: string, required by runs, run, and run-actions. An OWNER/REPOSITORY name containing only letters, numbers, dot, underscore, and hyphen.

branch: string, required by runs. The non-empty pull request head branch used to filter workflow runs.

state: string. For actions, one of OPEN, CLOSED, or MERGED. For run-actions, one of queued, in_progress, or completed. It determines which controls are emitted.

isDraft: boolean, used only by actions. A draft pull request omits merge.

runId: positive integer, required by run and run-actions. The GitHub Actions database identifier.

The root_driver serves {} for the virtual github entry. Deeper GitHub children carry the mount state as JSON. A mount state is limited to 16384 bytes.

AWS mount state:

view: string. One of root, profile, regions, region, search, types, type, resource, resource-actions, config-actions, services, service-query, service-resource, or service-actions.

profile: string. A validated local AWS CLI profile name, required below root.

region: string. A validated AWS Region identifier, required by cloud-backed views.

typeName: string. A CloudFormation resource type in Vendor::Service::Type form, required by type and resource views.

identifier: string. The Cloud Control primary identifier, required by resource and resource-actions.

operation: string. An identifier from the compiled service-operation catalog. Service query views require an allowlisted operation; detailed service resources and actions require an operation marked as detailed plus its validated identifier.

AWS action targets contain the profile and, for cloud resources, region plus either typeName and identifier or service operation and identifier. Configuration targets contain profile and exactly one setting, region or output. The target is stored behind the opaque action handle; the browser supplies only that handle and the action body.

System-profiler mount state:

apiVersion: string. Must be datatree.local/system-profiler/v1 so ordinary JSON cannot be mistaken for a local-machine mount state.

profile: string. Either hardware or usb. It selects both the exact system_profiler data type and the resource kind used for descendants.

view: string. Either root, which runs the allowlisted local query, or node, which browses already captured JSON without running another command.

node: JSON value, required only by node. It contains one object or array from the bounded system_profiler result. The encoded mount state may contain at most the configured output limit plus 4096 bytes of metadata.

GitHub action target:

url: string, present for pull request actions.

repository: string and runId: positive integer, present for workflow actions.

The action target is stored behind the action resource's opaque host handle. The browser never supplies a target URL, repository, run identifier, command, or arbitrary flag to the action endpoint.

Configuration:

DATATREE_ADDR: optional listen address override.

DATATREE_KUBECTL: optional kubectl executable override, used by root_driver to decide whether to offer the kubernetes entry and by kubernetes_driver for every kubectl invocation.

DATATREE_KUBECONFIG: optional kubeconfig path or platform path list. kubernetes_driver passes it to kubectl as KUBECONFIG. Empty keeps kubectl's normal kubeconfig selection.

DATATREE_KUBERNETES_MAX_RESULTS: non-negative maximum unique entries retained from each context, namespace, resource-type, or object listing. The default is 0, which retains every entry. A positive value enables a limit; a larger result is sorted, cut to the first configured number, and accompanied by an error leaf explaining the exact total and limit.

DATATREE_KUBERNETES_MAX_OUTPUT_BYTES: positive maximum bytes retained from each kubectl stdout stream and stderr stream. The default is 8388608. Exceeding it produces an error leaf with the observed and allowed byte counts.

DATATREE_GH: optional GitHub CLI executable override, used by root_driver to decide whether to offer the github entry and by github_driver for every gh invocation. Empty uses gh found on PATH.

DATATREE_GITHUB_MAX_OUTPUT_BYTES: positive maximum bytes retained from each GitHub CLI stdout stream and stderr stream. The default is 8388608. Exceeding it produces an error leaf with the observed and allowed byte counts.

DATATREE_AWS: optional AWS CLI executable override, used by root_driver to decide whether to offer aws and by aws_driver for every invocation. Empty uses aws found on PATH.

DATATREE_AWS_MAX_OUTPUT_BYTES: positive maximum bytes retained from each AWS CLI stdout stream and stderr stream. The default is 8388608. Exceeding it produces an error with the command, task, observed bytes, and configured bound.

DATATREE_SYSTEM_PROFILER: optional macOS system_profiler executable override. The default is /usr/sbin/system_profiler. root_driver offers hardware and usb only when this executable resolves, and system_profiler_driver uses it for every local-machine query.

DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES: positive maximum bytes retained from each system_profiler stdout stream and stderr stream. The default is 8388608. Exceeding it produces a visible error with the command, observed bytes, and configured bound.

DATATREE_SYSTEM_PROFILER_TIMEOUT: positive duration allowed for each system_profiler command. The default is 30s. A command that does not finish within the configured duration is terminated and produces a visible error naming the command, task, expected completion time, and configured setting.

DATATREE_GIT: optional Git executable override. Empty uses git found on PATH.

DATATREE_GIT_MAX_RESULTS: positive maximum rows retained in each commit or file-state view. The default is 500. A larger result is cut to the first configured number and accompanied by an error leaf explaining the exact limit.

DATATREE_GIT_MAX_OUTPUT_BYTES: positive maximum bytes retained from each Git stdout stream and stderr stream, and the maximum working-tree file size served by the Git driver. The default is 8388608. Exceeding it produces a complete error identifying the command or file, observed size, and configured bound.

Default listen address: 127.0.0.1:0, using the saved loopback port on restart when available; otherwise port 0 requests an available local port from the operating system.

Driver process:

cmd: process handle.

enc: JSON encoder for driver stdin.

dec: JSON decoder for driver stdout.

stdin: writable driver stdin stream.

stdout: readable driver stdout stream.

mu: per-driver request lock. It serializes range requests so concurrent HTTP calls cannot interleave JSON messages on one driver process.

5. Resource Kinds

directory maps to directory_driver.

git maps to git_driver.

github maps to github_driver.

aws maps to aws_driver.

hardware and usb map to system_profiler_driver.

diff maps to diff_driver.

diff-meta, diff-hunk, diff-context, diff-delete, and diff-add are presentation leaves and do not map to drivers.

github-action-close, github-action-reopen, github-action-merge, github-action-comment, github-action-approve, github-action-review-comment, github-action-request-changes, github-action-run-rerun, github-action-run-rerun-failed, and github-action-run-cancel are action leaves. They do not map to drivers and do not offer downloads.

archive and zip map to zip_driver.

kubernetes maps to kubernetes_driver.

json maps to json_driver.

k8s-pod maps to k8s_pod_driver.

k8s-workload maps to k8s_workload_driver.

k8s-service maps to k8s_service_driver.

k8s-configmap maps to k8s_configmap_driver.

k8s-node maps to k8s_node_driver.

image maps to image_driver.

pdf maps to pdf_driver.

html maps to html_driver.

csv maps to csv_driver.

text and file map to text_driver.

url, kv, and error are leaf kinds and do not map to another driver.

Unknown kinds map to text_driver.

6. Driver Behavior

6.1 root_driver

root_driver reads the directory supplied by the host. When started without a directory argument, it emits a filesystem location browser instead of reading the current working directory.

The location browser always emits HOME and / as directory resources. HOME serves the absolute home-directory path and / serves the filesystem-root path. root_driver also checks /Volumes, /mnt, and /media. Every readable, non-hidden directory immediately below one of those roots becomes a directory resource. Entries below /Volumes are named VOLUME: NAME. Entries below /mnt and /media are named MOUNT: ABSOLUTE_PATH. Missing roots, unreadable entries, hidden entries, non-directories, and duplicate paths are ignored. Expanding any location serves its exact absolute path to directory_driver.

It skips hidden entries whose names start with ".".

It emits one resource per visible entry.

Directories are kind directory and size 0.

Directories containing a .git directory or file are kind git. Other directories are kind directory. When the listed directory is inside a Git work tree, root_driver emits a first resource named . with kind git and size 0. This virtual row serves the selected directory path as bytes and lets git_driver resolve the containing repository top level.

Files are classified by extension:

.txt, .md, .log -> text.

.zip, .tar, .gz -> archive.

.jpg, .jpeg, .png, .gif, .bmp -> image.

.pdf -> pdf.

.html, .htm -> html.

.json -> json.

.csv -> csv.

Other files -> file.

When a kubectl executable can be resolved (the DATATREE_KUBECTL override when set, otherwise kubectl on the PATH), root_driver prepends a virtual resource named kubernetes with kind kubernetes and size 0 to the listing. When no kubectl can be resolved, the entry is omitted.

The kubernetes resource carries an opaque reserved routing key. A range request for that key returns the two bytes {} — the kubernetes driver's root mount state — without touching the filesystem. A real selected-directory child named kubernetes keeps its filename as its routing key and remains independently browseable.

When a GitHub CLI executable can be resolved (the DATATREE_GH override when set, otherwise gh on the PATH), root_driver prepends a virtual resource named github with kind github and size 0 to the listing. When no gh executable can be resolved, the entry is omitted.

The github resource carries an opaque reserved routing key. A range request for that key returns the two bytes {} — the GitHub driver's root mount state — without touching the filesystem. A real selected-directory child named github remains independently browseable.

When an AWS CLI executable can be resolved (the DATATREE_AWS override when set, otherwise aws on the PATH), root_driver prepends a virtual resource named aws with kind aws and size 0. Expanding its opaque reserved routing key serves {} as the AWS root mount state. A real selected-directory child named aws remains independently browseable. When no AWS CLI can be resolved, the entry is omitted.

In both location-browser and selected-directory modes, when the DATATREE_SYSTEM_PROFILER executable, or /usr/sbin/system_profiler by default, can be resolved, root_driver prepends hardware and usb resources with their matching kinds and size 0. Their opaque reserved routing keys return versioned JSON root mount states for system_profiler_driver. Real selected-directory children named hardware or usb use their filenames as routing keys and remain ordinary browseable filesystem entries. When the executable cannot be resolved, both virtual entries are omitted.

When root_driver receives a range request for a directory child, it returns the absolute directory path as bytes. When it receives a range request for a file child, it returns the requested file bytes.

In selected-directory mode, root_driver verifies that each requested child resolves inside the selected directory using path-segment-aware relative path checks. In location-browser mode, it serves only exact names from the location map built during initialization.

6.2 directory_driver

directory_driver requests all upstream bytes during initialization. The bytes are interpreted as an absolute directory path.

It verifies that the path exists and is a directory.

It lists non-hidden entries in that directory and classifies them using the same kind rules as root_driver, including Git repository detection.

When directory_driver receives a range request for a directory child, it returns the absolute directory path as bytes. When it receives a range request for a file child, it returns the requested file bytes.

directory_driver verifies that each requested child resolves inside the mounted directory using path-segment-aware relative path checks.

6.3 zip_driver

zip_driver requests all upstream bytes during initialization.

It parses the bytes as a ZIP archive.

It skips ZIP directory placeholder entries because directory_driver expects real filesystem paths.

It emits one resource per file entry. Entry names are preserved, including slash-separated paths inside the archive.

It classifies each entry by extension using the same file kind rules as root_driver, so nested .zip entries are expandable as archive resources.

It keeps the entry bytes in memory and stays alive to serve range requests for child entries.

6.4 text_driver

text_driver requests all upstream bytes during initialization.

It scans the decoded text for URLs matching the regular expression:

```
https?://[^\s'">]+
```

It emits one url resource per match. The resource name is the actual URL and the size is the URL length in bytes.

text_driver retains its input source and extracted URL bytes for further filtering and stays alive serving their ranges.

6.5 kubernetes_driver

kubernetes_driver browses kubernetes clusters through the kubectl command line tool. It is strictly read-only: every kubectl invocation passes through a single guard that only permits the subcommands get, api-resources, and config get-contexts (and nothing else under config). Any other invocation is refused before a process is spawned, and the driver aborts. kubectl is always executed directly with an argument vector, never through a shell.

kubernetes_driver requests all upstream bytes during initialization and interprets them as a kubernetes mount state (see section 4). Empty bytes and {} both mean the root level. A mount state is limited to 16384 bytes, and each context, namespace, type, and configured kubeconfig selection is limited to 4096 bytes and cannot contain a NUL byte. A state that exceeds a bound, fails to parse, or combines fields illegally produces a single error resource.

The hierarchy has five levels. Each level lists its children with one kubectl invocation:

Root: kubectl config get-contexts -o name. One resource of kind kubernetes per context. This reads only the selected local kubeconfig and contacts no cluster.

Context: kubectl get --context=NAME namespaces -o name. One resource of kind kubernetes per namespace, plus a first entry named [cluster-scoped] of kind kubernetes for cluster-scoped resource types. The bracket characters are never legal in a namespace name, so the entry cannot collide with a real namespace.

Namespace: kubectl api-resources --context=NAME --verbs=list --namespaced=true -o name. One resource of kind kubernetes per listable resource type.

Cluster scope: the same invocation with --namespaced=false.

Resource type: kubectl get --context=NAME [--namespace=NS] TYPE -o name. One readable resource of unknown size (negative one) per object, named by the object name after the slash. The object's kind is chosen by the resource type, ignoring any API group suffix (so "deployments.apps" matches "deployments"): pods -> k8s-pod; deployments, statefulsets, daemonsets, replicasets, jobs, and cronjobs -> k8s-workload; services -> k8s-service; configmaps and secrets -> k8s-configmap; nodes -> k8s-node; every other type -> json. Expanding an object therefore mounts the matching kubernetes object driver (section 6.7), or json_driver for types without one.

Every listing is deduplicated and sorted by name before it is exposed, so repeated kubectl output and Go map ordering cannot change the tree. By default every unique entry is retained. When DATATREE_KUBERNETES_MAX_RESULTS is positive, at most that many normal entries are retained; if more unique entries exist, an additional error leaf reports that the deterministic prefix is truncated and explains which setting changes the bound.

When kubernetes_driver receives a range request for a context, namespace, [cluster-scoped], or resource type child, it returns that child's mount state as bytes, so the next kubernetes_driver instance mounts one level deeper. When it receives a range request for an object child, it runs kubectl get --context=NAME [--namespace=NS] TYPE OBJECT -o json and returns the requested slice of the manifest bytes, honoring offset and length with -1 meaning "through the end". A failed manifest read returns a reserved driver-error JSON payload; json_driver and all five curated kubernetes object drivers recognize it and emit the complete kubectl failure as one error leaf instead of presenting empty data.

Context and namespace values are always passed as single --flag=value arguments, so a value beginning with a dash cannot be misread as a flag. Resource type and object names are passed as positional arguments and are rejected if they are empty, begin with a dash, contain whitespace, or exceed the selection bound.

kubectl stdout and stderr are retained only up to DATATREE_KUBERNETES_MAX_OUTPUT_BYTES; the driver continues reading and discarding excess bytes until kubectl exits, then reports the exact stream size and configured bound. When api-resources exits non-zero but produced bounded output, the driver keeps the partial type list because one unavailable API group must not hide every available group. Every other non-zero invocation is an error even if it wrote stdout, so a partial object manifest can never be presented as successful JSON. When kubectl fails, kubectl cannot be resolved, or either stream exceeds its bound, the driver emits a single resource of kind error whose name is the complete failure message, then exits.

kubernetes_driver stays alive after emitting resources and serves child ranges.

6.6 json_driver

json_driver browses a JSON document as hierarchical data. It requests all upstream bytes during initialization and classifies the document from its first non-space byte as an object, an array, or a scalar.

For an object, it emits one resource per key, sorted by key name so the tree is stable across expansions. For an array, it emits one resource per element named [0], [1], and so on. For each key or element whose value is itself an object or array, the resource has kind json and is expandable; expanding it mounts another json_driver on that value. For a scalar value, the resource has the leaf kind kv and its name is "key: value" (a JSON string is shown unquoted, except that an empty string keeps its quotes so the row stays visible; other scalars show their literal token). A document that is itself a bare valid scalar is shown as a single kv resource.

json_driver keeps each expandable child's raw JSON bytes in memory and stays alive to serve range requests for them, honoring offset and length with -1 meaning "through the end". An empty document produces no resources. A document that does not parse as JSON produces a single error resource.

json_driver never contacts the network or runs kubectl; it only parses bytes handed down by its parent driver. It parses nested values as raw JSON byte slices rather than dynamically typed values, so it browses arbitrary JSON without an escape type.

6.7 kubernetes object drivers

Five drivers present well-known kubernetes object manifests as curated views instead of raw JSON: k8s_pod_driver, k8s_workload_driver, k8s_service_driver, k8s_configmap_driver, and k8s_node_driver. They share one contract:

Each requests all upstream bytes during initialization and parses them as the object's JSON manifest (what kubernetes_driver serves from kubectl get -o json). A manifest that does not parse produces a single error resource.

Each emits the type's important scalars as kv leaves named "key: value", and expandable kind json children for nested structure. Section children use bracketed names such as [labels] and [manifest], which cannot collide with kubernetes identifiers or data keys.

Each always emits a [manifest] child carrying the complete manifest, so every field remains reachable through json_driver even when the curated view does not summarize it.

Each stays alive serving child ranges from memory, honoring offset and length with -1 meaning "through the end", and never runs kubectl or touches the network.

k8s_pod_driver (kind k8s-pod) shows phase, node, podIP, serviceAccount, started, and the summed restarts; one expandable child per spec container named "container NAME: IMAGE" (with "(ready R, restarts N)" appended when the container status is known) serving that container's raw JSON; and [labels], [conditions], [volumes], and [manifest] children.

k8s_workload_driver (kind k8s-workload, used for Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs, and CronJobs) shows the manifest kind; whichever replica counters the manifest has as desired, current, ready, available, updated, active, succeeded, and failed; the rollout strategy from spec.strategy.type or spec.updateStrategy.type; serviceName, schedule, suspend, and lastSchedule when present; one expandable child per template container (found at spec.template.spec.containers, or spec.jobTemplate.spec.template.spec.containers for CronJobs); and [selector], [conditions], [labels], and [manifest] children.

k8s_service_driver (kind k8s-service) shows the service type (defaulting to ClusterIP), clusterIP, each externalIP, each load balancer ingress address, one "port NAME: PORT/PROTOCOL -> TARGET" leaf per declared port (with the node port appended when present, and the port number naming ports that have no name), and one "selector KEY: VALUE" leaf per selector pair in sorted order; plus [ports], [labels], and [manifest] children.

k8s_configmap_driver (kind k8s-configmap, used for ConfigMaps and Secrets) shows the manifest kind, the secret type, and the immutable flag when present, then emits one child per data entry in sorted key order whose bytes are the entry's value: ConfigMap data values are used as-is, ConfigMap binaryData and Secret data values are base64-decoded first, and each entry is classified by filename using the disk-file rules, with text fallback for unknown filenames containing valid UTF-8, so config files are readable and downloadable directly. An entry that fails to base64-decode becomes an error leaf naming the key and the decode error. Whether secret values are visible at all is decided by the user's kubectl credentials; the driver only reads.

k8s_node_driver (kind k8s-node) shows kubeletVersion, osImage, operatingSystem, architecture, containerRuntime, kernel, podCIDR, and unschedulable (only when true); one "address TYPE: ADDRESS" leaf per address; "capacity KEY: VALUE" and "allocatable KEY: VALUE" leaves in sorted key order; one "condition TYPE: STATUS" leaf per condition; one "taint KEY[=VALUE]: EFFECT" leaf per taint; and [labels] and [manifest] children.

6.8 File preview drivers

image_driver requests all upstream bytes and reads only the image header. PNG, JPEG, GIF, and BMP inputs produce format, width, and height kv leaves. An empty input retains its zero-byte source resource. Unrecognized bytes produce one complete error resource. The driver does not decode full pixel data, contact the network, or modify the file.

pdf_driver requests all upstream bytes and requires a PDF version header within the first 1024 bytes. It emits the version, emits a page count only when uncompressed page objects can be counted reliably, and emits each unique absolute HTTP or HTTPS link found in a URI action. It omits a page count when compressed object streams prevent a reliable count. Invalid input produces a complete error resource and retains the original source bytes.

html_driver requests all upstream bytes and emits the first page title as a kv leaf after entity decoding and whitespace normalization. It emits each unique absolute HTTP or HTTPS href or src target as a url leaf in document order. It never loads or executes the document.

csv_driver requests all upstream bytes and treats the first record as column headings. It emits row and column counts, an expandable [headers] JSON child, and one expandable JSON child per data row. Empty headings receive stable "column N" names and duplicate headings receive numeric suffixes so cells are not lost. Ragged rows retain every present cell. Invalid CSV produces a complete error resource and retains the original source bytes.

6.9 Git repository driver

git_driver requests an absolute repository path during its first mount and JSON Git mount states on deeper mounts. It resolves the path with git rev-parse --show-toplevel and rejects bare repositories or paths that are not working trees. DATATREE_GIT selects the executable, defaulting to git on PATH.

Every Git invocation runs the executable directly with an argument vector, never through a shell. The complete command allowlist is rev-parse, symbolic-ref, status, log, show, ls-tree, and diff-tree. The driver passes --no-pager, --no-optional-locks, core.fsmonitor=false, and GIT_OPTIONAL_LOCKS=0. No write-capable Git subcommand is present.

The repository view shows the absolute top-level path, branch name or detached state, the short HEAD object id and full subject, or an explicit unborn message when the repository has no commits. It also shows the active row and output limits with the environment variable names used to change them. It exposes [commits], [committed files at HEAD], [all uncommitted changes], [staged changes], [unstaged changes], [untracked files], and [conflicted files] as expandable Git children.

The commits view lists newest first. Each row includes the short object id, ISO date, complete author name, and complete subject. At most DATATREE_GIT_MAX_RESULTS rows are shown. Expanding a commit shows its full object id, parent object ids or root-commit status, author name and email, ISO date, full subject, and each changed path with its added, modified, deleted, copied, renamed, type-changed, or unmerged status. Non-deleted files serve the bytes recorded by that exact commit.

The committed-files view lists every blob reachable at HEAD recursively, with its Git object size and normal file kind, and serves the bytes from HEAD rather than the working tree. Submodule entries are informational leaves. An unborn repository returns an empty commit list and committed-file list.

The all-uncommitted, staged, unstaged, untracked, and conflicted views use git status --porcelain=v1 -z --untracked-files=all --no-renames. The all-uncommitted view includes each changed path once and names every applicable state, such as "staged modified, unstaged modified". A file with both staged and unstaged changes also appears in both applicable specific views. Staged rows serve index bytes. All-uncommitted, unstaged, untracked, and conflicted rows serve current working-tree bytes when the path exists, falling back to index bytes for a staged path absent from the working tree. Symbolic links serve their link text rather than following the link outside the repository. Deleted paths remain visible as informational leaves.

All Git output is NUL-delimited where paths are present, so spaces, tabs, newlines, quotes, and non-ASCII characters in file names remain unambiguous. Result limits append an explanatory error leaf. Output, configuration, parsing, repository validation, and file-read failures become complete visible errors and include the repository, operation, received result, expected result, and configured bound when relevant.

6.10 GitHub pull request driver

github_driver requests a JSON mount state from its parent. The root view emits [open pull requests I authored] and [open pull requests requesting my review] as github children. Their mount states select authored or review-requested views.

The authored view runs gh search prs with --author=@me, --state=open, a 1000-result search window, and JSON fields isDraft, number, repository, state, title, updatedAt, and url. The review-requested view runs the same command with --review-requested=@me. Results are validated, sorted by repository and number, and shown as expandable github rows named "OWNER/REPOSITORY #NUMBER [draft] TITLE". If GitHub returns the complete 1000-result search window, the list appends an error leaf warning that additional server-side results may be unavailable rather than silently implying completeness.

Expanding one pull request runs gh pr view on its validated HTTPS URL and requests additions, assignees, author, base and head branches, body, changed-file counts, comments, commits, timestamps, deletions, files, draft state, labels, reviews and review requests, merge state, milestone, number, checks, state, title, and URL. github_driver prepends expandable [actions], [CI/CD runs], and [diff] children, then passes the returned JSON through the generic JSON listing logic, so every object and array is recursively expandable and every scalar is visible.

The pull request actions view always offers Add comment. An open pull request also offers Close pull request, Approve pull request, Submit review comment, and Request changes; it offers Merge pull request only when not a draft. A closed pull request offers Reopen pull request. A merged pull request has no state-changing action. The command builder permits only gh pr close URL; gh pr reopen URL; gh pr merge URL with exactly one of --merge, --squash, or --rebase; gh pr comment URL --body BODY; and gh pr review URL with exactly --approve, --comment --body BODY, or --request-changes --body BODY. It never permits --admin, --auto, --delete-branch, --web, editor input, arbitrary flags, or shell execution.

The CI/CD runs view runs gh run list with the validated repository and head branch, a 100-run limit, and fixed JSON fields. Runs are sorted by descending database identifier and named with workflow, run number, later attempt number when applicable, status, conclusion, and display title. Expanding a run executes gh run view for its positive database identifier and validated repository with fixed JSON fields including jobs. A queued or running run offers Cancel workflow run. A completed run offers Rerun all jobs and Rerun failed jobs. The action command gate permits only gh run cancel RUN_ID --repo OWNER/REPOSITORY, gh run rerun RUN_ID --repo OWNER/REPOSITORY, and that rerun command with --failed. It does not permit force cancellation, debug reruns, arbitrary jobs, browser opening, or repository values outside the validated target.

Expanding [diff] runs the exact read-only command gh pr diff URL --patch --color=never. github_driver splits the unified patch at diff --git file boundaries, derives each changed path from its destination, rename destination, source, or rename source header in that order, and emits one expandable diff resource per changed file. Because patch output may contain one section per commit, later sections for the same derived file name are appended to that file's first resource in command-output order. diff_driver parses the combined file patch. It emits metadata rows for file headers and binary or rename facts, hunk rows for @@ headers, and numbered context, deletion, and addition rows. Context rows advance both old and new line numbers, deletion rows advance only the old line, addition rows advance only the new line, and the no-newline marker advances neither.

The GitHub CLI is executed directly with an argument vector, never through a shell. The browsing command gate permits only the exact authored search, review-requested search, JSON pr view, patch pr diff, workflow run list, and workflow run view argument sequences generated by the driver. The separate action gate permits only the exact commands described above. Both refuse every other invocation before a process starts. stdout and stderr are bounded by DATATREE_GITHUB_MAX_OUTPUT_BYTES. Action errors describe the attempted operation without including comment or review bodies in command descriptions. Missing authentication, API failures, invalid output, invalid mount state, and output-limit failures become complete visible errors.

6.11 AWS driver

aws_driver starts at the local profile list from the exact command aws configure list-profiles. Profile names are parsed one line at a time so spaces remain intact. Expanding a profile reads only non-secret configuration keys through exact aws configure get commands. It never reads credential values. Set default region and Set output format use exact aws configure set commands and refuse every other setting, including access-key, secret-key, and session-token keys.

The regions view uses aws ec2 describe-regions --all-regions with the profile's configured region, or us-east-1 as the bootstrap endpoint when no valid region is configured. Each region exposes the service catalog, Resource Explorer, and Cloud Control branches. Resource Explorer runs search with query string *, a 1000-item CLI maximum, the selected profile and Region, JSON output, and no pager. Search coverage depends on Resource Explorer being enabled, indexed, and visible through the Region's default view; its service limit and permissions may omit resources.

The Cloud Control type view uses CloudFormation list-types for public AWS resource types and omits NON_PROVISIONABLE entries. Expanding a type runs Cloud Control list-resources with a 1000-item CLI maximum. Expanding a resource runs get-resource and exposes its validated JSON properties. Cloud Control covers supported resource types only; Resource Explorer results whose types do not support Cloud Control remain browseable but do not receive generic edit controls. Service-specific settings outside these standardized APIs are not falsely presented as universally editable.

The service catalog maps each visible row to a compiled operation identifier and an exact read-only AWS CLI argument prefix. It covers EC2 instances, security groups, VPCs, subnets, volumes, self-owned snapshots and images, elastic IPs, network interfaces, route tables, and key pairs; S3 buckets; IAM authorization details, summary, users, groups, roles, and customer-managed policies; Access Analyzer analyzers; up to 100 Security Hub findings; GuardDuty detectors; CloudTrail trails; AWS Config recorders and rules; Lambda functions; RDS instances and clusters; ECS and EKS clusters; DynamoDB tables; SQS queues; SNS topics; CloudFormation stacks; CloudWatch log groups and alarms; API Gateway REST APIs; ElastiCache clusters; ELBv2 load balancers; Route 53 hosted zones; Secrets Manager metadata without secret values; KMS key identifiers; ECR repositories; Backup vaults; Step Functions state machines; EventBridge rules; Auto Scaling groups; Certificate Manager certificates; regional WAF web ACLs; Organizations accounts; and Systems Manager managed nodes. Most results are emitted as one JSON response and recursively traversed by json_driver. EC2 instance and security-group, S3 bucket, IAM role, and IAM policy lists are parsed into named resources; expanding one issues only its exact describe or get command and shows the complete JSON details.

EC2 instance actions permit only start-instances, stop-instances, reboot-instances, and terminate-instances for the one validated instance identifier. Security-group actions permit only authorize-security-group-ingress and revoke-security-group-ingress with a non-empty JSON IP-permissions array. S3 actions permit only replacing or deleting one bucket policy and enabling or suspending versioning. IAM role actions permit only replacing the assume-role trust policy. IAM customer-managed policy actions create a new policy version from a complete JSON document and set it as default; AWS may reject this when the managed policy already has five versions, and Datatree does not delete an old version automatically. These actions do not provide generic resource deletion, bucket deletion, object deletion, secret retrieval, key administration, IAM credential management, or arbitrary service invocation.

Create sends a non-empty desired-state JSON object. Update sends a non-empty RFC 6902 patch array. Delete sends no body. Each Cloud Control action includes a fresh 128-bit hexadecimal idempotency token, selected profile and Region, JSON output, and no pager. The action command gate permits only exact create-resource, update-resource, and delete-resource shapes generated from validated opaque targets. Cloud Control operations are independent and asynchronous, do not automatically roll back partial failures, and should not be used to update a resource actively managed by CloudFormation or another controller.

The AWS CLI is executed directly with argument vectors, never through a shell. Read and action gates reject arbitrary services, endpoints, SSL bypass, unsigned calls, recursive deletion flags, credential writes, and unexpected arguments. Output is bounded by DATATREE_AWS_MAX_OUTPUT_BYTES. Authentication, authorization, Region, Resource Explorer setup, unsupported-type, malformed JSON, and output-limit errors remain visible with the attempted task and expected result.

6.12 Local hardware and USB driver

system_profiler_driver requests a versioned mount state from its parent. A hardware root runs exactly system_profiler SPHardwareDataType -json. Its usual single hardware-overview object is flattened so model name, model identifier, chip, processor layout, physical memory, firmware, activation-lock status, serial number, platform UUID, and other returned properties appear directly as kv leaves.

A usb root first runs exactly system_profiler -listDataTypes. It prefers SPUSBHostDataType on current macOS and falls back to SPUSBDataType when that is the available legacy type, then runs exactly system_profiler TYPE -json. The returned bus array is shown in command order. Each object's _name becomes its row name. Nested _items arrays are flattened into named hub and device children, while other nested objects and arrays remain expandable. Duplicate names are suffixed [2], [3], and so on within their parent. Field names are made readable without changing their values; common hardware and USB fields have concise labels.

Each expandable child carries a versioned node mount state containing only that captured JSON value. Expanding it parses those bytes and does not rerun system_profiler. Invalid mount states, invalid JSON, an invalid _items type, an unavailable USB data type, command failures, output-limit failures, and command timeouts become complete visible error rows. The command gate permits only -listDataTypes and JSON reads of SPHardwareDataType, SPUSBHostDataType, or SPUSBDataType. The executable is invoked directly without a shell, must finish within DATATREE_SYSTEM_PROFILER_TIMEOUT, never contacts a network service, and has no action kinds.

7. Host Algorithms

7.1 Load root driver

```
start root_driver
store it as session "root_driver"
read driver stdout until a resources message is received
for each resource:
    create a host handle owned by "root_driver"
    use resource.key as child_name when present, otherwise resource.name
    add handle to the HTTP resource object
store the resulting list as rootResources
```

7.2 Load driver for child handle

```
look up the child handle
choose driver from explicit driver name or kind mapping
start the driver as session "<driver>_<handle>"
while reading driver stdout:
    if driver asks for a range:
        send range request to the parent handle's owning driver
        return bytes to the new driver
    if driver emits resources:
        create handles owned by the new session
        return resources, driver, and session
```

7.3 Load driver for uploaded blob

```
look up uploaded blob bytes by res_id
reserve the blob for session "<driver>_<res_id>" so it cannot be evicted
start the driver as session "<driver>_<res_id>"
while reading driver stdout:
    if driver asks for a range:
        return the requested slice of the uploaded bytes
    if driver emits resources:
        create handles owned by the new session
        return resources, driver, and session
```

When a driver session stops, remove every handle it owns. If it reserved an uploaded blob, remove that blob and reduce the retained-byte count. Closing the driver's input is the shutdown request; the host waits for the driver to exit after it receives end-of-input.

7.4 Serve data

```
look up handle
send {cmd:"range", offset, length, child:handle.child_name} to handle.driver_name
read one range response
base64-decode data
return decoded bytes
```

8. HTTP API

GET /

Returns web/index.html.

GET /api/drivers

Returns a JSON array containing only drivers that can meaningfully inspect arbitrary uploaded file bytes: diff_driver, zip_driver, text_driver, json_driver, image_driver, pdf_driver, html_driver, and csv_driver. Internal root, filesystem, GitHub, AWS, Kubernetes, and system-profiler drivers are omitted because they require paths or validated mount states.

GET /api/upload-settings

Returns the active max_upload_bytes, max_retained_upload_bytes, upload_idle_timeout, and an explanation of eviction and unload behavior. The browser uses this response for its upload-limit readout.

GET /api/list

Returns:

```
{ "resources": [ Resource, ... ] }
```

POST /api/upload

Request body is raw bytes.

The host reads at most DATATREE_MAX_UPLOAD_BYTES plus one byte. A body over the per-file limit returns HTTP 413. If the retained-memory limit cannot make room without removing an active upload, the request returns HTTP 507. Otherwise the host evicts least-recently-used inactive uploads until the new body fits.

Returns:

```
{ "res_id": "res-1", "size": 1234 }
```

POST /api/load

Request:

```
{ "res_id": "res-1", "driver": "zip_driver" }
```

Returns:

```
{ "resources": [ Resource, ... ], "driver": "zip_driver", "session": "zip_driver_res-1" }
```

POST /api/autoLoad

Request:

```
{ "handle": "hdl-1", "kind": "archive" }
```

Returns:

```
{ "resources": [ Resource, ... ], "driver": "zip_driver", "session": "zip_driver_hdl-1" }
```

For leaf kinds, returns an empty resources array, driver "leaf", and an empty session.

POST /api/loadChild

Request:

```
{ "handle": "hdl-1", "kind": "archive", "driver": "" }
```

driver is optional. If present, it is used directly. If empty, kind mapping is used. If kind is empty too, the host uses the stored kind for the handle.

Returns the same shape as /api/autoLoad.

POST /api/unload

Request:

```
{ "session": "zip_driver_hdl-1" }
```

Returns:

```
{ "session": "zip_driver_hdl-1", "stopped": true }
```

For an uploaded-blob session, a successful unload also removes that blob and all handles owned by the session.

POST /api/github/action

The request must have an Origin header exactly matching http:// plus the request Host. Cross-origin requests receive HTTP 403 before handle lookup. The JSON request is bounded to 71680 bytes and contains:

```
{ "handle": "hdl-1", "body": "optional comment or review text", "merge_method": "optional merge, squash, or rebase" }
```

The handle must exist and have a github-action-* kind. The host reads the validated action target through that handle, dispatches only the operation represented by the stored kind, and never accepts target or command fields from the browser. body is accepted only by comment, review-comment, and request-changes actions and is limited to 65536 bytes. merge_method is accepted only by merge and defaults to squash. A successful response is:

```
{ "message": "GitHub action response" }
```

Malformed, unsupported, stale-handle, target-validation, and gh failures return HTTP 400 with a complete explanation.

POST /api/aws/action

The request must have an Origin header exactly matching http:// plus the request Host. The JSON body is bounded to 276480 bytes and contains an opaque aws-action-* handle plus an optional body of at most 262144 bytes. The host reads the validated target from the handle. It accepts a body only as a desired-state object, JSON patch array, Region, output format, IP-permissions array, policy object, or versioning state according to the stored action kind. A successful response contains message. Malformed, unsupported, stale-handle, target-validation, configuration, and AWS CLI failures return HTTP 400.

GET /api/data?handle=HID&offset=O&length=L

Returns raw bytes for a resource handle.

If offset is absent, it defaults to 0.

If length is absent, it defaults to -1.

9. Files On Disk

host.go contains the HTTP server, store, driver process management, and API handlers.

logs.go contains host log-message functions added for installed-layout verification.

host_selftest.go contains the host -test integration self-test helpers and workflow.

git_selftest.go contains the real-repository integration fixture and Git browsing checks used by host -test.

github_selftest.go contains the fake GitHub CLI fixture, nested pull request, CI, human-readable diff, comment, rerun, and command-audit checks used by host -test.

host_github_action_test.go verifies that the GitHub mutation endpoint rejects cross-origin requests and reaches handle validation for same-origin requests.

host_aws_action_test.go verifies that the AWS mutation endpoint rejects cross-origin requests and reaches handle validation for same-origin requests.

drivers_go/root_driver/root_driver.go contains the selected-directory and location-browser driver protocol.

drivers_go/root_driver/locations.go contains HOME, filesystem-root, and mounted-location discovery.

drivers_go/system_profiler_driver/system_profiler_driver.go contains executable resolution, the exact read-only command gate, bounded capture, mount-state validation, and hierarchical local hardware and USB presentation.

drivers_go/system_profiler_driver/system_profiler_driver_test.go verifies hardware facts, nested and duplicate USB devices, and rejection of a non-hardware system_profiler data type.

drivers_go/directory_driver.go contains the filesystem directory driver source.

drivers_go/git_driver/git_driver.go contains Git hierarchy presentation, mount-state validation, and file serving.

drivers_go/git_driver/git_command.go contains the read-only Git command gate, configuration, bounded output, and Git output parsing.

drivers_go/git_driver/logs.go contains the Git driver's diagnostic message functions.

drivers_go/git_driver/git_command_test.go verifies conflict labels and working-state classification.

drivers_go/github_driver/github_driver.go contains GitHub hierarchy presentation, mount-state validation, pull request parsing, and generic JSON detail browsing.

drivers_go/github_driver/github_command.go contains GitHub CLI resolution, the exact read-only command gate, bounded output, and command execution.

drivers_go/github_driver/github_views.go contains pull request action presentation, CI run hierarchy presentation, repository validation, and opaque action targets.

drivers_go/github_driver/github_action.go contains action argument generation, body and merge-method validation, the exact mutation command gate, and bounded action execution.

drivers_go/github_driver/github_driver_test.go verifies GitHub mount-state validation and that write-capable or browser-opening command shapes are refused.

drivers_go/aws_driver/aws_driver.go contains AWS mount-state validation, profile and Region hierarchy, and opaque targets.

drivers_go/aws_driver/aws_command.go contains AWS CLI configuration, validation, exact read-only command gates, and bounded execution.

drivers_go/aws_driver/aws_views.go contains Resource Explorer and Cloud Control hierarchy presentation.

drivers_go/aws_driver/aws_action.go contains configuration and Cloud Control action validation, idempotency tokens, exact mutation gates, and bounded execution.

drivers_go/aws_driver/aws_service_command.go contains the curated service catalog and exact read-only service command gate. drivers_go/aws_driver/aws_service_views.go presents generic JSON responses and detailed EC2, S3, and IAM resources. drivers_go/aws_driver/aws_service_actions.go validates and builds the exact EC2, security-group, S3, and IAM mutations.

drivers_go/aws_driver/aws_driver_test.go verifies the complete hierarchy and actions against a fake AWS CLI and rejects unsafe command shapes without contacting AWS.

drivers_go/diff_driver/diff_driver.go parses one unified file patch into metadata, hunk, context, deletion, and addition rows with line numbers.

drivers_go/diff_driver/diff_driver_test.go verifies unified-diff row types, text, and old/new line numbering.

drivers_go/zip_driver.go contains the ZIP archive driver source.

drivers_go/text_driver.go contains the URL extraction driver source.

drivers_go/kubernetes_driver.go contains the read-only kubernetes browsing driver source.

drivers_go/json_driver.go contains the JSON structure browsing driver source.

drivers_go/k8s_pod_driver.go contains the pod view driver source.

drivers_go/k8s_workload_driver.go contains the workload view driver source.

drivers_go/k8s_service_driver.go contains the service view driver source.

drivers_go/k8s_configmap_driver.go contains the ConfigMap and Secret payload driver source.

drivers_go/k8s_node_driver.go contains the node view driver source.

drivers_go/image_driver/image_driver.go contains the image metadata preview driver.

drivers_go/pdf_driver/pdf_driver.go contains the PDF metadata and link preview driver.

drivers_go/html_driver/html_driver.go contains the inert HTML title and link preview driver.

drivers_go/csv_driver/csv_driver.go contains the structured CSV preview driver.

web/index.html contains the browser UI source embedded into the Datatree executable. It uses browser-native JavaScript fetch calls and has no separate runtime asset.

build.sh compiles the host, every driver package, and the embedded web UI into one ./datatree executable.

run.sh compiles the single executable, then starts it with its optional directory argument.

test.sh compiles the project, runs all package tests, verifies the single binary, runs driver modes from that binary, runs ./datatree -test, and runs install_test.sh.

demo.sh compiles and starts the single executable, which prints and opens its operating-system-selected local HTTP URL.

install.sh compiles the project, places the single executable at $HOME/.local/libexec/datatree by default, and creates $HOME/.local/bin/datatree as a relative symlink to it. The symlink preserves the invocation directory and optional directory argument. DATATREE_PREFIX selects another prefix.

install_test.sh installs into a temporary prefix, verifies that the prefix contains exactly one regular file and that bin/datatree links to it, runs the installed command's foreground -test-installed-layout check from an empty directory, and prints the retained temporary path for inspection. The check verifies the executable permission, embedded web UI, embedded root_driver startup, and embedded json_driver loading. It starts no HTTP server.

10. Test Requirements

The project is complete only when ./test.sh passes.

./datatree -test creates a temporary visible fixture directory in the user's home directory, then verifies:

root_driver exposes HOME and / in no-argument location-browser mode, lists the fixture directory when the selected directory is the user's home directory, and exposes the . Git row when the selected directory is a Git repository.

The self-test writes fake kubectl, GitHub CLI, AWS CLI, and system_profiler executables and verifies that selected-directory files named hardware, usb, kubernetes, github, and aws remain ordinary readable files while independently routed virtual resources with those display names remain expandable. It verifies that the hardware overview shows the local model fact, the USB tree expands through bus, device, and product identifier levels, and both system-profiler virtual entries disappear when the executable cannot be resolved. Package tests also verify duplicate USB-name disambiguation, command timeout enforcement, the upload-compatible driver list, opaque routing-key selection, and that the command gate rejects unrelated system-profiler data types before execution.

directory_driver expands that fixture.

GET-style handle range serving returns exact file bytes.

text_driver extracts the actual URL as a url resource.

zip_driver expands a ZIP file.

zip_driver classifies and expands a nested ZIP file.

zip_driver can also load from uploaded blob bytes through the host blob path.

The self-test creates a real local Git working tree with two commits, a staged file, an unstaged modification, and an untracked file, plus an unborn repository. directory_driver identifies both as kind git. git_driver shows branch and HEAD data, lists and expands commits, serves an exact historical file, serves committed HEAD bytes independently of the modified working tree, serves exact staged, unstaged, and untracked bytes, returns an empty conflict view, appends a visible limit error when DATATREE_GIT_MAX_RESULTS is one, and returns empty commit and committed-file views for the unborn repository.

The self-test also writes a fake kubectl executable and selected kubeconfig into the fixture directory, points DATATREE_KUBECTL and DATATREE_KUBECONFIG at them, and verifies the kubernetes stack without ever contacting a real cluster:

root_driver lists the virtual kubernetes entry.

kubernetes_driver walks the context, namespace, resource type, and object levels through the fake kubectl.

Context output is sorted and deduplicated. Pod, workload, service, ConfigMap or Secret, node, and generic custom resources are routed to their expected drivers, including API-group-qualified workload types. The pod manifest served through the handle stack matches the fake kubectl output exactly, including offset and length slicing.

k8s_pod_driver presents the curated pod view: the phase, node, and summed restarts kv leaves are present, and the "container NAME: IMAGE (ready R, restarts N)" child expands through json_driver to the container's image leaf.

json_driver browses the pod view's [manifest] child as hierarchical data: the apiVersion scalar leaf is present, metadata.annotations.docs is reached as a kv leaf holding the embedded URL, and spec.containers[0].image is reached by descending through an object, an array, and an array element.

The [cluster-scoped] entry lists cluster resource types, and their objects carry the type's mapped kind (nodes are k8s-node).

test.sh additionally smoke-tests each kubernetes object driver by piping it a small manifest and checking one expected curated row.

test.sh verifies the browser upload controls, settings explanation, per-upload rejection, retained-memory eviction, active-upload protection, idle expiry, unload cleanup, and smoke-tests the image, PDF, HTML, and CSV preview output.

A context the fake kubectl rejects produces a single error resource.

A denied Secret manifest stays visible as the complete kubectl error through k8s_configmap_driver. One-result and ten-byte limits prove deterministic truncation and bounded kubectl output without mutating a cluster.

The kubernetes entry is hidden when no kubectl executable can be resolved.

The aws entry is hidden when no AWS CLI executable can be resolved. AWS package tests use a fake CLI to verify profile settings, Region discovery, the service catalog, generic service JSON, detailed EC2 instances and security groups, S3 buckets, IAM roles and policies, their exact actions, Resource Explorer results, Cloud Control types, resources, properties, updates, local output-format changes, and rejection of arbitrary commands without contacting an AWS account.

The self-test writes a fake gh executable, points DATATREE_GH at it, and verifies the GitHub stack without contacting GitHub: root_driver exposes github; github_driver exposes authored and review-requested views; the authored list exposes a repository-qualified pull request; expanding the pull request exposes scalar details, nested files, [actions], [CI/CD runs], and [diff]; the fake comment action receives its exact body; the CI branch lists and expands a failed run and reruns its failed jobs; expanding [diff] exposes main.go and its numbered addition; the empty review-requested list remains empty; and an invocation audit refuses unexpected commands.

test.sh verifies that web/index.html contains right-arrow and down-arrow expansion states, the recursive-expand control, recursive traversal, GitHub and AWS action endpoint calls, confirmations, action explanations, AWS desired-state and JSON-patch inputs, bookmark fragment encoding, clipboard copy, and bookmark restoration. Browser verification confirms that activating recursive expansion reaches PR actions, CI jobs, and diff lines; action controls have their expected inputs and selectors; copied fragments restore their path after reload; and desktop and mobile layouts do not overflow or overlap.

Every kubectl invocation recorded by the fake kubectl is get, api-resources, or config get-contexts. Any other recorded invocation fails the test, which guards the read-only guarantee.

The self-test removes its temporary fixture directory when it finishes.
