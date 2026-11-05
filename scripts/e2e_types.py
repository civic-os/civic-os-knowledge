#!/usr/bin/env python3
"""End-to-end test of config-driven concept types (KB_TYPES) on the local docker stack.

Brings up docker-compose.yaml (MinIO as S3, Keycloak with the civic-os-kb realm,
kb-server built from this tree) as its own compose project, seeds MinIO with a
bundle, and drives the MCP tools over HTTP with Keycloak bearer tokens. Each
scenario recreates kb-server with a different KB_TYPES, so the server pulls the
bundle from S3 the way production pulls from Spaces.

Scenarios:
  A  the Civic OS KB_TYPES from deploy/configmap.yaml.example (creates, moves, S3 sync, viz)
  B  KB_TYPES unset: built-in defaults, orphaned folders warn but stay readable
  C  a type removed from KB_TYPES after concepts exist in its folder
  D  malformed KB_TYPES stops the server

The bundle is synthetic by default (a few concepts per instance folder), so no
real KB content is needed. Pass --bundle DIR to seed a real export instead; it
must contain a projects/ concept and follow the folder/type rule.

Usage:
  python3 scripts/e2e_types.py            # run, then tear the stack down (down -v)
  python3 scripts/e2e_types.py --keep     # leave the stack up afterwards
  python3 scripts/e2e_types.py --bundle ~/kb-export

Needs docker (with compose) and python3; no Python packages. Uses ports 8080,
8082, 9000 and 9001, and compose project "kb-e2e", so it leaves any
default-project stack and its volumes alone.
"""
import argparse
import json
import os
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PROJECT = "kb-e2e"
COMPOSE = ["docker", "compose", "-p", PROJECT, "-f", os.path.join(REPO, "docker-compose.yaml")]
KB = "http://localhost:8080"
TOKEN_URL = "http://localhost:8082/realms/civic-os-kb/protocol/openid-connect/token"
MC_ALIAS = "mc alias set local http://minio:9000 minioadmin minioadmin >/dev/null; "


# ----------------------------------------------------------------------------- config

def instance_types_json():
    """KB_TYPES from deploy/configmap.yaml.example (the `KB_TYPES: |` block). The real
    configmap.yaml is gitignored; TestLocalConfigTypesMatchExample keeps the two in step."""
    lines, inside = [], False
    for line in open(os.path.join(REPO, "deploy", "configmap.yaml.example")):
        if re.match(r"^\s+KB_TYPES: \|\s*$", line):
            inside = True
            continue
        if inside:
            if line.startswith("    "):
                lines.append(line[4:].rstrip("\n"))
            elif line.strip():
                break
    if not lines:
        sys.exit("deploy/configmap.yaml.example has no KB_TYPES block")
    return "\n".join(lines)


def default_folders():
    with open(os.path.join(REPO, "internal", "bundle", "default_types.json")) as f:
        return [t["folder"] for t in json.load(f)]


# ----------------------------------------------------------------------------- bundle

def synthetic_bundle(dest, types, per_folder=3):
    """A few stub concepts per folder, typed by the folder. module/ starts empty so
    scenario A can count what it creates."""
    for t in types:
        if t["folder"] == "module":
            continue
        for i in range(1, per_folder + 1):
            write_concept(dest, f"{t['folder']}/sample-{i}.md", t["name"], f"Sample {i}")


def write_concept(root, path, ctype, title, body=None):
    full = os.path.join(root, path)
    os.makedirs(os.path.dirname(full), exist_ok=True)
    with open(full, "w") as f:
        f.write(f"---\ntype: {ctype}\ntitle: {title}\n---\n\n{body or 'Stub of ' + path + '.'}\n")


def bundle_paths(root):
    out = []
    for d, _, files in os.walk(root):
        out += [os.path.relpath(os.path.join(d, f), root) for f in files if f.endswith(".md")]
    return sorted(out)


# ----------------------------------------------------------------------------- stack

def compose(*args, override=None, check=True):
    cmd = COMPOSE + (["-f", override] if override else []) + list(args)
    return subprocess.run(cmd, cwd=REPO, check=check, capture_output=True, text=True)


def write_override(path, kb_types):
    block = "\n".join("        " + line for line in kb_types.splitlines()) or "        "
    with open(path, "w") as f:
        f.write(f"services:\n  kb-server:\n    environment:\n      KB_TYPES: |\n{block}\n")


