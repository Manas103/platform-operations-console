"""Synthetic data for the console's three functional claims: per-service deploy state,
per-vintage validation status, and the P&L drill-down chain. All data is generated here from a
fixed seed; nothing is read from a real deployment, broker or database. The shapes mirror the
sibling projects this console visualizes (self-service-kafka-topic-platform's service-platform
extension for deploy state, investment-data-quality-lineage-gate for vintages and P&L rules),
not a live integration with either.
"""
from __future__ import annotations

import random
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone

SERVICES = ["payments-api", "orders-api", "pricing-api", "reference-data-api", "reporting-api"]
STATUSES = ["healthy", "healthy", "healthy", "degraded", "rolling-out"]

FUNDS = ["F01", "F02", "F03", "F04", "F05"]
RULE_NAMES = [
    "stale_price", "missing_price", "future_dated_price", "negative_price",
    "holdings_to_nav_tie_out", "orphan_position", "duplicate_identifier",
    "missing_benchmark", "fx_rate_missing_for_nonbase_currency",
]


@dataclass
class DeployState:
    service: str
    version: str
    status: str
    last_deploy_at: str
    team: str


@dataclass
class VintageStatus:
    vintage_id: str
    instrument_id: str
    business_date: str
    recorded_at: str
    validation_status: str  # "pass" or "fail"
    rule_failures: int


@dataclass
class PnlRun:
    fund_id: str
    run_date: str
    status: str  # "ok" or "failed"
    true_total: float
    reported_nav: float


@dataclass
class RuleViolation:
    rule_name: str
    rule_category: str
    source_file: str
    source_row: int
    detail: str


_rng = random.Random(42)


def _iso(dt: datetime) -> str:
    return dt.replace(microsecond=0).isoformat()


def generate_deploy_states() -> list[DeployState]:
    now = datetime(2026, 10, 1, tzinfo=timezone.utc)
    out = []
    for i, service in enumerate(SERVICES):
        status = STATUSES[i % len(STATUSES)]
        out.append(DeployState(
            service=service,
            version=f"1.{i}.{_rng.randint(0, 9)}",
            status=status,
            last_deploy_at=_iso(now - timedelta(hours=_rng.randint(1, 72))),
            team=["payments", "orders", "pricing", "reference-data", "reporting"][i],
        ))
    return out


def generate_vintage_statuses(count: int = 40) -> list[VintageStatus]:
    base_date = datetime(2026, 9, 1, tzinfo=timezone.utc)
    out = []
    for i in range(count):
        instrument_id = f"INS-{(i % 12) + 1:04d}"
        business_date = (base_date + timedelta(days=i % 30)).date().isoformat()
        recorded_at = base_date + timedelta(days=i % 30, hours=_rng.randint(0, 23))
        failed = _rng.random() < 0.15
        out.append(VintageStatus(
            vintage_id=f"VIN-{i:05d}",
            instrument_id=instrument_id,
            business_date=business_date,
            recorded_at=_iso(recorded_at),
            validation_status="fail" if failed else "pass",
            rule_failures=_rng.randint(1, 3) if failed else 0,
        ))
    return out


def generate_pnl_runs(days: int = 10) -> list[PnlRun]:
    base_date = datetime(2026, 9, 1, tzinfo=timezone.utc).date()
    out = []
    for d in range(days):
        run_date = (base_date + timedelta(days=d)).isoformat()
        for fund in FUNDS:
            true_total = round(_rng.uniform(8_000_000, 12_000_000), 2)
            # a failed run disagrees with its own true total by more than the 0.5% tie-out
            # tolerance the rule engine in investment-data-quality-lineage-gate uses
            failed = _rng.random() < 0.12
            noise = _rng.uniform(0.01, 0.03) if failed else _rng.uniform(-0.0004, 0.0004)
            reported_nav = round(true_total * (1 + noise), 2)
            out.append(PnlRun(
                fund_id=fund, run_date=run_date,
                status="failed" if failed else "ok",
                true_total=true_total, reported_nav=reported_nav,
            ))
    return out


def generate_violations_for_run(fund_id: str, run_date: str) -> list[RuleViolation]:
    """The second click's worth of detail: the specific rule(s) that made this run fail, each
    with a column-level lineage path, the same shape investment-data-quality-lineage-gate's
    Violation.java already establishes for this portfolio."""
    seed = hash((fund_id, run_date)) & 0xffff
    rng = random.Random(seed)
    n = rng.randint(1, 3)
    out = []
    for i in range(n):
        rule = RULE_NAMES[rng.randint(0, len(RULE_NAMES) - 1)]
        category = "position_fund" if "position" in rule or "tie_out" in rule or "orphan" in rule \
            else "pricing" if "price" in rule else "security_master"
        out.append(RuleViolation(
            rule_name=rule, rule_category=category,
            source_file=f"positions_{run_date}.csv" if category == "position_fund" else f"pricing_{run_date}.csv",
            source_row=rng.randint(2, 50000),
            detail=f"{rule} violated for fund {fund_id} on {run_date}",
        ))
    return out
