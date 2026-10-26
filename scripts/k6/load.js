import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const base = __ENV.API_URL || 'http://api:8080';
const labels = ['policies', 'quote', 'billing', 'tasks', 'health', 'report', 'payment', 'replay'];
const times = {}, counts = {}, errors = {};
const thresholds = { http_req_failed: ['rate<0.01'], checks: ['rate>0.99'], gp_idempotent: ['rate==1'] };
for (const label of labels) {
  times[label] = new Trend(`gp_duration_${label}`, true);
  counts[label] = new Counter(`gp_count_${label}`);
  errors[label] = new Rate(`gp_errors_${label}`);
  thresholds[`gp_errors_${label}`] = ['rate<0.01'];
  thresholds[`gp_duration_${label}`] = label === 'report' ? ['p(95)<5000', 'p(99)<10000']
    : label === 'health' ? ['p(95)<500'] : ['p(95)<1500', 'p(99)<3000'];
}
const idempotent = new Rate('gp_idempotent');
export const options = {
  scenarios: { mixed: { executor: 'constant-vus', vus: 20, duration: '3m', gracefulStop: '15s' } },
  setupTimeout: '2m', thresholds, summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max'],
};
const headers = token => ({ Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' });
function json(response) { try { return response.json(); } catch (_) { return {}; } }
function must(response, status) { if (response.status !== status) fail(`setup HTTP ${response.status}, expected ${status}`); return json(response); }
function login(name) {
  return must(http.post(`${base}/api/v1/auth/login`, JSON.stringify({ email: `${name}@guvenpolice.test`, password: __ENV.DEMO_PASSWORD }),
    { headers: { 'Content-Type': 'application/json' }, tags: { name: 'setup_login' } }), 200).access_token;
}
function body(customer, plate) {
  return { customer_id: customer, product: 'KASKO', term_start: new Date().toISOString().slice(0, 10),
    ncd_level: 2, vehicle_group: 'C', plate, coverages: ['KASKO_ANA', 'CAM'] };
}
export function setup() {
  const account = login('muhasebe');
  const activity = must(http.get(`${base}/api/v1/system/activity`, { headers: headers(account), tags: { name: 'setup_preflight' } }), 200);
  if (activity.simulation && ['RUNNING', 'LIVE'].includes(activity.simulation.status)) fail('simulator is running');
  const agent = login('acente'), adjuster = login('hasar'), uw = login('underwriter');
  const customers = must(http.get(`${base}/api/v1/customers?limit=100`, { headers: headers(agent), tags: { name: 'setup_customers' } }), 200).items;
  const customer = customers.find(c => c.national_id_masked.endsWith('0001'));
  if (!customer) fail('demo customer absent');
  const run = `${Date.now()}-${Math.floor(Math.random() * 1e9)}`;
  const policies = [];
  // Her VU'nun ayrı poliçesi var; tekrar aynı mali etkiyi üretmemeli.
  for (let i = 0; i < 20; i++) {
    const serial = (Date.now() * 20 + i) % 10000;
    const q = must(http.post(`${base}/api/v1/quotes`, JSON.stringify(body(customer.id, `34LDT${String(serial).padStart(4, '0')}`)),
      { headers: headers(agent), tags: { name: 'setup_quote' } }), 201);
    if (q.status === 'UW_REVIEW') must(http.post(`${base}/api/v1/quotes/${q.id}/review`, JSON.stringify({ decision: 'APPROVE', note: 'k6' }),
      { headers: headers(uw), tags: { name: 'setup_review' } }), 200);
    const h = headers(agent); h['Idempotency-Key'] = `load-${run}-policy-${i}`;
    policies.push(must(http.post(`${base}/api/v1/quotes/${q.id}/convert`, '{}', { headers: h, tags: { name: 'setup_convert' } }), 201).id);
  }
  const to = new Date(); to.setUTCDate(to.getUTCDate() + 1);
  const from = new Date(to); from.setUTCDate(from.getUTCDate() - 30);
  return { agent, account, adjuster, uw, customer: customer.id, run, policies,
    from: from.toISOString().slice(0, 10), to: to.toISOString().slice(0, 10) };
}
let cursor = '';
function measured(label, method, path, token, body, extra) {
  const h = headers(token); Object.assign(h, extra || {});
  const response = http.request(method, `${base}${path}`, body, { headers: h, timeout: '20s', tags: { name: label } });
  times[label].add(response.timings.duration); counts[label].add(1);
  const ok = response.status === (method === 'POST' && label !== 'quote' ? 201 : 200);
  errors[label].add(!ok); check(response, { [`${label} HTTP`]: () => ok });
  return response;
}
export default function (data) {
  const choice = Math.random() * 100;
  const policy = data.policies[__VU - 1];
  if (choice < 30) {
    const r = measured('policies', 'GET', `/api/v1/policies?limit=20${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`, data.uw);
    cursor = json(r).next_cursor || '';
  } else if (choice < 50) measured('quote', 'POST', '/api/v1/quotes/preview', data.agent, JSON.stringify(body(data.customer, '34LDP1234')));
  else if (choice < 70) measured('billing', 'GET', `/api/v1/policies/${policy}/billing`, data.account);
  else if (choice < 80) measured('tasks', 'GET', '/api/v1/claim-tasks?limit=20', data.adjuster);
  else if (choice < 90) measured('health', 'GET', '/health', '');
  else if (choice < 95) measured('report', 'GET', `/api/v1/reports/production?group_by=product&from=${data.from}&to=${data.to}`, data.account);
  else {
    const key = `load-${data.run}-${__VU}-${__ITER}`;
    const payload = JSON.stringify({ amount_kurus: 1, method: 'CARD' });
    const first = measured('payment', 'POST', `/api/v1/policies/${policy}/payments`, data.account, payload, { 'Idempotency-Key': key });
    const replay = measured('replay', 'POST', `/api/v1/policies/${policy}/payments`, data.account, payload, { 'Idempotency-Key': key });
    const ok = first.status === 201 && replay.status === 201 && json(first).id === json(replay).id && first.body === replay.body;
    idempotent.add(ok); check(replay, { 'same payment on replay': () => ok });
  }
  sleep(0.15);
}
export function handleSummary(data) {
  const seconds = data.state.testRunDurationMs / 1000;
  const rows = labels.map(label => {
    const t = data.metrics[`gp_duration_${label}`]?.values || {};
    const n = data.metrics[`gp_count_${label}`]?.values.count || 0;
    return { endpoint: label, n, p50_ms: t.med, p95_ms: t['p(95)'], p99_ms: t['p(99)'],
      error_rate: data.metrics[`gp_errors_${label}`]?.values.rate, requests_per_second: n / seconds };
  });
  const report = { vus: 20, duration_seconds: seconds, rows, metrics: data.metrics,
    notes: 'Setup requests are not included in per-endpoint custom metrics. No tokens/bodies stored.' };
  const line = r => `${r.endpoint}\t${r.n}\t${r.p50_ms?.toFixed(2)}\t${r.p95_ms?.toFixed(2)}\t${r.p99_ms?.toFixed(2)}\t${(100*r.error_rate).toFixed(3)}%\t${r.requests_per_second.toFixed(2)}`;
  return { '/results/summary.json': JSON.stringify(report, null, 2),
    stdout: `endpoint\tN\tp50ms\tp95ms\tp99ms\terror\treq/s\n${rows.map(line).join('\n')}\n` };
}