def mc(seed_dir, script):
    cmd = ["docker", "run", "--rm", "--network", f"{PROJECT}_default", "-v", f"{seed_dir}:/seed:ro",
           "--entrypoint", "sh", "minio/mc", "-c", MC_ALIAS + script]
    return subprocess.run(cmd, capture_output=True, text=True).stdout


def token(user="kbadmin"):
    # kbadmin/kbadmin is a fixture user in deploy/keycloak/civic-os-kb-realm.json (local only)
    data = urllib.parse.urlencode(dict(grant_type="password", client_id="civic-os-kb",
                                       client_secret="civic-os-kb-secret", username=user,
                                       password=user, scope="openid")).encode()
    return json.load(urllib.request.urlopen(TOKEN_URL, data, timeout=10))["access_token"]


class Stack:
    def __init__(self, tmp, seed_dir, expected_count):
        self.tmp, self.seed_dir, self.expected_count = tmp, seed_dir, expected_count

    def reseed(self):
        """Reset S3 to the seed bundle; the next server start pulls it."""
        out = mc(self.seed_dir,
                 "mc rm -r --force local/civic-os-kb/knowledge local/civic-os-kb/versions >/dev/null 2>&1; "
                 "mc mirror -q /seed local/civic-os-kb/knowledge/ >/dev/null; "
                 "mc ls -r local/civic-os-kb/knowledge | wc -l")
        if out.strip() != str(self.expected_count):
            sys.exit(f"seeding MinIO failed: expected {self.expected_count} objects, got {out.strip()!r}")

    def s3_keys(self):
        listing = mc(self.seed_dir, "mc ls -r local/civic-os-kb/knowledge")
        return [line.split()[-1] for line in listing.splitlines() if line.strip()]

    def server(self, kb_types=None):
        return Server(self, kb_types)


class Server:
    """kb-server recreated with the given KB_TYPES (None = unset)."""

    def __init__(self, stack, kb_types):
        override = None
        if kb_types is not None:
            override = os.path.join(stack.tmp, "override-run.yaml")
            write_override(override, kb_types)
        compose("up", "-d", "--no-deps", "--force-recreate", "kb-server", override=override)
        self.cid = compose("ps", "-a", "-q", "kb-server").stdout.split()[0]
        self.tmp = stack.tmp
        for _ in range(150):
            if self.state() == "exited":
                break
            try:
                urllib.request.urlopen(f"{KB}/health", timeout=0.3)
                break
            except Exception:
                time.sleep(0.2)
        self.id = 0
        self.tok = token() if self.state() == "running" else None

    def state(self):
        return subprocess.run(["docker", "inspect", "-f", "{{.State.Status}}", self.cid],
                              capture_output=True, text=True).stdout.strip()

    def exit_code(self):
        for _ in range(50):
            if self.state() == "exited":
                break
            time.sleep(0.1)
        out = subprocess.run(["docker", "inspect", "-f", "{{.State.ExitCode}}", self.cid],
                             capture_output=True, text=True).stdout
        return int(out or -1)

    def log(self):
        r = subprocess.run(["docker", "logs", self.cid], capture_output=True, text=True)
        return r.stdout + r.stderr

    def viz(self):
        time.sleep(0.6)  # viz regeneration is debounced by 250ms
        dest = os.path.join(self.tmp, "viz.html")
        subprocess.run(["docker", "cp", f"{self.cid}:/data/knowledge/viz.html", dest], check=True, capture_output=True)
        with open(dest) as f:
            return f.read()

    def viz_colors(self):
        return json.loads(re.search(r"const typeColors = (\{.*?\});", self.viz()).group(1))

    def rpc(self, method, params=None, tok=None):
        self.id += 1
        body = json.dumps({"jsonrpc": "2.0", "id": self.id, "method": method, "params": params or {}}).encode()
        req = urllib.request.Request(f"{KB}/mcp", body, {
            "Content-Type": "application/json", "Accept": "application/json, text/event-stream",
            "Authorization": f"Bearer {tok or self.tok}"})
        return json.load(urllib.request.urlopen(req, timeout=15))

    def tool(self, name, **args):
        r = self.rpc("tools/call", {"name": name, "arguments": args})["result"]
        return r.get("isError", False), "\n".join(c.get("text", "") for c in r.get("content", []))

    def stop(self):
        compose("stop", "kb-server")  # SIGTERM flushes pending S3 writes


