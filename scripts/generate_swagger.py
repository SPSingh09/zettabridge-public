#!/usr/bin/env python3
"""Generate OpenAPI 2.0 spec from internal/handler/swag_*.go annotations.

Run: python3 scripts/generate_swagger.py
Requires: Python 3.9+ (stdlib only)
"""
from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SWAG_DOCS = ROOT / "internal/handler/swag_docs.go"
OUT_YAML = ROOT / "swagger/swagger.yaml"
OUT_JSON = ROOT / "swagger/swagger.json"
OUT_GO = ROOT / "swagger/docs.go"

REF = "#/definitions/handler.{name}"


def ref(name: str) -> dict:
    return {"$ref": REF.format(name=name)}


def wrap_data(schema: dict) -> dict:
    return {
        "allOf": [
            ref("APIResponse"),
            {"type": "object", "properties": {"data": schema}},
        ]
    }


def err(*codes: str) -> dict:
    return {c: {"description": c, "schema": ref("ErrorResponse")} for c in codes}


def parse_routes() -> list[dict]:
    text = SWAG_DOCS.read_text(encoding="utf-8")
    blocks = re.split(r"\n// (\w+)Doc godoc\n", text)[1:]
    routes = []
    for i in range(0, len(blocks), 2):
        _name, body = blocks[i], blocks[i + 1]
        m = re.search(r"@Router\s+(\S+)\s+\[(\w+)\]", body)
        if not m:
            continue
        path, method = m.group(1), m.group(2)
        summary = re.search(r"@Summary\s+(.+)", body)
        desc = re.search(r"@Description\s+(.+)", body)
        tags = re.findall(r"@Tags\s+(\S+)", body)
        sec = re.findall(r"@Security\s+(\S+)", body)
        consumes = re.findall(r"@Accept\s+(\S+)", body)
        produces = re.findall(r"@Produce\s+(\S+)", body)
        params = []
        for p in re.finditer(
            r"@Param\s+(\w+)\s+(\w+)\s+(\S+)\s+(\w+)\s+\"([^\"]*)\"",
            body,
        ):
            pname, loc, typ, req, desc_txt = p.groups()
            entry = {
                "name": pname,
                "in": loc,
                "description": desc_txt,
                "required": req.lower() == "true",
            }
            if loc == "body":
                entry["schema"] = ref(typ)
            else:
                entry["type"] = typ
            params.append(entry)
        responses = {}
        for r in re.finditer(
            r'@Success\s+(\d+)\s+(.+?)(?=\n// @|\nfunc |\Z)',
            body,
            re.S,
        ):
            code, rest = r.group(1), r.group(2).strip()
            if "No content" in rest or '"No content"' in rest:
                responses[code] = {"description": "No content"}
                continue
            if "Switching Protocols" in rest:
                responses[code] = {"description": "Switching Protocols"}
                continue
            if "Redirect" in rest:
                responses[code] = {"description": "Redirect"}
                continue
            m2 = re.search(r"\{object\}\s+(\S+)", rest)
            if m2:
                typ = m2.group(1)
                if typ == "APIResponse":
                    responses[code] = {
                        "description": rest.split("{object}")[0].strip(),
                        "schema": ref("APIResponse"),
                    }
                elif typ.startswith("APIResponse{data="):
                    inner = typ[len("APIResponse{data=") : -1]
                    if inner.startswith("[]"):
                        inner = inner[2:]
                        schema = {"type": "array", "items": ref(inner)}
                    else:
                        schema = ref(inner)
                    responses[code] = {
                        "description": re.sub(r"\{object\}.*", "", rest).strip(),
                        "schema": wrap_data(schema),
                    }
                elif typ == "map[string]string":
                    responses[code] = {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {"type": "string"},
                        },
                    }
                else:
                    responses[code] = {
                        "description": rest,
                        "schema": ref(typ),
                    }
            elif "string" in rest and "CSV" in rest:
                responses[code] = {"description": "CSV export", "schema": {"type": "string"}}
            else:
                responses[code] = {"description": rest}
        for r in re.finditer(r"@Failure\s+(\d+)", body):
            c = r.group(1)
            if c not in responses:
                responses[c] = {"description": "Error", "schema": ref("ErrorResponse")}

        op = {
            "summary": summary.group(1).strip() if summary else path,
            "responses": responses,
        }
        if desc:
            op["description"] = desc.group(1).strip()
        if tags:
            op["tags"] = tags
        if sec:
            op["security"] = [{s: []} for s in sec]
        if consumes:
            op["consumes"] = [
                "application/json" if c == "json" else c for c in consumes
            ]
        if produces:
            op["produces"] = []
            for p in produces:
                if p == "json":
                    op["produces"].append("application/json")
                elif p == "plain":
                    op["produces"].append("text/plain")
                elif p == "text/csv":
                    op["produces"].append("text/csv")
                else:
                    op["produces"].append(p)
        if params:
            op["parameters"] = params
        routes.append({"path": path, "method": method, "op": op})
    return routes


