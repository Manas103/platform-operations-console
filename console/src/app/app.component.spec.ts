import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { AppComponent } from './app.component';

describe('AppComponent', () => {
  let httpMock: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [AppComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => httpMock.verify());

  it('creates and renders a downsampled demo chart to exactly 2000 points', () => {
    const fixture = TestBed.createComponent(AppComponent);
    fixture.detectChanges();

    httpMock.expectOne('http://localhost:8000/deploy-state').flush([]);
    httpMock.expectOne('http://localhost:8000/vintage-status').flush([]);
    httpMock.expectOne('http://localhost:8000/pnl-runs/funds-with-failures').flush([]);

    expect(fixture.componentInstance.chartPointCount).toBe(2000);
  });

  it('drills down fund -> run -> violation in exactly three selections', () => {
    const fixture = TestBed.createComponent(AppComponent);
    fixture.detectChanges();
    httpMock.expectOne('http://localhost:8000/deploy-state').flush([]);
    httpMock.expectOne('http://localhost:8000/vintage-status').flush([]);
    httpMock.expectOne('http://localhost:8000/pnl-runs/funds-with-failures').flush(['F01']);

    const comp = fixture.componentInstance;
    comp.selectFund('F01');
    httpMock.expectOne('http://localhost:8000/pnl-runs/F01')
      .flush([{ fund_id: 'F01', run_date: '2026-09-01', status: 'failed', true_total: 1, reported_nav: 1 }]);
    expect(comp.runsForSelectedFund.length).toBe(1);

    comp.selectRun(comp.runsForSelectedFund[0]);
    httpMock.expectOne('http://localhost:8000/pnl-runs/F01/2026-09-01/violations')
      .flush([{ rule_name: 'stale_price', rule_category: 'pricing', source_file: 'pricing_2026-09-01.csv', source_row: 42, detail: 'x' }]);
    expect(comp.violationsForSelectedRun.length).toBe(1);

    comp.selectViolation(comp.violationsForSelectedRun[0]);
    expect(comp.selectedViolation?.rule_name).toBe('stale_price');
  });
});