# ----------------------------------------------------------------------------- checks

results = []


def check(name, ok, detail=""):
    results.append((name, bool(ok)))
    print(("PASS " if ok else "FAIL ") + name + ("" if ok else f"\n     {str(detail)[:600]}"))


def scenario_a(stack, instance, paths):
    print("\n== A: instance KB_TYPES")
    types = json.loads(instance)
    folders = [t["folder"] for t in types]
    n = len(paths)
    spec = next(p for p in paths if p.startswith("projects/"))
    spec_name = os.path.basename(spec)
    modules = sum(p.startswith("module/") for p in paths)  # a real export may already have some

    stack.reseed()
    srv = stack.server(instance)
    log = srv.log()
    check(f"A1 startup reads KB_TYPES with all {len(folders)} folders",
          "concept types from KB_TYPES: " + ", ".join(f + "/" for f in folders) in log, log)
    check("A2 no type warning at startup", "don't match" not in log, log)
    check(f"A3 pulled and indexed all {n} concepts from S3", f"indexed {n} concepts" in log, log)

    try:
        srv.rpc("tools/list", tok="not-a-token")
        check("A4 MCP rejects a bad bearer token", False, "accepted")
    except urllib.error.HTTPError as e:
        check("A4 MCP rejects a bad bearer token", e.code == 401, e.code)

    tools = {t["name"]: t for t in srv.rpc("tools/list")["result"]["tools"]}
    check("A5 nine tools registered", len(tools) == 9, list(tools))
    for t in ("kb_create", "kb_move"):
        missing = [f for f in folders if f"\n- {f}/ — " not in tools[t]["description"]]
        check(f"A6 {t} description lists every folder", not missing, f"missing {missing}")

    _, out = srv.tool("kb_list")
    check(f"A7 kb_list sees {n} concepts", out.startswith(f"{n} concept(s)"), out[:80])
    _, out = srv.tool("kb_links")
    check("A8 audit: 0 folder/type mismatches", "Folder/type mismatches (0)" in out, out)

    err, out = srv.tool("kb_create", path="module/vendors.md", title="Vendors",
                        body=f"Part of [the spec](/{spec}).")
    check("A9 create in module/ infers Project Module", not err and "(Project Module, version: 1)" in out, out)
    err, out = srv.tool("kb_create", path="module/lending.md", type="project module", title="Lending")
    check("A10 type matching is case-insensitive", not err and "(Project Module, version: 1)" in out, out)
    err, out = srv.tool("kb_create", path="module/x.md", type="Project Specification", title="X")
    check("A11 another folder's type is rejected, pointing to that folder",
          err and "Project Specification concepts live in projects/ (e.g. projects/x.md)" in out, out)
    err, out = srv.tool("kb_create", path="modules/x.md", title="X")
    check("A12 unregistered folder rejected, listing module/",
          err and "not in a registered folder" in out and "module/" in out, out)
    err, out = srv.tool("kb_create", path="module/y.md", type="Feature Module", title="Y")
    check("A13 unknown type rejected", err and 'unknown type "Feature Module"; module/ holds Project Module' in out, out)

    moved = f"module/{spec_name}"
    err, out = srv.tool("kb_move", moves=[{"path": spec, "new_path": moved}])
    check("A14 kb_move projects/ → module/ changes the type",
          not err and f"{spec} → {moved}" in out and "type Project Specification → Project Module" in out, out)
    err, out = srv.tool("kb_read", path=moved)
    check("A15 moved concept reads as Project Module with its old path as an alias",
          not err and "type: Project Module" in out and spec in out, out[:400])
    _, out = srv.tool("kb_read", path="module/vendors.md")
    check("A16 inbound link rewritten to module/", f"(/{moved})" in out, out[-200:])
    err, out = srv.tool("kb_read", path=spec)
    check("A17 read of the old path follows the alias", not err and f"moved: {spec} → {moved}" in out, out[:200])
    err, out = srv.tool("kb_diff", path=moved, version=1)
    check("A18 diff shows the type change", "-type: Project Specification" in out and "+type: Project Module" in out, out[:600])

    err, out = srv.tool("kb_update", path="module/lending.md", type="Project Specification")
    check("A19 update to another folder's type rejected with a kb_move hint",
          err and "move it into that type's folder with kb_move" in out, out)
    err, out = srv.tool("kb_update", path="module/lending.md", description="Lending module")
    check("A20 normal update in module/ succeeds without a warning", not err and "Type warning" not in out, out)

    _, out = srv.tool("kb_list", type="Project Module")
    check(f"A21 kb_list by Project Module returns {modules + 3}", out.startswith(f"{modules + 3} concept(s)"), out[:200])
    _, out = srv.tool("kb_search", query="vendors lending", type="Project Module")
    check("A22 search filters by Project Module", "module/vendors.md" in out and "module/lending.md" in out, out[:400])
    err, out = srv.tool("kb_move", moves=[{"path": "module/lending.md", "new_path": "widgets/lending.md"}])
    check("A23 move into an unregistered folder is rejected, nothing moved", err and "Nothing was moved" in out, out)
    _, out = srv.tool("kb_links")
    check("A24 audit still clean after writes", "Folder/type mismatches (0)" in out and "Broken links (0)" in out, out)

    colors = srv.viz_colors()
    check(f"A25 viz colors one per type ({len(types)}), configured ones as given",
          set(colors) == {t["name"] for t in types}
          and all(colors[t["name"]] == t["color"] for t in types if t.get("color")), colors)
    check("A26 viz draws Project Module nodes", '"type":"Project Module"' in srv.viz())

    srv.stop()
    keys = set(stack.s3_keys())
    check("A27 S3 holds the new module/ keys and dropped the moved one",
          {"module/vendors.md", "module/lending.md", moved} <= keys and spec not in keys and len(keys) == n + 2, len(keys))
    srv = stack.server(instance)
    _, out = srv.tool("kb_list", type="Project Module")
    log = srv.log()
    check(f"A28 a fresh container pulls {n + 2} concepts from S3 with no warnings",
          f"indexed {n + 2} concepts" in log and "don't match" not in log
          and out.startswith(f"{modules + 3} concept(s)"), log + out[:100])
    srv.stop()