def load_definitions() -> dict:
    """Build definitions from swag_types.go struct tags (simplified enums)."""
    types_go = (ROOT / "internal/handler/swag_types.go").read_text(encoding="utf-8")
    defs: dict = {}

    # Hand-maintained critical schemas (parsed types get basic object shells).
    enum_fixes = {
        "AdminSetPlanRequest": {"plan": ["free", "paper", "pro", "pro_plus"]},
        "MeResponse": {"plan": ["free", "paper", "pro", "pro_plus"]},
        "CheckoutRequest": {"plan": ["paper", "pro", "pro_plus"]},
        "CreateCredentialRequest": {
            "account_mode": ["live"],
            "broker_type": ["zerodha", "angel", "dhan", "mt5_cloud", "mock"],
        },
        "CredentialResponse": {
            "account_mode": ["live"],
            "broker_type": ["zerodha", "angel", "dhan", "mt5_cloud", "mock"],
        },
    }

    for m in re.finditer(r"type (\w+) struct \{([^}]+)\}", types_go, re.S):
        name, body = m.group(1), m.group(2)
        props = {}
        for fm in re.finditer(
            r"(\w+)\s+([\w\[\]*]+)\s+`json:\"([^\"]+)\"(?:\s+example:\"([^\"]*)\")?(?:\s+enums:([^`]+))?`",
            body,
        ):
            field, go_type, json_key, example, enums = fm.groups()
            if json_key.endswith(",omitempty"):
                json_key = json_key.replace(",omitempty", "")
            prop: dict = {}
            if enums:
                prop["type"] = "string"
                prop["enum"] = [e.strip() for e in enums.split(",")]
            elif go_type in ("string", "*string"):
                prop["type"] = "string"
            elif go_type in ("int", "*int"):
                prop["type"] = "integer"
            elif go_type in ("float64", "*float64"):
                prop["type"] = "number"
            elif go_type == "bool":
                prop["type"] = "boolean"
            elif go_type.startswith("[]"):
                prop["type"] = "array"
                prop["items"] = {"type": "string"}
            else:
                prop["type"] = "string"
            if example:
                if prop.get("type") == "integer":
                    prop["example"] = int(example) if example.isdigit() else example
                elif prop.get("type") == "number":
                    try:
                        prop["example"] = float(example)
                    except ValueError:
                        prop["example"] = example
                elif prop.get("type") == "boolean":
                    prop["example"] = example.lower() == "true"
                else:
                    prop["example"] = example
            props[json_key] = prop
        if name in enum_fixes:
            for k, vals in enum_fixes[name].items():
                if k in props:
                    props[k]["enum"] = vals
                    if vals:
                        props[k]["example"] = vals[0]
        defs[name] = {"type": "object", "properties": props}

    defs["APIResponse"] = {"type": "object", "properties": {"data": {}}}
    return defs


def build_spec() -> dict:
    paths: dict = {}
    for r in parse_routes():
        paths.setdefault(r["path"], {})[r["method"]] = r["op"]

    return {
        "swagger": "2.0",
        "info": {
            "title": "ZettaBridge API",
            "description": "REST and WebSocket API for ZettaBridge — TradingView webhook ingestion, paper and live broker execution, billing, and platform admin.",
            "version": "1.0",
            "contact": {"name": "ZettaBridge", "url": "https://zettabridge.net"},
        },
        "host": "api.staging.zettabridge.net",
        "basePath": "/",
        "schemes": ["https", "http"],
        "securityDefinitions": {
            "BearerAuth": {
                "type": "apiKey",
                "name": "Authorization",
                "in": "header",
                "description": 'JWT access token from POST /v1/auth/login. Prefix with "Bearer ".',
            },
            "ServiceToken": {
                "type": "apiKey",
                "name": "Authorization",
                "in": "header",
                "description": "Adapter→Core service token (Bearer ZB_SERVICE_TOKEN).",
            },
        },
        "paths": paths,
        "definitions": {f"handler.{k}": v for k, v in load_definitions().items()},
    }


def write_yaml(spec: dict, path: Path) -> None:
    try:
        import yaml  # type: ignore
    except ImportError:
        path.write_text(json.dumps(spec, indent=2), encoding="utf-8")
        return
    path.write_text(yaml.dump(spec, sort_keys=False, allow_unicode=True, width=120), encoding="utf-8")


def write_docs_go(json_spec: dict, path: Path) -> None:
    payload = json.dumps(json_spec, indent=4)
    # Escape backticks for raw string
    content = f"""//go:build ignore

// Package swagger Code generated by scripts/generate_swagger.py. DO NOT EDIT.
package swagger

import "github.com/swaggo/swag"

const docTemplate = `{payload}`

var SwaggerInfo = &swag.Spec{{
\tVersion:          "1.0",
\tHost:             "api.staging.zettabridge.net",
\tBasePath:         "/",
\tSchemes:          []string{{"https", "http"}},
\tTitle:            "ZettaBridge API",
\tDescription:       "REST and WebSocket API for ZettaBridge",
\tInfoInstanceName: "swagger",
\tSwaggerTemplate:  docTemplate,
\tLeftDelim:        "{{{{",
\tRightDelim:       "}}}}",
}}

func init() {{
\tswag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}}
"""
    path.write_text(content, encoding="utf-8")


def main() -> None:
    spec = build_spec()
    OUT_JSON.write_text(json.dumps(spec, indent=2) + "\n", encoding="utf-8")
    write_yaml(spec, OUT_YAML)
    write_docs_go(spec, OUT_GO)
    print(f"Generated {len(spec['paths'])} paths -> {OUT_JSON}")


if __name__ == "__main__":
    main()
