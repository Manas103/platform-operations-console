"""Platform Operations Console API.

Endpoints:
  GET /deploy-state                                per-service deploy state
  GET /vintage-status                               per-vintage validation status
  GET /pnl-runs/funds-with-failures                 click 1: which funds have a failed run
  GET /pnl-runs/{fund_id}                            click 2: that fund's runs by date
  GET /pnl-runs/{fund_id}/{run_date}/violations      click 3: the rule(s) and record(s) that
                                                       made that run fail

Three clicks (fund -> date -> violation) reach the exact record and rule, never more, for any
failed run this console generates; see the console's own e2e test for the same claim proven by
driving the UI itself, not just this API.
"""
from __future__ import annotations

from dataclasses import asdict

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware

from . import data

app = FastAPI(title="Platform Operations Console", version="0.1.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:4200"],
    allow_methods=["*"],
    allow_headers=["*"],
)

_deploy_states = data.generate_deploy_states()
_vintage_statuses = data.generate_vintage_statuses()
_pnl_runs = data.generate_pnl_runs()


@app.get("/deploy-state")
def deploy_state() -> list[dict]:
    return [asdict(s) for s in _deploy_states]


@app.get("/vintage-status")
def vintage_status() -> list[dict]:
    return [asdict(v) for v in _vintage_statuses]


@app.get("/pnl-runs/funds-with-failures")
def funds_with_failures() -> list[str]:
    failed_funds = {r.fund_id for r in _pnl_runs if r.status == "failed"}
    return sorted(failed_funds)


@app.get("/pnl-runs/{fund_id}")
def runs_for_fund(fund_id: str) -> list[dict]:
    runs = [r for r in _pnl_runs if r.fund_id == fund_id]
    if not runs:
        raise HTTPException(404, f"no runs for fund {fund_id}")
    return [asdict(r) for r in runs]


@app.get("/pnl-runs/{fund_id}/{run_date}/violations")
def violations_for_run(fund_id: str, run_date: str) -> list[dict]:
    run = next((r for r in _pnl_runs if r.fund_id == fund_id and r.run_date == run_date), None)
    if run is None:
        raise HTTPException(404, f"no run for {fund_id} on {run_date}")
    if run.status != "failed":
        return []
    return [asdict(v) for v in data.generate_violations_for_run(fund_id, run_date)]
