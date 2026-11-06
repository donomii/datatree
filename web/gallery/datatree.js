import * as T from "../../projects/games/domino-rush/vendor/three.module.js";
import { ArchiveEntry, fileLimit, parseArchive } from "./archive.js";

const archive = {
    "maps.zip": {
        "mediterranean.json": { region: "Mediterranean", layer: "Roads", format: "GeoJSON" },
        "tiles.zip": { "0-0.png": "Map tile, 256 × 256 pixels", "0-1.png": "Map tile, 256 × 256 pixels" },
    },
    "catalogue.json": { title: "Courtyard collection", edition: 1, open: true },
    "readme.txt": "A sample archive for the Datatree exhibit.",
};
const viewport = document.querySelector("#viewport");
const controls = document.querySelector("#tree-controls");
const pathText = document.querySelector("#tree-path");
const valueText = document.querySelector("#tree-value");
const vr = document.querySelector("#vr");
const filePicker = document.querySelector("#tree-file");
const path = [{ name: "exhibits.zip", value: archive }];
const scene = new T.Scene();
scene.background = new T.Color(0x201a18);
const camera = new T.PerspectiveCamera(62, 1, 0.05, 30);
const rig = new T.Group();
camera.position.set(0, 1.6, 2.55);
rig.add(camera);
scene.add(rig);
const tree = new T.Group();
tree.position.set(0, 1.45, -2.4);
scene.add(tree);
scene.add(new T.HemisphereLight(0xffe0b5, 0x87685a, 2.5));
const bronze = new T.MeshStandardMaterial({ color: 0xa98255, roughness: 0.75 });
const floor = new T.Mesh(new T.CircleGeometry(5, 48), new T.MeshStandardMaterial({ color: 0x664936 }));
floor.rotation.x = -Math.PI / 2;
scene.add(floor);
const plaster = new T.MeshStandardMaterial({ color: 0x804b37, roughness: 0.93 });
function stonework(width, height, depth, x, y, z, material) {
    const piece = new T.Mesh(new T.BoxGeometry(width, height, depth), material);
    piece.position.set(x, y, z);
    scene.add(piece);
}
stonework(6.4, 3.65, 0.28, 0, 1.825, -3.3, plaster);
for (const x of [-2.65, 2.65]) {
    stonework(0.32, 3, 0.38, x, 1.5, -3.02, bronze);
    stonework(0.53, 0.17, 0.59, x, 3.04, -3.02, bronze);
}
stonework(5.85, 0.23, 0.42, 0, 3.22, -3.03, bronze);
stonework(4.9, 0.42, 0.9, 0, 0.21, -2.55, bronze);
let renderer;
let selected;
let targets = [];
let page = 0;
let revision = 0;
const pageSize = 3;

function isBranch(value) {
    return typeof value === "object" && value !== null;
}

