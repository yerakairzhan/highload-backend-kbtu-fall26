import http from 'k6/http';

// Fixed arrival rate, not fixed users: we hold offered load constant and watch
// latency, instead of letting latency throttle the offered load.
//
// Env knobs:
//   URL       target base, e.g. https://edge:9443
//   NAME      output file stem in /results
//   RATE      requests/sec (default 500)
//   DURATION  steady duration (default 60s)
//   COLD      "1" disables connection reuse (the handshake experiment)
export const options = {
  insecureSkipTLSVerify: true, // lab only; never in a production-claiming report
  noConnectionReuse: __ENV.COLD === '1',
  summaryTrendStats: ['p(50)', 'p(95)', 'p(99)', 'max'],
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 500),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: Number(__ENV.PREVUS || 200),
      maxVUs: Number(__ENV.MAXVUS || 800),
      gracefulStop: '5s',
    },
  },
};

const URL = __ENV.URL || 'https://edge:9443';

export default function () {
  http.get(`${URL}/api/quote/1`);
}

// Write a compact, dependency-free summary to /results so runs are captured
// even when the terminal scrollback is gone.
export function handleSummary(data) {
  const name = __ENV.NAME || 'run';
  const m = data.metrics;
  const g = (metric, stat) =>
    m[metric] && m[metric].values && m[metric].values[stat] != null
      ? Number(m[metric].values[stat].toFixed(3))
      : null;

  // Achieved RPS = completed iterations / CONFIGURED duration.
  //
  // Why not k6's own http_reqs.rate or state.testRunDurationMs: inside the
  // Docker Desktop VM the k6 process saw single clock jumps (a run that the
  // scenario itself reported as "1m0s" with exactly rate*60 iterations came
  // out as testRunDurationMs=3473006, i.e. 58 minutes, and http_reqs.rate=5.2).
  // The constant-arrival-rate scheduler provably kept wall-clock pace (the
  // iteration count equals rate*duration), so the configured duration is the
  // trustworthy denominator. Raw k6 values are kept for transparency and a
  // flag is raised when they disagree with the configuration. run-all.sh also
  // stamps host wall-clock time around every run as independent evidence.
  const cfgDuration = __ENV.DURATION || '60s';
  const cfgSeconds = (() => {
    let s = 0;
    for (const [, n, u] of cfgDuration.matchAll(/(\d+(?:\.\d+)?)(ms|s|m|h)/g)) {
      s += Number(n) * { ms: 0.001, s: 1, m: 60, h: 3600 }[u];
    }
    return s || 60;
  })();
  const iterations = m.iterations ? m.iterations.values.count : null;
  const durMs = data.state && data.state.testRunDurationMs ? data.state.testRunDurationMs : null;
  const achieved = iterations != null ? Number((iterations / cfgSeconds).toFixed(1)) : null;
  const clockJump = durMs != null && Math.abs(durMs / 1000 - cfgSeconds) > 5;

  const compact = {
    name,
    url: URL,
    cold: options.noConnectionReuse === true,
    rate_target: Number(__ENV.RATE || 500),
    duration: cfgDuration,
    achieved_rps: achieved, // iterations / configured duration
    iterations,
    k6_raw: {
      // kept for transparency; unreliable when clock_jump_suspected is true
      test_run_duration_s: durMs ? Number((durMs / 1000).toFixed(2)) : null,
      http_reqs_rate: m.http_reqs ? Number(m.http_reqs.values.rate.toFixed(1)) : null,
      clock_jump_suspected: clockJump,
    },
    dropped_iterations: m.dropped_iterations ? m.dropped_iterations.values.count : 0,
    http_req_duration_ms: {
      p50: g('http_req_duration', 'p(50)'),
      p95: g('http_req_duration', 'p(95)'),
      p99: g('http_req_duration', 'p(99)'),
      max: g('http_req_duration', 'max'),
    },
    http_req_waiting_ms: { // time to first byte = server's own contribution
      p50: g('http_req_waiting', 'p(50)'),
      p95: g('http_req_waiting', 'p(95)'),
    },
    http_req_connecting_ms: { // TCP connect
      p50: g('http_req_connecting', 'p(50)'),
      p95: g('http_req_connecting', 'p(95)'),
    },
    http_req_tls_handshaking_ms: { // TLS handshake - the number the task asks for
      p50: g('http_req_tls_handshaking', 'p(50)'),
      p95: g('http_req_tls_handshaking', 'p(95)'),
    },
    http_req_blocked_ms: { // waiting for a free connection (pool pressure)
      p50: g('http_req_blocked', 'p(50)'),
      p95: g('http_req_blocked', 'p(95)'),
    },
  };

  const lines = [
    `RUN ${name}  url=${URL}  cold=${compact.cold}`,
    `  achieved_rps=${compact.achieved_rps}  iters=${compact.iterations}  dropped=${compact.dropped_iterations}`,
    `  duration_ms  p50=${compact.http_req_duration_ms.p50}  p95=${compact.http_req_duration_ms.p95}  p99=${compact.http_req_duration_ms.p99}  max=${compact.http_req_duration_ms.max}`,
    `  connecting   p50=${compact.http_req_connecting_ms.p50}  tls p50=${compact.http_req_tls_handshaking_ms.p50}  waiting p50=${compact.http_req_waiting_ms.p50}  blocked p50=${compact.http_req_blocked_ms.p50}`,
    '',
  ].join('\n');

  const out = {};
  out['stdout'] = lines;
  out[`/results/${name}.summary.json`] = JSON.stringify(compact, null, 2);
  out[`/results/${name}.full.json`] = JSON.stringify(data, null, 2);
  return out;
}
