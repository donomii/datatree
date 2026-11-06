# pseudocode

A host process orchestrates a stack of driver subprocesses. The host, every driver, and the browser UI are built into one executable. Each driver subprocess is another mode of that executable and speaks JSON lines on stdin/stdout. The host bridges browser HTTP requests to the driver stack and proxies range requests downward.

## Data structures

```
Resource     = { name: string, kind: string, size: int64, key?: opaque string }
RangeReq     = { cmd: "range", offset, length, child? }
RangeResp    = { data: base64-string, size: int64 }
ChildHandle  = { hid, child_name, size, kind, driver_name }
GitMount     = { repository, view, revision? }
SystemProfilerMount = { apiVersion, profile, view, node? }

Store:
    blobs:           id -> { bytes, last_used }
    blob_bytes:      total retained uploaded bytes
    driver_blob_ids: driver session -> uploaded resource id
    upload_limits:   per-file bytes, total retained bytes, inactive lifetime
    handles:         hid -> ChildHandle
    drivers:         instance_name -> driver_process
    root_resources:  list of Resource shown at top level
```

## Host startup

```
root_directory = argument directory, or empty for the location browser
store = NewStore()
store.root_directory = root_directory
spawn goroutine:
    sleep 200ms
    loadRootDriver()                # populates store.root_resources
http.Handle("/",            serve embedded web UI)
web UI snapshot: visit visible directory rows and currently expanded children
omit action rows and handles; save name, kind, size, and children as version 1 JSON
reject snapshots exceeding 12 levels, 5000 resources, or 4 MiB
http.Handle("/api/drivers", list upload-compatible driver names)
http.Handle("/api/list",    return store.root_resources)
http.Handle("/api/upload",  store request bytes and return resource id)
http.Handle("/api/upload-settings", return active limits and their explanation)
http.Handle("/api/load",    mount selected driver on an uploaded resource id)
http.Handle("/api/autoLoad", autoLoad)
http.Handle("/api/data",    serveData)
listen_address = DATATREE_ADDR when configured, otherwise "127.0.0.1:0"
listener = Listen(listen_address)
browser_url = URL using listener's actual address and selected port
print browser_url
open browser_url
Serve(listener)
```

## Driver lifecycle

```
function startDriver(driverName):
    executable = current Datatree executable
    cmd = exec(executable, "--driver", driverName)
    pipe stdin/stdout into json encoder/decoder
    return process

function driverLoop(proc, on_range):
    loop:
        msg = decode JSON from proc.stdout
        if msg.cmd == "range":
            data = on_range(msg.offset, msg.length)
            send { data: base64(data), size: len(data) }
        elif msg has "resources":
            return msg.resources
```

## Loading a driver

```
function loadRootDriver():
    if store.root_directory is empty:
        proc = startDriver("root_driver")
    else:
        proc = startDriver("root_driver", store.root_directory)
    register as "root_driver"
    resources = driverLoop(proc, on_range = empty bytes)
                         # root_driver lists locations or the selected directory
    for each r in resources:
        child_key = r.key when present, otherwise r.name
        h = ChildHandle{ child_name: child_key, kind: r.kind,
                         size: r.size, driver_name: "root_driver" }
        record handle
        emit "{name}|{hid}" to UI
    return resources

function rootDriverWithoutDirectory():
    emit HOME as directory carrying the user's absolute home path
    emit / as directory carrying the filesystem-root path
    for each readable non-hidden directory directly below /Volumes:
        emit "VOLUME: name" as directory carrying its absolute path
    for each readable non-hidden directory directly below /mnt or /media:
        emit "MOUNT: absolute path" as directory carrying its absolute path
    ignore missing mount roots, non-directories, and duplicate paths
    serve a location path only for an exact emitted name
function rootDriverWithDirectory(directory):
    list the directory's real children

for either root-driver mode:
    emit each available virtual service with a NUL-prefixed opaque key
    route its opaque key to its mount state
    route an ordinary child name to the filesystem or location map
    therefore virtual and real children with the same display name coexist

function loadDriverForHandle(driverName, parentHandle):
    proc = startDriver(driverName)
    register as instance "driverName_hid"
    resources = driverLoop(proc, on_range = serveChildRange(parentHandle, ...))
                         # forwards range requests up the stack
    for each r in resources:
        register handle pointing at this instance
    return resources

function loadDriverForBlob(driverName, resourceId):
    require resourceId exists in uploaded blobs
    reserve resourceId for session "driverName_resourceId"
    start and register the selected driver
    answer its range requests from the uploaded bytes
    register handles for its resources
    return resources

function stopDriver(session):
    remove the session
    close its input and wait for it to exit
    remove every handle owned by the session
    if the session reserved an uploaded blob:
        remove the blob and subtract its size from retained bytes
```

