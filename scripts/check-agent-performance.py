#!/usr/bin/env python3
"""Read-only HTTP parity proof for the agency panel; credentials stay in memory."""
import argparse
import datetime
import json
import os
from pathlib import Path
import statistics
import subprocess
import time
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
METRICS = ("policy_count", "written_premium_kurus", "incurred_claims_kurus", "loss_ratio_bp")


def git(*args):
    env = {**os.environ, "GIT_OPTIONAL_LOCKS": "0"}
    return subprocess.check_output(["git", *args], cwd=ROOT, env=env, text=True).strip()


def demo_password():
    value = os.environ.get("DEMO_PASSWORD")
    if value:
        return value
    path = ROOT / ".env"
    if path.is_file():
        for line in path.read_text().splitlines():
            if line.startswith("DEMO_PASSWORD="):
                return line.split("=", 1)[1].strip().strip("\"").strip("'")
    raise ValueError("DEMO_PASSWORD ortamda veya .env içinde gerekli")


def request(base, path, token=None, body=None):
    headers = {}
    if token:
        headers["Authorization"] = "Bearer " + token
    payload = None
    if body is not None:
        payload = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    start = time.monotonic()
    req = urllib.request.Request(base + path, data=payload, headers=headers)
    with urllib.request.urlopen(req, timeout=30) as response:
        data = json.load(response)
    return data, round((time.monotonic() - start) * 1000, 3)


def login(base, email, password):
    response, _ = request(base, "/api/v1/auth/login", body={"email": email, "password": password})
    return response["access_token"]


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def compare_rows(panel, legacy):
    assert panel["total"] == legacy["total"], "genel toplamlar farklı"
    rows = {row["key"]: row for row in panel["items"]}
    assert len(rows) == len(panel["items"]), "pano anahtarları tekil değil"
    for expected in legacy["items"]:
        actual = rows.pop(expected["key"])
        assert actual["label"] == expected["label"], "satır adı farklı"
        for key in METRICS:
            assert actual[key] == expected[key], "acente metriği farklı: " + key
    for extra in rows.values():
        assert all(extra[key] == 0 for key in METRICS[:-1]), "ek satır sıfır değil"
        assert extra["loss_ratio_bp"] is None, "ek satır oranı boş değil"


def expected_agent_id():
    # Same independent profile lookup used for the original B1 proof; no credentials printed.
    command = [
        "docker", "compose", "exec", "-T", "db", "sh", "-c",
        'exec psql -X -A -t -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"',
    ]
    query = """BEGIN READ ONLY;
SELECT a.id FROM agents a JOIN users u ON u.id=a.user_id
 WHERE u.email='acente@guvenpolice.test';
ROLLBACK;
"""
    result = subprocess.run(command, cwd=ROOT, input=query, text=True, capture_output=True, check=True)
    value = result.stdout.strip()
    assert value.isdigit(), "demo acente profili bulunamadı"
    return value


def run(args, evidence):
    password = demo_password()
    token = login(args.api_url, "muhasebe@guvenpolice.test", password)
    default, _ = request(args.api_url, "/api/v1/reports/agents", token)
    today = datetime.date.fromisoformat(default["to"]) - datetime.timedelta(days=1)
    narrow = urllib.parse.urlencode({"from": str(today - datetime.timedelta(days=30)), "to": str(today)})
    periods = (("all-time", "from=&to="), ("default", ""), ("narrow", narrow))
    prefix = args.output.stem.removesuffix("-consistency")
    for name, query in periods:
        times = []
        for _ in range(args.samples):
            path = "/api/v1/reports/agents" + ("?" + query if query else "")
            panel, elapsed = request(args.api_url, path, token)
            times.append(elapsed)
        bounds = urllib.parse.urlencode({"from": panel["from"] or "", "to": panel["to"] or "", "group_by": "agent"})
        legacy, _ = request(args.api_url, "/api/v1/reports/production?" + bounds, token)
        raw_path = args.output.parent / (prefix + "-" + name + "-raw.json")
        write_json(raw_path, {"tested_commit": evidence["tested_commit"], "panel": panel, "adr18": legacy})
        compare_rows(panel, legacy)
        result = {"period": name, "from": panel["from"], "to": panel["to"], "http_ms": times,
                  "median_ms": statistics.median(times), "total": panel["total"], "result": "PASS"}
        evidence["results"].append(result)
        print(name + ": satır/toplam PASS; medyan " + str(result["median_ms"]) + " ms", flush=True)
    agent = login(args.api_url, "acente@guvenpolice.test", password)
    expected = expected_agent_id()
    own, _ = request(args.api_url, "/api/v1/reports/agents?from=&to=&agent_id=unassigned&include_total=true", agent)
    assert "total" not in own and len(own["items"]) == 1, "AGENT kapsamı genişledi"
    assert own["items"][0]["key"] == expected, "AGENT başka acenteyi gördü"
    evidence.update(agent_parameter_isolation="PASS", expected_agent_id=expected, result="PASS")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-url", default=os.environ.get("API_URL", "http://localhost:8080"))
    parser.add_argument("--output", type=Path, default=ROOT / "docs/testing/b1-http-consistency.json")
    parser.add_argument("--samples", type=int, default=3)
    args = parser.parse_args()
    if args.samples < 1:
        parser.error("samples en az 1 olmalı")
    args.api_url = args.api_url.rstrip("/")
    if not args.output.is_absolute():
        args.output = (Path.cwd() / args.output).resolve()
    evidence = {"tested_commit": git("rev-parse", "HEAD"), "branch": git("branch", "--show-current"),
                "measured_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                "command": "python3 scripts/check-agent-performance.py", "results": [], "token_saved": False,
                "scope": "100k SIM plus current demo/verify records; run without concurrent business mutations"}
    try:
        paths = ("backend", "frontend", "scripts", "openapi.yaml", "requests.http")
        assert not git("diff", "HEAD", "--name-only", "--", *paths), "kod commit edilmemiş"
        assert not git("ls-files", "--others", "--exclude-standard", "--", *paths), "izlenmeyen kaynak dosyası var"
        evidence["source_clean"] = True
        run(args, evidence)
    except Exception as exc:
        evidence.update(result="FAIL", failure=type(exc).__name__ + ": " + str(exc))
        write_json(args.output, evidence)
        print("FAIL: " + evidence["failure"])
        return 1
    write_json(args.output, evidence)
    print("PASS: " + str(args.output) + "; tested_commit=" + evidence["tested_commit"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