def scenario_b(stack, paths):
    print("\n== B: KB_TYPES unset (built-in defaults)")
    defaults = default_folders()
    orphans = {}
    for p in paths:
        f = p.split("/")[0]
        if f not in defaults:
            orphans[f] = orphans.get(f, 0) + 1
    total = sum(orphans.values())
    orphan = next(p for p in paths if p.split("/")[0] in orphans)

    stack.reseed()
    srv = stack.server(None)
    log = srv.log()
    check("B1 startup says built-in defaults",
          "concept types from built-in defaults (KB_TYPES unset): " + ", ".join(f + "/" for f in defaults) in log, log)
    want = (f"WARNING: {total} concepts don't match the concept types (unregistered folder or stray type): "
            + ", ".join(f"{f}/ ({c})" for f, c in sorted(orphans.items())) + ".")
    check(f"B2 warning names the {len(orphans)} orphaned folders with exact counts", want in log, log)
    err, out = srv.tool("kb_create", path=f"{orphan.split('/')[0]}/new.md", title="New")
    check("B3 create in an orphaned folder is blocked", err and "not in a registered folder" in out, out)
    err, out = srv.tool("kb_read", path=orphan)
    check("B4 orphaned concepts stay readable", not err, out[:200])
    err, out = srv.tool("kb_update", path=orphan, description="still editable")
    check("B5 orphaned concepts stay updatable, with a type warning",
          not err and f"Type warning: {orphan} is outside the registered folders" in out, out)
    _, out = srv.tool("kb_links")
    check(f"B6 audit lists {total} mismatches", f"Folder/type mismatches ({total})" in out, out[:300])
    err, out = srv.tool("kb_create", path=f"{defaults[0]}/x.md", title="X")
    check("B7 default folders stay writable", not err and "version: 1" in out, out)
    srv.stop()