## Serving bytes through the stack

```
function serveChildRange(handle, offset, length):
    driver = drivers[handle.driver_name]
    send  { cmd: "range", offset, length, child: handle.child_name } to driver
    resp  = read driver's response
    return base64-decode(resp.data)
```

This is recursive in effect: when one driver asks the host for bytes during init, the host calls `serveChildRange(parentHandle, ...)`, which talks to the parent driver, which may itself be servicing its own child requests.

## HTTP handlers

For a loopback listener, reject request authorities other than the bound IP
or localhost at the listening port.

When gallery assets are configured, require loopback and an explicit browse
directory. Serve the embedded exhibit and its named portfolio dependencies.
Open the exhibit URL. On Browse live Datatree, fetch root resources from the
same origin. Omit action rows. Selecting an expandable resource sends its
opaque handle to the child endpoint and renders the result as plaques and
matching buttons. Only the latest navigation request may replace the view.
Expose the live-directory entry and return to the sample as selectable plaques
as well as page actions. Display branch and live-directory request status in
the scene and preserve the current tree if a request fails.
During a headset session, request optional hand tracking and draw the joints
and links whose poses are available. Draw each tracked input's beam to its
nearest selectable plaque. On the controller trigger or hand's primary
selection, activate that plaque only if the input has a valid current aim.
Use the focused keyboard or a standard gamepad to move, look, and activate the
plaque under the center dot. In a headset, use the left stick to move relative
to the viewer's heading and the right stick for one snap turn per tilt. Rotate
around the viewer's position and constrain virtual movement to the exhibit floor.

Before dispatching an API request, reject browser origins other than the
Datatree origin and browser metadata indicating a cross-origin request.
Allow direct HTTP clients and ordinary navigation to the browser page.

```
GET  /api/list:
    return { resources: store.root_resources }

POST /api/autoLoad { handle, kind }:
    h = store.getHandle(handle)
    driver = match kind:
        "directory"          -> "directory_driver"
        "git"                -> "git_driver"
        "github"             -> "github_driver"
        "aws"                -> "aws_driver"
        "hardware" | "usb" -> "system_profiler_driver"
        "diff"               -> "diff_driver"
        "text"               -> "text_driver"
        "archive" | "zip"    -> "zip_driver"
        "image"               -> "image_driver"
        "pdf"                 -> "pdf_driver"
        "html"                -> "html_driver"
        "csv"                 -> "csv_driver"
        "kubernetes"          -> "kubernetes_driver"
        "json"                -> "json_driver"
        "k8s-pod"             -> "k8s_pod_driver"
        "k8s-workload"        -> "k8s_workload_driver"
        "k8s-service"         -> "k8s_service_driver"
        "k8s-configmap"       -> "k8s_configmap_driver"
        "k8s-node"            -> "k8s_node_driver"
        "url" | "kv" | "error" -> not expandable
        otherwise            -> "text_driver"
    res = loadDriverForHandle(driver, h)
    return { resources: res, driver: driver }

POST /api/upload with raw bytes:
    reject after reading at most per-file limit plus one byte
    remove expired inactive uploads
    while storing the body would exceed total retained bytes:
        remove the least recently used inactive upload
        if every retained upload is active: reject with the byte counts
    resourceId = store bytes with current last-used time
    return { res_id: resourceId, size: byte count }

GET /api/upload-settings:
    return active per-file limit, retained-memory limit, idle lifetime, and explanation

POST /api/load { res_id, driver }:
    resources = loadDriverForBlob(driver, res_id)
    return { resources, driver, session }

GET /api/data?handle=H&offset=O&length=L:
    h     = store.getHandle(H)
    bytes = serveChildRange(h, O, L)
    write octet-stream
```

## Recursive browser expansion

