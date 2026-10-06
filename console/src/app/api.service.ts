import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { DeployState, PnlRun, RuleViolation, VintageStatus } from './models';

/** Thin wrapper over the FastAPI console API (backend/app/main.py). */
@Injectable({ providedIn: 'root' })
export class ApiService {
  private readonly base = 'http://localhost:8000';

  constructor(private http: HttpClient) {}

  deployState(): Observable<DeployState[]> {
    return this.http.get<DeployState[]>(`${this.base}/deploy-state`);
  }

  vintageStatus(): Observable<VintageStatus[]> {
    return this.http.get<VintageStatus[]>(`${this.base}/vintage-status`);
  }

  fundsWithFailures(): Observable<string[]> {
    return this.http.get<string[]>(`${this.base}/pnl-runs/funds-with-failures`);
  }

  runsForFund(fundId: string): Observable<PnlRun[]> {
    return this.http.get<PnlRun[]>(`${this.base}/pnl-runs/${fundId}`);
  }

  violationsForRun(fundId: string, runDate: string): Observable<RuleViolation[]> {
    return this.http.get<RuleViolation[]>(`${this.base}/pnl-runs/${fundId}/${runDate}/violations`);
  }
}