async function liveResources(endpoint, body) {
    const response = await fetch(endpoint, body === undefined ? {} : {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error(`Datatree could not open the resource (${response.status}): ${(await response.text()).slice(0, 300)}`);
    const { resources } = await response.json();
    if (resources === null) return Object.create(null);
    if (!Array.isArray(resources)) throw new Error("Datatree returned an invalid resource list.");
    const result = Object.create(null);
    for (const row of resources) {
        if (typeof row?.name !== "string" || typeof row.kind !== "string" || !Number.isSafeInteger(row.size)) {
            throw new Error("Datatree returned an invalid resource.");
        } else if (!row.kind.includes("-action-")) {
            let name = row.name;
            for (let suffix = 2; Object.hasOwn(result, name); suffix++) name = `${row.name} (${suffix})`;
            result[name] = row.handle && liveKinds.has(row.kind)
                ? new LiveBranch(row)
                : `${row.kind}, ${row.size < 0 ? "size unknown" : `${row.size} bytes`}`;
        }
    }
    return result;
}

const liveKinds = new Set(["directory", "archive", "zip", "json", "text", "file", "image", "pdf", "html", "csv"]);

class LiveBranch {
    constructor(row) {
        this.row = row;
    }

    open() {
        return liveResources("/api/loadChild", { handle: this.row.handle });
    }
}

function label(title, detail, fill) {
    const canvas = document.createElement("canvas");
    canvas.width = 512;
    canvas.height = 208;
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = fill;
    ctx.fillRect(0, 0, 512, 208);
    ctx.strokeStyle = "#d4ad76";
    ctx.lineWidth = 6;
    ctx.strokeRect(8, 8, 496, 192);
    ctx.fillStyle = "#fff0d6";
    ctx.textAlign = "center";
    ctx.font = "bold 49px Georgia";
    ctx.fillText(title, 256, 84, 472);
    ctx.font = "34px system-ui";
    ctx.fillText(detail, 256, 152, 472);
    const map = new T.CanvasTexture(canvas);
    map.colorSpace = T.SRGBColorSpace;
    return new T.MeshBasicMaterial({ map });
}

function plaque(x, y, z, width, title, detail, fill, action) {
    const back = new T.Mesh(new T.BoxGeometry(width + 0.06, 0.48, 0.12), bronze);
    back.position.set(x, y, z);
    tree.add(back);
    const face = new T.Mesh(new T.PlaneGeometry(width, 0.42), label(title, detail, fill));
    face.position.set(x, y, z + 0.063);
    tree.add(face);
    if (action) {
        face.userData.action = action;
        targets.push(face);
    }
}

function branchLine(x, z) {
    const shape = new T.BufferGeometry().setFromPoints([
        new T.Vector3(0, 0.58, -0.22), new T.Vector3(0, 0.32, -0.22),
        new T.Vector3(x, 0.32, z), new T.Vector3(x, 0.10, z),
    ]);
    tree.add(new T.Line(shape, new T.LineBasicMaterial({ color: 0xd5ac77 })));
}

async function open(name, value) {
    const request = ++revision;
    try {
        if (value instanceof ArchiveEntry || value instanceof LiveBranch) {
            renderTree(`Opening ${name}…`);
            value = await value.open();
        }
    } catch (error) {
        if (request === revision) renderTree(error.message);
        return;
    }
    if (request !== revision) return;
    if (isBranch(value)) {
        if (path.length >= 16) {
            renderTree("This branch exceeds the 16-level gallery limit.");
            return;
        }
        path.push({ name, value });
        selected = undefined;
        page = 0;
    } else {
        selected = { name, value };
    }
    renderTree();
}

function goUp() {
    revision++;
    if (path.length > 1) path.pop();
    selected = undefined;
    page = 0;
    renderTree();
}

function button(text, action) {
    const control = document.createElement("button");
    control.textContent = text;
    control.addEventListener("click", action);
    controls.append(control);
}

function renderTree(status = "") {
    controls.replaceChildren();
    const current = path[path.length - 1];
    pathText.textContent = path.map(entry => entry.name).join(" / ");
    valueText.textContent = status || (selected ? `${selected.name} = ${String(selected.value).slice(0, 4096)}` : "");
    for (const child of [...tree.children]) {
        child.geometry.dispose();
        if (child.material !== bronze) {
            child.material.map?.dispose();
            child.material.dispose();
        }
        tree.remove(child);
    }
    targets = [];
    plaque(0, 0.82, -0.22, 1.85, current.name, "CONTAINER", "#744637");
    if (path[0].value === archive) {
        plaque(1.63, 0.82, 0.05, 1.25, "Live Datatree", "BROWSE DIRECTORY", "#455d52", browseLive);
    } else {
        plaque(1.63, 0.82, 0.05, 1.25, "Sample archive", "RETURN TO SAMPLE", "#70533d", restoreSample);
    }
    if (path.length > 1) {
        plaque(-1.5, 0.82, 0.05, 0.9, "← Up", "PARENT", "#70533d", goUp);
        button("Up to parent", goUp);
    }
    const all = Object.entries(current.value);
    const pages = Math.ceil(all.length / pageSize);
    const entries = all.slice(page * pageSize, (page + 1) * pageSize);
    function turnPage(offset) {
        revision++;
        page += offset;
        selected = undefined;
        renderTree();
    }
    if (page > 0) {
        const previous = () => turnPage(-1);
        plaque(-1.3, -0.76, 0.2, 0.64, "←", "PREV", "#70533d", previous);
        button("Previous page", previous);
    }
    if (page + 1 < pages) {
        const next = () => turnPage(1);
        plaque(1.3, -0.76, 0.2, 0.64, "→", "NEXT", "#70533d", next);
        button("Next page", next);
    }
    if (pages > 1) pathText.textContent += ` · page ${page + 1}/${pages}`;
    entries.forEach(([name, value], i) => {
        const x = (i - (entries.length - 1) / 2) * 1.25;
        const z = Math.abs(x) * 0.15;
        const action = () => open(name, value);
        branchLine(x, z);
        const branch = value instanceof ArchiveEntry ? value.kind === "zip" || value.kind === "json" : isBranch(value);
        plaque(x, -0.17, z, 1.17, name.slice(0, 90), branch ? "OPEN BRANCH" : "READ VALUE",
            branch ? "#455d52" : "#625141", action);
        button(name, action);
    });
    if (status) {
        plaque(0, -0.76, 0.2, 1.87, "Datatree", status.slice(0, 100), "#53466a");
    } else if (selected) {
        plaque(0, -0.76, 0.2, 1.87, selected.name.slice(0, 90), String(selected.value).slice(0, 160), "#53466a");
    }
    tree.updateWorldMatrix(true, true);
}

function openRoot(name, value) {
    path.splice(0, path.length, { name, value });
    selected = undefined;
    page = 0;
    renderTree();
}

function snapshotTree(text) {
    const snapshot = JSON.parse(text);
    if (snapshot?.datatree_snapshot !== 1) throw new Error("Choose a version 1 Datatree snapshot.");
    let count = 0;
    function branch(rows, depth) {
        if (!Array.isArray(rows) || depth > 12) throw new Error("Invalid or too deeply nested Datatree resources.");
        const result = Object.create(null);
        for (const row of rows) {
            if (++count > 5000 || typeof row?.name !== "string" || typeof row.kind !== "string" ||
                !Number.isSafeInteger(row.size)) throw new Error("Invalid resource or more than 5,000 Datatree resources.");
            let name = row.name;
            for (let suffix = 2; Object.hasOwn(result, name); suffix++) name = `${row.name} (${suffix})`;
            result[name] = row.children === undefined
                ? `${row.kind}, ${row.size < 0 ? "size unknown" : `${row.size} bytes`}`
                : branch(row.children, depth + 1);
        }
        return result;
    }
    return branch(snapshot.resources, 1);
}

filePicker.addEventListener("change", async () => {
    const file = filePicker.files[0];
    if (!file) return;
    const request = ++revision;
    filePicker.value = "";
    try {
        const snapshot = file.name.toLowerCase().endsWith(".json");
        if (snapshot && file.size > 4 * 1024 * 1024) throw new Error("Choose a Datatree snapshot of at most 4 MiB.");
        if (!snapshot && file.size > fileLimit) throw new Error("Choose a ZIP of at most 16 MiB.");
        const value = snapshot ? snapshotTree(await file.text()) : parseArchive(new Uint8Array(await file.arrayBuffer()));
        if (request === revision) openRoot(file.name, value);
    } catch (error) {
        if (request === revision) valueText.textContent = error.message;
    }
});
function restoreSample() {
    revision++;
    openRoot("exhibits.zip", archive);
}

async function browseLive() {
    const request = ++revision;
    renderTree("Reading live Datatree resources…");
    try {
        const value = await liveResources("/api/list");
        if (request === revision) openRoot("Live Datatree", value);
    } catch (error) {
        if (request === revision) renderTree(error.message);
    }
}

document.querySelector("#tree-sample").addEventListener("click", restoreSample);
document.querySelector("#tree-live").addEventListener("click", browseLive);

const ray = new T.Raycaster();
const vrInputs = [];

function selectTarget() {
    ray.intersectObjects(targets)[0]?.object.userData.action();
}

const navigationKeys = new Set();
const keyBindings = new Set(["KeyW", "KeyA", "KeyS", "KeyD", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Enter"]);
const viewer = new T.Vector3();
const forward = new T.Vector3();
const offset = new T.Vector3();
const movement = new T.Vector3();
const up = new T.Vector3(0, 1, 0);
const heading = new T.Quaternion();
const gaze = new T.Raycaster(undefined, undefined, 0, 6);
const cursor = new T.Mesh(new T.SphereGeometry(0.012, 8, 6), new T.MeshBasicMaterial());
cursor.visible = false;
scene.add(cursor);
let lastFrame = null;
let turnArmed = true;
let selectHeld = false;

function stick(value = 0) {
    return Math.abs(value) > 0.2 ? Math.sign(value) * (Math.abs(value) - 0.2) / 0.8 : 0;
}

function standardGamepad() {
    try {
        return Array.from(navigator.getGamepads?.() || []).find(pad => pad?.connected && pad.mapping === "standard");
    } catch {
        return undefined;
    }
}

function moveViewer(x, z, seconds) {
    const length = Math.max(1, Math.hypot(x, z));
    movement.set(x / length, 0, z / length);
    const yaw = Math.hypot(forward.x, forward.z) > 0.01
        ? Math.atan2(-forward.x, -forward.z) : rig.rotation.y;
    movement.applyAxisAngle(up, yaw).multiplyScalar(1.45 * seconds).add(viewer);
    const nearest = Math.min(-1.85, viewer.z);
    movement.z = Math.max(nearest, movement.z);
    const radius = Math.max(4.6, Math.hypot(viewer.x, viewer.z));
    const distance = Math.hypot(movement.x, movement.z);
    if (distance > radius) {
        movement.x *= radius / distance;
        movement.z *= radius / distance;
    }
    rig.position.x += movement.x - viewer.x;
    rig.position.z += movement.z - viewer.z;
}

function updateNavigation(time, frame) {
    const seconds = lastFrame === null ? 0 : T.MathUtils.clamp((time - lastFrame) / 1000, 0, 0.05);
    lastFrame = time;
    const session = renderer.xr.getSession();
    const pose = frame?.getViewerPose(renderer.xr.getReferenceSpace());
    if (document.hidden || (session && (!pose || session.visibilityState !== "visible"))) {
        cursor.visible = false;
        return;
    }
    const standard = standardGamepad();
    const sources = Array.from(session?.inputSources || []);
    const motion = sources.find(source => source.handedness === "left" && source.gamepad?.mapping === "xr-standard");
    const turning = sources.find(source => source.handedness === "right" && source.gamepad?.mapping === "xr-standard");
    const pressed = key => Number(navigationKeys.has(key));
    const x = stick(motion?.gamepad.axes[2] ?? standard?.axes[0]) + pressed("KeyD") - pressed("KeyA");
    const z = stick(motion?.gamepad.axes[3] ?? standard?.axes[1]) + pressed("KeyS") - pressed("KeyW");
    const turn = stick(turning?.gamepad.axes[2] ?? standard?.axes[2]) + pressed("ArrowRight") - pressed("ArrowLeft");
    const look = stick(standard?.axes[3]) + pressed("ArrowDown") - pressed("ArrowUp");
    const position = pose?.transform.position || camera.position;
    offset.set(position.x, position.y, position.z);
    rig.localToWorld(viewer.copy(offset));
    let angle = 0;
    if (session) {
        if (Math.abs(turn) < 0.2) turnArmed = true;
        if (turnArmed && Math.abs(turn) >= 0.55) {
            angle = -Math.sign(turn) * Math.PI / 6;
            turnArmed = false;
        }
    } else {
        angle = -turn * seconds * 1.8;
        camera.rotation.x = T.MathUtils.clamp(camera.rotation.x - look * seconds * 1.4, -1.25, 1.25);
    }
    if (angle) {
        rig.rotation.y += angle;
        offset.applyAxisAngle(up, rig.rotation.y);
        rig.position.x = viewer.x - offset.x;
        rig.position.z = viewer.z - offset.z;
    }
    const orientation = pose?.transform.orientation || camera.quaternion;
    heading.set(orientation.x, orientation.y, orientation.z, orientation.w);
    forward.set(0, 0, -1).applyQuaternion(heading).applyAxisAngle(up, rig.rotation.y);
    if (x || z) moveViewer(x, z, seconds);
    cursor.visible = Boolean(standard) || (!session && document.activeElement === renderer.domElement);
    const select = Boolean(standard?.buttons[0]?.pressed) || Boolean(pressed("Enter"));
    if (cursor.visible) {
        rig.localToWorld(gaze.ray.origin.copy(position));
        gaze.ray.direction.copy(forward);
        const hit = gaze.intersectObjects(targets)[0];
        gaze.ray.at(hit ? Math.max(0, hit.distance - 0.03) : 1.4, cursor.position);
        cursor.material.color.setHex(hit ? 0xffffff : 0xd5ac77);
        if (select && !selectHeld) hit?.object.userData.action();
    }
    selectHeld = select;
}

const handChains = [
    ["wrist", "thumb-metacarpal", "thumb-phalanx-proximal", "thumb-phalanx-distal", "thumb-tip"],
    ...["index", "middle", "ring", "pinky"].map(finger => ["wrist",
        ...["metacarpal", "phalanx-proximal", "phalanx-intermediate", "phalanx-distal", "tip"]
            .map(joint => `${finger}-finger-${joint}`)]),
];
const handJoints = [...new Set(handChains.flat())];
const handLinks = handChains.flatMap(chain => chain.slice(1).map((joint, i) => [chain[i], joint]));

function showTrackedHand(hand, sphere, color) {
    const dots = new T.InstancedMesh(sphere, new T.MeshBasicMaterial({ color }), handJoints.length);
    dots.count = 0;
    dots.frustumCulled = false;
    dots.instanceMatrix.setUsage(T.DynamicDrawUsage);
    const points = new T.BufferAttribute(new Float32Array(handLinks.length * 6), 3);
    points.setUsage(T.DynamicDrawUsage);
    const skeleton = new T.BufferGeometry().setAttribute("position", points);
    const lines = new T.LineSegments(skeleton, new T.LineBasicMaterial({ color }));
    lines.frustumCulled = false;
    hand.add(dots, lines);
    const transform = new T.Matrix4();
    return () => {
        if (!hand.visible) return;
        let count = 0;
        for (const name of handJoints) {
            const joint = hand.joints[name];
            if (joint?.visible) {
                const radius = joint.jointRadius > 0 ? joint.jointRadius : 0.008;
                transform.makeScale(radius, radius, radius).setPosition(joint.position);
                dots.setMatrixAt(count++, transform);
            }
        }
        dots.count = count;
        dots.instanceMatrix.needsUpdate = true;
        let vertex = 0;
        for (const [from, to] of handLinks) {
            const a = hand.joints[from], b = hand.joints[to];
            if (a?.visible && b?.visible) {
                points.setXYZ(vertex++, a.position.x, a.position.y, a.position.z);
                points.setXYZ(vertex++, b.position.x, b.position.y, b.position.z);
            }
        }
        skeleton.setDrawRange(0, vertex);
        points.needsUpdate = true;
    };
}

function setupVrInput(index, sphere, color) {
    const controller = renderer.xr.getController(index);
    const hand = renderer.xr.getHand(index);
    rig.add(controller, hand);
    const updateHand = showTrackedHand(hand, sphere, color);
    const beam = new T.Line(new T.BufferGeometry().setFromPoints([new T.Vector3(), new T.Vector3(0, 0, -1)]),
        new T.LineBasicMaterial({ color }));
    const reach = 6;
    beam.scale.z = reach;
    controller.add(beam);
    const aim = new T.Raycaster(undefined, undefined, 0, reach);
    const rotation = new T.Quaternion();
    function aimedPlaque() {
        controller.getWorldPosition(aim.ray.origin);
        controller.getWorldQuaternion(rotation);
        aim.ray.direction.set(0, 0, -1).applyQuaternion(rotation);
        return aim.intersectObjects(targets)[0];
    }
    // WebXR supplies one select event for the primary controller or hand gesture.
    controller.addEventListener("select", () => {
        if (controller.visible) aimedPlaque()?.object.userData.action();
    });
    return () => {
        updateHand();
        const hit = controller.visible ? aimedPlaque() : undefined;
        beam.scale.z = hit ? hit.distance : reach;
        beam.material.color.setHex(hit ? 0xffffff : color);
    };
}

async function setupVr() {
    if (!navigator.xr || !await navigator.xr.isSessionSupported("immersive-vr")) return;
    renderer.xr.enabled = true;
    renderer.xr.setReferenceSpaceType("local-floor");
    const sphere = new T.SphereGeometry(1, 8, 6);
    vrInputs.push(setupVrInput(0, sphere, 0xffd998), setupVrInput(1, sphere, 0xade7ff));
    vr.hidden = false;
    document.querySelector("#vr-controls").hidden = false;
    vr.addEventListener("click", async () => {
        try {
            const session = await navigator.xr.requestSession("immersive-vr", {
                requiredFeatures: ["local-floor"], optionalFeatures: ["hand-tracking"],
            });
            await renderer.xr.setSession(session);
        } catch {
            valueText.textContent = "The headset session could not start.";
        }
    });
}

try {
    renderer = new T.WebGLRenderer({ antialias: true });
    renderer.setPixelRatio(Math.min(devicePixelRatio, 1.5));
    renderer.domElement.tabIndex = 0;
    renderer.domElement.setAttribute("aria-label", "Datatree gallery movement and selection");
    viewport.append(renderer.domElement);
    document.querySelector("#gallery-navigation").hidden = false;
    function resize() {
        camera.aspect = viewport.clientWidth / Math.max(1, viewport.clientHeight);
        camera.position.z = Math.max(2.55, 3.6 / Math.max(0.25, camera.aspect) - 2.4);
        camera.updateProjectionMatrix();
        renderer.setSize(viewport.clientWidth, viewport.clientHeight);
    }
    resize();
    window.addEventListener("resize", resize);
    renderer.domElement.addEventListener("click", event => {
        renderer.domElement.focus({ preventScroll: true });
        const rect = renderer.domElement.getBoundingClientRect();
        ray.setFromCamera(new T.Vector2((event.clientX - rect.left) / rect.width * 2 - 1,
            1 - (event.clientY - rect.top) / rect.height * 2), camera);
        selectTarget();
    });
    renderer.domElement.addEventListener("keydown", event => {
        if (keyBindings.has(event.code) && !event.metaKey && !event.ctrlKey && !event.altKey) {
            navigationKeys.add(event.code);
            event.preventDefault();
        }
    });
    renderer.domElement.addEventListener("keyup", event => navigationKeys.delete(event.code));
    renderer.domElement.addEventListener("blur", () => navigationKeys.clear());
    window.addEventListener("blur", () => navigationKeys.clear());
    renderer.xr.addEventListener("sessionstart", () => {
        lastFrame = null;
        turnArmed = false;
    });
    renderer.xr.addEventListener("sessionend", () => { lastFrame = null; });
    setupVr().catch(() => { vr.hidden = true; });
    renderer.setAnimationLoop((time, frame) => {
        updateNavigation(time, frame);
        if (renderer.xr.isPresenting) vrInputs.forEach(update => update());
        renderer.render(scene, camera);
    });
} catch {
    viewport.hidden = true;
}
renderTree();