```
render each expandable resource with:
    one expand-or-collapse button showing right arrow when closed and down arrow when open
    one recursive-expand button
    one copy-link button when the resource belongs to the reloadable root tree

recursive_expand(resource_row):
    if the selected row is not expanded:
        call POST /api/autoLoad for the selected row
        render the returned children
    else:
        keep the selected row expanded
    for each rendered child in order:
        if the child is expandable:
            recursive_expand(child)
        else:
            leave the child unchanged
    keep a failed child expansion visible and continue with later siblings

expand_or_collapse(resource_row):
    when opening, insert the child tree and change the arrow to down
    keep the down arrow for empty results and visible expansion errors
    when closing, remove the child tree and change the arrow to right

copy_link(resource_path):
    encode up to 64 resource name and kind segments in a URL-safe fragment
    put the fragment in the current address
    copy the complete URL when the browser clipboard is available

restore_link(fragment):
    validate and decode the resource path
    fetch fresh root resources
    at each level, match the named resource and kind and expand it
    report the exact segment when the current tree no longer contains the path
```

## A driver

```
on startup:
    optionally request upstream bytes via { cmd: "range", offset, length } on stdout
    read response on stdin
    parse / inspect data
    emit { resources: [ {name, kind, size}, ... ] } on stdout

if non-leaf:
    loop:
        read { cmd: "range", child, offset, length } from stdin
        bytes = produce child's bytes (extract zip entry, read file, etc.)
        emit { data: base64(bytes), size }
```

## Browser upload

```
on page load:
    list upload-compatible drivers and fill the preview selector
    show the active upload limits and explain their retention behavior

on Upload and preview:
    require one selected file and one selected driver
    upload raw file bytes
    mount selected driver on returned resource id
    display progress, errors, and the returned resource tree

on Close FILE:
    unload the returned driver session
    remove the preview tree after the server confirms that its bytes and handles were released
```

## Preview drivers

```
image: identify format and dimensions from the header
pdf: validate version header; list reliable page count and absolute links
html: list normalized title and unique absolute links without executing content
csv: list dimensions; expose headings and rows as expandable JSON children
```

## Git repository browsing

```
configuration:
    git = DATATREE_GIT or git from PATH
    max_results = positive DATATREE_GIT_MAX_RESULTS or 500
    max_output_bytes = positive DATATREE_GIT_MAX_OUTPUT_BYTES or 8388608

when a directory contains .git:
    classify it as kind git
    serve its absolute path when expanded

when the selected directory is inside a Git work tree:
    add "." as kind git
    serve the selected directory path when "." is expanded

mount(initial absolute path):
    repository = read-only Git rev-parse top-level path
    view = repository

mount(JSON GitMount):
    validate absolute repository path, known view, and full commit id where required

repository view:
    read branch and HEAD without optional locks
    emit repository, branch, HEAD or unborn status, and active limits
    emit expandable commits, committed, all-uncommitted, staged, unstaged, untracked, and conflicted sections

commits view:
    run bounded, NUL-delimited Git log newest first
    emit short id, ISO date, author, and complete subject
    each child mount carries the full immutable commit id

commit view:
    emit full id, parents, author identity, ISO date, and subject
    list changed paths from read-only diff-tree
    serve each non-deleted file from the selected commit

committed view:
    list blobs recursively from HEAD with ls-tree
    serve each file from HEAD, never from the working tree

working-state views:
    parse NUL-delimited porcelain status with rename detection disabled
    all-uncommitted = every changed path once, labelled with every applicable state
    staged      = index status; serve index bytes
    unstaged    = working-tree status; serve current path bytes
    untracked   = untracked status; serve current path bytes
    conflicted  = unmerged status; serve current path bytes when it exists
    keep deleted paths as informational leaves
    serve symbolic-link text without following the link

read-only command gate:
    allow only rev-parse, symbolic-ref, status, log, show, ls-tree, diff-tree
    invoke Git directly with no pager and optional locks disabled
    bound stdout and stderr
    refuse every other subcommand before starting it
```

## GitHub pull request browsing

