import { Component, ElementRef, OnInit, ViewChild } from '@angular/core';
import { CommonModule } from '@angular/common';
import * as d3 from 'd3';
import { ApiService } from './api.service';
import { lttb } from './lttb';
import { DeployState, PnlRun, Point, RuleViolation, VintageStatus } from './models';

/**
 * The console: per-service deploy state, per-vintage validation status, a three-click
 * drill-down from a failed P&L run to the exact record and rule that caused it, and a
 * time-series chart downsampled with largest-triangle-three-buckets. This component holds no
 * authority of its own (the same thin-client rule
 * guarded-instruction-validation/self-service-kafka-topic-platform's consoles already use): it
 * only displays what the backend API already computed.
 */
@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './app.component.html',
  styleUrl: './app.component.css',
})
export class AppComponent implements OnInit {
  @ViewChild('chart', { static: true }) chartRef!: ElementRef<SVGSVGElement>;

  title = 'platform operations console';
  loadError = '';

  deployStates: DeployState[] = [];
  vintageStatuses: VintageStatus[] = [];

  // the three-click drill-down
  failedFunds: string[] = [];
  selectedFund: string | null = null;
  runsForSelectedFund: PnlRun[] = [];
  selectedRun: PnlRun | null = null;
  violationsForSelectedRun: RuleViolation[] = [];
  selectedViolation: RuleViolation | null = null;

  // the downsampled chart
  chartPointCount = 0;
  chartRenderMillis = 0;

  constructor(private api: ApiService) {}

  ngOnInit(): void {
    this.api.deployState().subscribe({
      next: (s) => (this.deployStates = s),
      error: () => (this.loadError = 'Could not reach the backend at http://localhost:8000. Is it running?'),
    });
    this.api.vintageStatus().subscribe((s) => (this.vintageStatuses = s));
    this.api.fundsWithFailures().subscribe((f) => (this.failedFunds = f));
    this.renderDemoChart();

    // Exposed for bench/measure.mjs (Playwright), the same window-function pattern this
    // portfolio's event-exploration-console-profiled benchmark already uses: the full-scale
    // (31,536,000-point) claim is measured by driving this real page in a real headless
    // browser, not estimated from the smaller demo chart above.
    (window as any).runFullScaleRenderBenchmark = (totalPoints: number, threshold: number, spikeIndex: number) => {
      const data: Point[] = new Array(totalPoints);
      for (let i = 0; i < totalPoints; i++) {
        data[i] = { x: i, y: Math.sin(i / 3600) * 5 };
      }
      data[spikeIndex] = { x: spikeIndex, y: 500 };

      const start = performance.now();
      const reduced = lttb(data, threshold);
      this.drawChart(reduced, data[0].x, data[totalPoints - 1].x);
      const elapsedMs = performance.now() - start;

      return {
        elapsedMs,
        outputPoints: reduced.length,
        spikeSurvived: reduced.some((p) => p.y === 500),
      };
    };
  }

  // click 1: pick a fund that has at least one failed run
  selectFund(fundId: string): void {
    this.selectedFund = fundId;
    this.selectedRun = null;
    this.violationsForSelectedRun = [];
    this.selectedViolation = null;
    this.api.runsForFund(fundId).subscribe((runs) => (this.runsForSelectedFund = runs));
  }

  // click 2: pick that fund's failed date
  selectRun(run: PnlRun): void {
    this.selectedRun = run;
    this.selectedViolation = null;
    this.api.violationsForRun(run.fund_id, run.run_date).subscribe((v) => (this.violationsForSelectedRun = v));
  }

  // click 3: pick the violation; this is the record and rule that caused the failure
  selectViolation(v: RuleViolation): void {
    this.selectedViolation = v;
  }

  /** A smaller demo series for live interaction; the 31M-point/400ms claim itself is measured
   * by bench/render-benchmark.spec.ts against this exact same lttb()+D3 render path at full scale. */
  private renderDemoChart(): void {
    const n = 86_400; // one day of 1Hz samples
    const data: Point[] = new Array(n);
    for (let i = 0; i < n; i++) {
      data[i] = { x: i, y: Math.sin(i / 1500) * 10 + (i === 40000 ? 60 : 0) };
    }
    const start = performance.now();
    const reduced = lttb(data, 2000);
    this.drawChart(reduced, data[0].x, data[n - 1].x);
    this.chartRenderMillis = performance.now() - start;
    this.chartPointCount = reduced.length;
  }

  private drawChart(points: Point[], xMin: number, xMax: number): void {
    const svg = d3.select(this.chartRef.nativeElement);
    svg.selectAll('*').remove();
    const width = 600;
    const height = 200;
    const x = d3.scaleLinear().domain([xMin, xMax]).range([0, width]);
    const yExtent = d3.extent(points, (p) => p.y) as [number, number];
    const y = d3.scaleLinear().domain(yExtent).range([height, 0]);
    const line = d3.line<Point>().x((p) => x(p.x)).y((p) => y(p.y));
    svg.attr('viewBox', `0 0 ${width} ${height}`);
    svg.append('path').datum(points).attr('d', line).attr('fill', 'none').attr('stroke', 'steelblue');
  }
}
