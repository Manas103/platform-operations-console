from fastapi.testclient import TestClient

from app.main import app, _pnl_runs

client = TestClient(app)


def test_deploy_state_lists_every_service():
    resp = client.get("/deploy-state")
    assert resp.status_code == 200
    body = resp.json()
    assert len(body) == 5
    for row in body:
        assert row["status"] in ("healthy", "degraded", "rolling-out")
        assert row["service"] and row["version"] and row["team"]


def test_vintage_status_every_failed_vintage_has_at_least_one_rule_failure():
    resp = client.get("/vintage-status")
    assert resp.status_code == 200
    body = resp.json()
    assert len(body) == 40
    for v in body:
        if v["validation_status"] == "fail":
            assert v["rule_failures"] >= 1
        else:
            assert v["rule_failures"] == 0


def test_funds_with_failures_matches_the_underlying_data_exactly():
    resp = client.get("/pnl-runs/funds-with-failures")
    assert resp.status_code == 200
    expected = sorted({r.fund_id for r in _pnl_runs if r.status == "failed"})
    assert resp.json() == expected
    assert len(expected) > 0, "the synthetic data must produce at least one failed fund to drill into"


def test_three_click_drilldown_reaches_a_rule_and_a_record():
    # click 1: a fund with a failure
    funds = client.get("/pnl-runs/funds-with-failures").json()
    fund_id = funds[0]

    # click 2: that fund's runs, find the failed date
    runs = client.get(f"/pnl-runs/{fund_id}").json()
    failed_run = next(r for r in runs if r["status"] == "failed")

    # click 3: the violations (rule + record) for that run
    violations = client.get(f"/pnl-runs/{fund_id}/{failed_run['run_date']}/violations").json()
    assert len(violations) >= 1
    v = violations[0]
    assert v["rule_name"]
    assert v["source_file"] and v["source_row"] > 0, "the record this violation points to must be identifiable"


def test_violations_for_an_ok_run_is_empty():
    ok_run = next(r for r in _pnl_runs if r.status == "ok")
    resp = client.get(f"/pnl-runs/{ok_run.fund_id}/{ok_run.run_date}/violations")
    assert resp.json() == []


def test_unknown_fund_is_404():
    resp = client.get("/pnl-runs/NOT-A-FUND")
    assert resp.status_code == 404