```
configuration:
    gh = DATATREE_GH or gh on PATH
    max_output_bytes = positive DATATREE_GITHUB_MAX_OUTPUT_BYTES or 8388608

mount state = { view, url?, repository?, branch?, state?, isDraft?, runId? }
reject a mount state over 16384 bytes
require pull-request URLs to be HTTPS, non-empty, bounded, and free of whitespace

root view:
    emit authored and review-requested sections as github children

authored view:
    run exact allowed command: gh search prs --author=@me --state=open --limit=1000 --json=selected_fields
    validate and sort results by repository and number
    emit each result as a github child carrying its pull request URL

review-requested view:
    run the equivalent exact allowed search with --review-requested=@me
    validate and sort results by repository and number

pull-request view:
    run exact allowed command: gh pr view validated_url --json=detail_fields
    prepend [actions] carrying pull request URL, state, and draft status
    prepend [CI/CD runs] carrying validated repository and head branch
    prepend a github child named [diff] carrying { view: diff, url: validated_url }
    browse the returned object through generic JSON listing

pull-request actions view:
    always offer comment
    for open requests offer close, approve, review comment, request changes, and merge when not draft
    for closed requests offer reopen

CI/CD runs view:
    run exact allowed command: gh run list for validated repository and branch with limit 100 and fixed JSON fields
    sort newest database identifiers first
    emit each run as a github child carrying repository and positive run identifier

workflow run view:
    run exact allowed command: gh run view for validated repository and run identifier with fixed JSON fields
    browse jobs and metadata through generic JSON listing
    prepend [actions] carrying repository, run identifier, and status

workflow run actions view:
    queued or in-progress -> offer cancel
    completed -> offer rerun all and rerun failed

diff view:
    run exact allowed command: gh pr diff validated_url --patch --color=never
    split the patch at unified-diff file boundaries
    append repeated file sections to the first matching file in output order
    emit each changed file as a diff child containing that file's patch

diff file view:
    emit file headers and rename or binary facts as metadata rows
    emit each hunk header as a hunk row
    emit context, removed, and added lines with old and new line numbers
    advance old and new line numbers according to unified-diff semantics

read-only command gate:
    allow only the exact generated search prs, pr view, pr diff, run list, and run view argument sequences
    refuse every other gh invocation before starting it
    retain at most max_output_bytes from stdout and stderr

POST /api/github/action:
    require Origin to exactly match the local request host
    resolve the opaque github-action handle and read its validated target
    accept body only for comment and review actions, bounded to 65536 bytes
    accept merge method only for merge, defaulting to squash
    require confirmation in the browser for state-changing and review-decision actions
    permit only exact close, reopen, merge, comment, review, cancel, and rerun command shapes
    execute gh directly without a shell and report the bounded result
```

## AWS browsing and actions

Content browsing extension:

```
S3 bucket or prefix:
    validate profile, region, bucket, opaque prefix and optional page token
    list objects and common prefixes using delimiter slash
    expose prefixes as nested views and objects as ordinary file resources with actual kinds and sizes
    distinguish reserved controls with unique internal keys
    expose returned continuation as next-page child and preserve bucket actions

S3 object:
    allow only exact remote-object to stdout copy
    read actual file bytes independently of the CLI metadata limit
    resolve provider descriptors internally and serve file bytes directly to downstream filters
    serve bytes through existing range protocol for nested drivers and download
    report permission, size and validation failures as visible errors

AWS data query:
    retain complete JSON response
    find compiled data edges matching the current operation
    validate each resource array and identifier
    expose named children for the resource's next read-only data queries
    retain parent identifier when the next query requires it
    allow only compiled argument templates; follow CLI automatic pagination
    explicitly report database-connection and state-changing SQS limitations

Browser preview:
    expanding text, HTML, binary, JSON, CSV, image or PDF also reads its content handle
    page text and binary content in 64 KiB ranges through the entire resource
    render text and HTML source using text nodes, binary and split UTF-8 bytes as hexadecimal
    offer an explained filter selector on each readable child, defaulting to its ordinary kind
    mount the selected filter on its byte handle regardless of extension or provider
    retain JSON scalar bytes for subsequent filtering
    keep duplicate ZIP names independently servable; report corrupt or unreadable entries
    render images and PDFs through local blob URLs
    preserve nested driver results and revoke preview URLs when collapsing
```