def scenario_c(stack, instance):
    print("\n== C: a type removed from KB_TYPES after concepts exist")
    stack.reseed()
    srv = stack.server(instance)
    srv.tool("kb_create", path="module/a.md", title="A")
    srv.tool("kb_create", path="module/b.md", title="B")
    srv.stop()
    srv = stack.server(json.dumps([t for t in json.loads(instance) if t["folder"] != "module"]))
    log = srv.log()
    check("C1 startup warns about module/ (2)", "module/ (2)" in log, log)
    err, out = srv.tool("kb_create", path="module/c.md", title="C")
    check("C2 create in module/ is blocked", err and "not in a registered folder" in out, out)
    err, out = srv.tool("kb_move", moves=[{"path": "module/a.md", "new_path": "projects/a.md"}])
    check("C3 orphans can be moved out to a registered folder", not err and "module/a.md → projects/a.md" in out, out)
    check("C4 viz drops the removed type's color", "Project Module" not in srv.viz_colors())
    srv.stop()


def scenario_d(stack, instance):
    print("\n== D: malformed KB_TYPES stops the server")
    cases = [
        ("D1 invalid JSON", '[{"name":', "parse type registry"),
        ("D2 unknown field", '[{"name":"A","folder":"a","holds":"x","colour":"#000000"}]', 'unknown field "colour"'),
        ("D3 empty array", "[]", "type registry is empty"),
        ("D4 duplicate folder", json.dumps(json.loads(instance) + [{"name": "Other", "folder": "module", "holds": "x"}]),
         'folder "module" is already registered'),
        ("D5 bad color", '[{"name":"A","folder":"a","holds":"x","color":"blue"}]', 'color "blue" is not #rrggbb'),
        ("D6 nested folder", '[{"name":"A","folder":"a/b","holds":"x"}]', "not a valid top-level folder"),
        ("D7 object instead of array", '{"name":"A","folder":"a","holds":"x"}', "parse type registry"),
    ]
    for name, kb_types, expect in cases:
        srv = stack.server(kb_types)
        code, log = srv.exit_code(), srv.log()
        check(name, code != 0 and expect in log, f"exit={code} log={log[-300:]}")
    srv = stack.server("  \n ")
    check("D8 whitespace-only KB_TYPES falls back to the defaults", "built-in defaults" in srv.log(), srv.log())
    srv.stop()


# ----------------------------------------------------------------------------- main

def port_open(port):
    with socket.socket() as s:
        return s.connect_ex(("localhost", port)) == 0


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--bundle", help="seed this bundle directory instead of a synthetic one")
    ap.add_argument("--keep", action="store_true", help="leave the stack running afterwards")
    args = ap.parse_args()

    instance = instance_types_json()
    tmp = tempfile.mkdtemp(prefix="kb-e2e-")
    seed = os.path.join(tmp, "bundle")
    if args.bundle:
        shutil.copytree(os.path.expanduser(args.bundle), seed)
    else:
        synthetic_bundle(seed, json.loads(instance))
    paths = bundle_paths(seed)
    if not any(p.startswith("projects/") for p in paths):
        sys.exit("the bundle needs at least one projects/ concept")

    up = compose("ps", "-q", "kb-server", check=False).stdout.strip()
    if not up:
        busy = [p for p in (8080, 8082, 9000, 9001) if port_open(p)]
        if busy:
            sys.exit(f"ports in use: {busy}; stop whatever holds them first")

    print(f"Starting the {PROJECT} stack (MinIO, Keycloak, kb-server built from {REPO}) …")
    override = os.path.join(tmp, "override-instance.yaml")
    write_override(override, instance)
    r = compose("up", "-d", "--build", "--wait", override=override, check=False)
    if r.returncode != 0:
        sys.exit(f"docker compose up failed:\n{r.stdout}{r.stderr}")
    print(f"Seeding {len(paths)} concepts into MinIO for each scenario.")

    stack = Stack(tmp, seed, len(paths))
    try:
        scenario_a(stack, instance, paths)
        scenario_b(stack, paths)
        scenario_c(stack, instance)
        scenario_d(stack, instance)
    finally:
        if args.keep:
            print(f"\nStack left running (compose project {PROJECT}). Tear down with:\n"
                  f"  docker compose -p {PROJECT} down -v")
        else:
            compose("down", "-v", check=False)
        shutil.rmtree(tmp, ignore_errors=True)

    failed = [n for n, ok in results if not ok]
    print(f"\n{len(results) - len(failed)}/{len(results)} passed")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
