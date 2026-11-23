#!/usr/bin/env python3
"""Percorre os fluxos da prova de conceito pela API Gateway e mede latências.

Uso: python3 avaliacao/medir.py [saida.json]

Requer o ambiente completo em execução (./start.sh). Consulte avaliacao/README.md.
"""
import json
import os
import random
import statistics
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = sys.argv[1] if len(sys.argv) > 1 else "resultados.json"
API = os.environ.get("API", "http://127.0.0.1:8080")
SIGNER = os.path.join(ROOT, "prontuarios-cc/build/patient-signer")
KS = os.path.join(ROOT, "key-sharing")
KMS_CLI = os.path.join(KS, "build/kms")
CUSTODIANS = "192.168.56.21:50051,192.168.56.22:50051,192.168.56.23:50051"
IPFS_ORG1 = "http://192.168.56.31:5001"
REPS = int(os.environ.get("REPS", "5"))
# Repetições, por execução, das operações que o fluxo principal executa uma
# única vez (cadastros, consentimentos, eliminação, histórico etc.).
OP_REPS = int(os.environ.get("OP_REPS", "5"))
SIZES = [1024, 10 * 1024, 100 * 1024, 1024 * 1024]

results = {"checks": [], "latency": {}, "envelope": []}


def call(method, path, org, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(API + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if org:
        req.add_header("X-Org", org)
    t0 = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            status, raw = r.status, r.read()
    except urllib.error.HTTPError as e:
        status, raw = e.code, e.read()
    dt = time.perf_counter() - t0
    try:
        payload = json.loads(raw) if raw else {}
    except json.JSONDecodeError:
        payload = {"raw": raw.decode(errors="replace")}
    return status, payload, dt


def lat(name, dt):
    results["latency"].setdefault(name, []).append(dt)


def check(label, ok, detail=""):
    results["checks"].append({"label": label, "ok": bool(ok), "detail": str(detail)[:300]})
    print(("  OK    " if ok else "  FALHA ") + label + ("" if ok else f": {detail}"), flush=True)


def cpf():
    d = [random.randint(0, 9) for _ in range(9)]
    for n in (9, 10):
        s = sum(d[i] * (n + 1 - i) for i in range(n))
        r = 11 - s % 11
        d.append(0 if r > 9 else r)
    return "".join(map(str, d))


def custodian(node, action):
    subprocess.run(["vagrant", "ssh", node, "-c", f"sudo systemctl {action} custodian"],
                   cwd=KS, check=False, capture_output=True)


def ipfs_cat(cid):
    req = urllib.request.Request(f"{IPFS_ORG1}/api/v0/cat?arg={cid}", method="POST")
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read()


def direct_decrypt(key_id, envelope, access_log=None):
    """Solicita a decifração diretamente aos custodiantes, sem passar pela API,
    com o certificado do coordenador, como faria uma API comprometida."""
    path = os.path.join(work, "envelope.enc")
    with open(path, "wb") as f:
        f.write(envelope)
    tls = os.path.join(KS, "build/tls")
    args = [KMS_CLI, "-custodians", CUSTODIANS, "-k", "2",
            "-tls-ca", os.path.join(tls, "ca.crt"),
            "-tls-cert", os.path.join(tls, "coordinator.crt"),
            "-tls-key", os.path.join(tls, "coordinator.key")]
    if access_log:
        args += ["-access-log", access_log]
    cp = subprocess.run(args + ["decrypt", key_id, path], cwd=work, capture_output=True, text=True)
    decrypted = os.path.exists(os.path.join(work, "decifrado_envelope"))
    return cp.returncode, (cp.stdout + cp.stderr).strip(), decrypted


work = tempfile.mkdtemp()
keyfile = os.path.join(work, "patient.key")
nonces = {}  # signatureNonce de cada paciente


def sign(action, pid, payload, key=None):
    n = nonces.get(pid, 0) + 1
    out = subprocess.run([SIGNER, "sign", key or keyfile, action, pid, str(n), json.dumps(payload)],
                         check=True, capture_output=True, text=True).stdout.strip()
    nonces[pid] = n
    return n, out


def new_patient(key, name):
    """Gera o par de chaves e cadastra o paciente em Org1."""
    pub = subprocess.run([SIGNER, "keygen", key], check=True, capture_output=True, text=True).stdout.strip()
    pcpf = cpf()
    st, p, dt = call("POST", "/api/patients", "Org1", {"cpf": pcpf, "name": name, "signingPublicKey": pub})
    return pcpf, st, p, dt


def grant_body(pid, prof, key=None):
    payload = {"professional": prof["id"], "institution": "", "accessLevel": "leituraEscrita",
               "purpose": "acompanhamento clinico", "records": [], "expiresAt": ""}
    n, sig = sign("grantPermission", pid, payload, key)
    return {"professional": prof["@key"], "accessLevel": "leituraEscrita",
            "purpose": "acompanhamento clinico", "nonce": n, "signature": sig}


print("== Disponibilidade ==", flush=True)
st, h, dt = call("GET", "/api/health", None)
results["health"] = h
check("GET /api/health", st == 200, h)

print("== Cadastros ==", flush=True)
for org, cnes, name, cnpj in [("Org1", "2345678", "Hospital A", "11222333000181"),
                              ("Org2", "3456789", "Hospital B", "45997418000153"),
                              ("Org3", "4567890", "Hospital C", "11444777000161")]:
    st, p, dt = call("POST", "/api/institutions", org, {"id": cnes, "name": name, "cnpj": cnpj})
    if st == 200:
        lat("registerInstitution", dt)

profs = {}
for org, nm, reg in [("Org1", "Dra. Medicao A", "CRM-DF 9001"), ("Org2", "Dr. Medicao B", "CRM-DF 9002")]:
    st, p, dt = call("POST", "/api/professionals", org,
                     {"cpf": cpf(), "name": nm, "professionalType": "medico", "councilRegistry": reg})
    check(f"registerProfessional {org}", st == 200, p)
    lat("registerProfessional", dt)
    profs[org] = p["professional"]

pcpf, st, p, dt = new_patient(keyfile, "Paciente Medicao")
check("registerPatient (keygen no KMS + transação)", st == 200, p)
lat("registerPatient", dt)
patient = p["patient"]
pkey, pid = patient["@key"], patient["id"]
kms_key = patient.get("kmsKeyId", "")
check("CPF ausente do ativo paciente", pcpf not in json.dumps(patient), patient)

st, p, dt = call("POST", "/api/patients/lookup", "Org2", {"cpf": pcpf})
lat("getPatientByCpf", dt)
check("getPatientByCpf a partir de Org2", st == 200 and pid in json.dumps(p), p)

print("== Consentimento assinado ==", flush=True)
profA, profB = profs["Org1"], profs["Org2"]
body = grant_body(pid, profA)
n = body["nonce"]
st, p, dt = call("POST", f"/api/patients/{pkey}/permissions", "Org1", body)
lat("grantPermission", dt)
check("grantPermission assinada", st == 200, p)
perm = json.loads(p["result"]) if isinstance(p.get("result"), str) else p.get("result", {})
st, p2, _ = call("POST", f"/api/patients/{pkey}/permissions", "Org1", body)
check("repetição da assinatura recusada", st != 200, p2)
bad = dict(body, accessLevel="leitura")
st, p2, _ = call("POST", f"/api/patients/{pkey}/permissions", "Org1", dict(bad, nonce=n + 1))
check("assinatura sobre campos alterados recusada", st != 200, p2)

print("== Adição e leitura por tamanho ==", flush=True)
cids = []
for size in SIZES:
    for i in range(REPS):
        content = ("x" * size)
        st, p, dt = call("POST", "/api/records", "Org1",
                         {"patient": pkey, "author": profA["@key"], "title": f"medicao {size} {i}",
                          "recordType": "consulta", "content": content})
        if st != 200:
            check(f"addRecord {size}", False, p)
            continue
        lat(f"addRecord_{size}", dt)
        cid = p["cid"]
        cids.append((size, cid))
        for s in p.get("trace", []):
            if s.get("action") == "encryptDocument" and i == 0:
                results["envelope"].append({"size": size, "detail": s.get("detail")})
        st, r, dt = call("POST", f"/api/records/{cid}/access", "Org1", {"professional": profA["@key"]})
        ok = st == 200 and r.get("granted") and len(r.get("document", {}).get("content", "")) == size
        if ok:
            lat(f"accessRecord_{size}", dt)
        else:
            check(f"accessRecord {size}", False, r)
check("adições e leituras autorizadas concluídas", len(cids) == len(SIZES) * REPS, len(cids))

small = [c for s, c in cids if s == SIZES[0]]

print("== Confidencialidade no IPFS ==", flush=True)
cp = subprocess.run(["vagrant", "ssh", "fabric-node-2", "-c", f"ipfs cat {small[0]}"],
                    cwd=os.path.join(ROOT, "fabric-network"), capture_output=True, text=True)
env = cp.stdout
check("envelope recuperado a partir do nó de outra instituição", '"ciphertext"' in env, env[:200])
check("envelope não contém o texto em claro", "medicao" not in env and "xxxxxxxx" not in env, env[:200])

print("== Acesso negado ==", flush=True)
for cid in small:
    st, r, dt = call("POST", f"/api/records/{cid}/access", "Org2", {"professional": profB["@key"]})
    lat("accessDenied", dt)
check("leitura sem permissão negada", st == 200 and not r.get("granted") and r.get("operation") == "ACESSO_NEGADO", r)
denied_log = r.get("accessLog")

print("== Decifração direta nos custodiantes ==", flush=True)
# Simula uma API comprometida: o certificado do coordenador e o envelope são
# conhecidos, mas a decifração depende de uma autorização válida e não
# utilizada no livro-razão.
st, r, _ = call("POST", f"/api/records/{small[0]}/access", "Org1", {"professional": profA["@key"]})
used_log = r.get("accessLog")
check("leitura autorizada pela API (consome a autorização)", st == 200 and "document" in r, r)
env0, env1 = ipfs_cat(small[0]), ipfs_cat(small[1])
cases = [
    ("sem registro de auditoria", env0, None, "registro de auditoria do livro-razão é obrigatório"),
    ("registro de leitura negada", env0, denied_log, "recusada pelo livro-razão"),
    ("registro inexistente", env0, "accessLog:00000000-0000-0000-0000-000000000000", "recusada pelo livro-razão"),
    ("registro já utilizado", env0, used_log, "já autorizou uma decifração"),
    ("registro válido com envelope de outro prontuário", env1, used_log, "não corresponde ao prontuário autorizado"),
]
results["direct_decrypt"] = []
for label, envelope, log, expected in cases:
    rc, out, decrypted = direct_decrypt(kms_key, envelope, log)
    results["direct_decrypt"].append({"case": label, "rc": rc, "output": out[-300:]})
    check(f"custodiantes recusam decifração: {label}", rc != 0 and not decrypted and expected in out, out)

print("== Acesso extraordinário ==", flush=True)
for cid in small:
    st, r, dt = call("POST", f"/api/records/{cid}/emergency", "Org2",
                     {"professional": profB["@key"], "reason": "paciente inconsciente no pronto-socorro"})
    if st == 200 and r.get("granted"):
        lat("emergencyAccess", dt)
    else:
        check("emergencyAccess", False, r)
st, r, dt = call("POST", f"/api/records/{small[0]}/access", "Org2", {"professional": profB["@key"]})
check("uso único do acesso extraordinário", st == 200 and not r.get("granted"), r)
st, r, dt = call("POST", f"/api/records/{small[1]}/emergency", "Org2",
                 {"professional": profB["@key"], "reason": "curta"})
check("justificativa curta recusada", st != 200 or not r.get("granted"), r)

print("== Tolerância a falhas dos custodiantes ==", flush=True)
try:
    custodian("custodian-3", "stop")
    ok2of3 = 0
    for i in range(OP_REPS):
        st, r, dt = call("POST", f"/api/records/{small[i % len(small)]}/access", "Org1",
                         {"professional": profA["@key"]})
        if st == 200 and r.get("granted") and "document" in r:
            ok2of3 += 1
            lat("access_2of3", dt)
        else:
            detail2of3 = r
    check(f"leituras com 2 de 3 custodiantes ({ok2of3} de {OP_REPS})", ok2of3 == OP_REPS,
          "" if ok2of3 == OP_REPS else detail2of3)
    custodian("custodian-2", "stop")
    st, r, dt = call("POST", f"/api/records/{small[2]}/access", "Org1", {"professional": profA["@key"]})
    check("leitura com 1 de 3 custodiantes falha na decifração", st != 200 and "document" not in r, r)
    results["below_threshold_response"] = {"status": st, "body": r}
finally:
    custodian("custodian-2", "start")
    custodian("custodian-3", "start")
time.sleep(3)
st, r, dt = call("POST", f"/api/records/{small[2]}/access", "Org1", {"professional": profA["@key"]})
check("leitura após restaurar os custodiantes", st == 200 and "document" in r, r)

print("== Eliminação e revogação ==", flush=True)
n, sig = sign("invalidateMedicalRecord", pid, {"record": small[3], "reason": "solicitacao do titular"})
st, p, dt = call("POST", f"/api/records/{small[3]}/invalidate", "Org1",
                 {"nonce": n, "signature": sig, "reason": "solicitacao do titular"})
lat("invalidateMedicalRecord", dt)
check("eliminação assinada", st == 200, p)
st, r, _ = call("POST", f"/api/records/{small[3]}/access", "Org1", {"professional": profA["@key"]})
check("leitura de prontuário eliminado negada", st != 200 or not r.get("granted"), r)

permkey = perm.get("permission", {}).get("@key")
permid = perm.get("permission", {}).get("id")
n, sig = sign("revokePermission", pid, {"permission": permid})
st, p, dt = call("POST", f"/api/permissions/{permkey}/revoke", "Org1", {"nonce": n, "signature": sig})
lat("revokePermission", dt)
check("revogação assinada", st == 200, p)
st, r, _ = call("POST", f"/api/records/{small[4]}/access", "Org1", {"professional": profA["@key"]})
check("leitura após revogação negada", st == 200 and not r.get("granted"), r)

print("== Repetição das operações unitárias ==", flush=True)
# Cada repetição cadastra um profissional e um paciente, localiza o paciente
# pelo CPF, concede uma permissão, adiciona um prontuário de 1 KiB, elimina-o e
# revoga a permissão, de modo que as operações executadas uma única vez no
# fluxo principal também tenham amostras suficientes. A adição não é medida
# aqui para não alterar as amostras por tamanho.
done = 0
for i in range(OP_REPS):
    org = ("Org1", "Org2")[i % 2]
    st, p, dt = call("POST", "/api/professionals", org,
                     {"cpf": cpf(), "name": f"Profissional Repeticao {i}", "professionalType": "medico",
                      "councilRegistry": f"CRM-DF {9100 + i}"})
    if st != 200:
        check(f"repetição {i}: registerProfessional", False, p)
        continue
    lat("registerProfessional", dt)

    key = os.path.join(work, f"patient-{i}.key")
    xcpf, st, p, dt = new_patient(key, f"Paciente Repeticao {i}")
    if st != 200:
        check(f"repetição {i}: registerPatient", False, p)
        continue
    lat("registerPatient", dt)
    xpatient = p["patient"]
    xkey, xid = xpatient["@key"], xpatient["id"]

    st, p, dt = call("POST", "/api/patients/lookup", "Org2", {"cpf": xcpf})
    if st != 200 or xid not in json.dumps(p):
        check(f"repetição {i}: getPatientByCpf", False, p)
        continue
    lat("getPatientByCpf", dt)

    st, p, dt = call("POST", f"/api/patients/{xkey}/permissions", "Org1", grant_body(xid, profA, key))
    if st != 200:
        check(f"repetição {i}: grantPermission", False, p)
        continue
    lat("grantPermission", dt)
    xperm = (json.loads(p["result"]) if isinstance(p.get("result"), str) else p.get("result", {}))["permission"]

    st, p, _ = call("POST", "/api/records", "Org1",
                    {"patient": xkey, "author": profA["@key"], "title": f"repeticao {i}",
                     "recordType": "consulta", "content": "x" * SIZES[0]})
    if st != 200:
        check(f"repetição {i}: addRecord", False, p)
        continue
    xcid = p["cid"]

    n, sig = sign("invalidateMedicalRecord", xid, {"record": xcid, "reason": "solicitacao do titular"}, key)
    st, p, dt = call("POST", f"/api/records/{xcid}/invalidate", "Org1",
                     {"nonce": n, "signature": sig, "reason": "solicitacao do titular"})
    if st != 200:
        check(f"repetição {i}: invalidateMedicalRecord", False, p)
        continue
    lat("invalidateMedicalRecord", dt)

    n, sig = sign("revokePermission", xid, {"permission": xperm["id"]}, key)
    st, p, dt = call("POST", f"/api/permissions/{xperm['@key']}/revoke", "Org1", {"nonce": n, "signature": sig})
    if st != 200:
        check(f"repetição {i}: revokePermission", False, p)
        continue
    lat("revokePermission", dt)
    done += 1
check(f"repetições das operações unitárias concluídas ({done} de {OP_REPS})", done == OP_REPS, done)

print("== Histórico ==", flush=True)
for i in range(OP_REPS):
    st, hist, dt = call("GET", f"/api/patients/{pkey}/history", "Org3")
    lat("getAccessHistory", dt)
items = hist if isinstance(hist, list) else hist.get("history", hist.get("result", []))
ops = {}
for it in items if isinstance(items, list) else []:
    ops[it.get("operation")] = ops.get(it.get("operation"), 0) + 1
results["history_ops"] = ops
check("histórico consultado a partir de Org3", st == 200 and ops, hist if not ops else ops)

summary = {}
for k, v in results["latency"].items():
    summary[k] = {"n": len(v), "media": statistics.mean(v), "mediana": statistics.median(v),
                  "min": min(v), "max": max(v),
                  "desvio": statistics.stdev(v) if len(v) > 1 else 0.0}
results["summary"] = summary
with open(OUT, "w") as f:
    json.dump(results, f, indent=2, ensure_ascii=False)
print(json.dumps(summary, indent=1))
failures = sum(1 for c in results["checks"] if not c["ok"])
print("falhas:", failures)
sys.exit(1 if failures else 0)