```
configuration:
    aws = DATATREE_AWS or aws on PATH
    max_output_bytes = positive DATATREE_AWS_MAX_OUTPUT_BYTES or 8388608

root view:
    run exact local command aws configure list-profiles
    emit each profile as an aws child

profile view:
    read only non-secret region, output, role, source-profile, credential-source, and SSO settings
    emit configuration actions and regions-and-resources branches

regions view:
    run exact describe-regions command for the validated profile
    use configured region or us-east-1 as the bootstrap endpoint
    emit each region as an aws child

region view:
    emit service catalog, Resource Explorer search, and Cloud Control resource-type branches

service catalog view:
    emit each compiled read-only operation as an aws child
    include compute, networking, storage, identity, security, auditing, databases, containers, messaging, monitoring, deployment, backup, and account-management services
    execute only the exact argument prefix stored for the selected operation
    emit generic responses as recursively browseable JSON
    parse EC2 instances, security groups, S3 buckets, IAM roles, and customer-managed IAM policies into named detailed resources

detailed service resource view:
    run the exact describe or get command for the validated operation and identifier
    emit complete details as JSON and the operation's action branch

Resource Explorer view:
    search for all resources visible to the default view, maximum 1000
    sort and emit each result as expandable JSON

Cloud Control types view:
    list public AWS resource types, maximum 1000
    omit non-provisionable types
    emit each supported type as an aws child

resource-type view:
    offer create with desired-state JSON
    list resources of that type, maximum 1000

resource view:
    get current resource properties
    emit properties as JSON and actions as aws children

POST /api/aws/action:
    require same-origin POST and browser confirmation
    resolve the opaque aws-action handle and read its validated target
    set-region -> exact local configure set for region
    set-output -> exact local configure set for allowed output format
    create -> validated desired-state JSON object plus random idempotency token
    update -> validated non-empty RFC 6902 JSON patch plus random idempotency token
    delete -> validated opaque target plus random idempotency token and no body
    EC2 instance -> exact start, stop, reboot, or terminate for one identifier and no body
    security group -> exact authorize or revoke ingress with a non-empty IP-permissions JSON array
    S3 bucket -> exact put or delete policy, or put versioning state
    IAM role -> exact trust-policy replacement with a complete JSON policy object
    IAM customer-managed policy -> create a complete JSON policy version and make it default
    refuse credentials, arbitrary services, arbitrary flags, and shell execution
```

## Kubernetes browsing

```
configuration:
    kubeconfig = DATATREE_KUBECONFIG or kubectl's normal selection
    max_results = non-negative DATATREE_KUBERNETES_MAX_RESULTS or 0
    max_results 0 means retain every result
    max_output_bytes = positive DATATREE_KUBERNETES_MAX_OUTPUT_BYTES or 8388608

mount state = { context?, namespace?, cluster?, type? }
reject a mount state over 16384 bytes
reject selections over 4096 bytes or containing NUL

list(state):
    if context is absent:
        run allowed command: kubectl config get-contexts -o name
    else if scope is absent:
        run allowed command: kubectl get --context=context namespaces -o name
        prepend [cluster-scoped]
    else if type is absent:
        run allowed command: kubectl api-resources --context=context --verbs=list --namespaced=scope -o name
    else:
        run allowed command: kubectl get --context=context [--namespace=namespace] type -o name
        map pods, workloads, services, config data, and nodes to their curated driver kinds
        map other resource types to json
    retain at most max_output_bytes from stdout and stderr while reading each stream to completion
    deduplicate and sort output names
    if max_results is positive, retain the first max_results names and append an explanatory error leaf when truncated

expand object:
    run allowed command: kubectl get --context=context [--namespace=namespace] type object -o json
    on success, serve the bounded manifest bytes
    on failure, serve a reserved error payload that the selected object driver displays as an error leaf
```

## Local hardware and USB browsing

```
configuration:
    system_profiler = DATATREE_SYSTEM_PROFILER or /usr/sbin/system_profiler
    max_output_bytes = positive DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES or 8388608
    command_timeout = positive DATATREE_SYSTEM_PROFILER_TIMEOUT or 30s

mount state = { apiVersion: "datatree.local/system-profiler/v1", profile, view, node? }
require profile to be hardware or usb
require view root to have no node
require view node to contain one valid JSON value

hardware root:
    run exact allowed command: system_profiler SPHardwareDataType -json
    require the matching top-level JSON field
    when the field is one overview object, list that object's properties directly

usb root:
    run exact allowed command: system_profiler -listDataTypes
    select SPUSBHostDataType, or legacy SPUSBDataType when it is the available type
    run exact allowed command: system_profiler selected_type -json
    require the matching top-level JSON field

list system-profiler value:
    arrays -> name object children from _name, otherwise from their index
    objects -> flatten _items arrays and list every other field
    objects or arrays -> emit an expandable child with a versioned node mount state
    scalar -> emit a readable kv leaf without changing its value
    suffix duplicate names with [2], [3], and so on within their parent

read-only command gate:
    allow only -listDataTypes and JSON reads of SPHardwareDataType, SPUSBHostDataType, or SPUSBDataType
    execute directly without a shell or network access
    retain at most max_output_bytes from stdout and stderr
    terminate a command that does not finish within command_timeout and return a visible error
```

## Installed-layout check

```
install into a temporary prefix
run the installed single binary from an empty directory
verify the installed binary exists and is executable
verify the embedded web UI contains its expected heading
start embedded root_driver through the host driver protocol
load JSON bytes through embedded json_driver
exit in the foreground and print the retained temporary path
```
