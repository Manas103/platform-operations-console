export interface DeployState {
  service: string;
  version: string;
  status: 'healthy' | 'degraded' | 'rolling-out';
  last_deploy_at: string;
  team: string;
}

export interface VintageStatus {
  vintage_id: string;
  instrument_id: string;
  business_date: string;
  recorded_at: string;
  validation_status: 'pass' | 'fail';
  rule_failures: number;
}

export interface PnlRun {
  fund_id: string;
  run_date: string;
  status: 'ok' | 'failed';
  true_total: number;
  reported_nav: number;
}

export interface RuleViolation {
  rule_name: string;
  rule_category: string;
  source_file: string;
  source_row: number;
  detail: string;
}

/** One raw time-series sample: seconds since series start, and its value. */
export interface Point {
  x: number;
  y: number;
}
