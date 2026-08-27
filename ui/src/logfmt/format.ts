/**
 * Turns whatever a container wrote into something a person can scan.
 *
 * Structured loggers (slog, zap, pino, logrus, GCP) all emit one JSON object per
 * line with a level, a time and a message under one of a handful of names. The
 * server ships the lines raw so this stays the one place that knows those names.
 */
export type Level = 'error' | 'warn' | 'info' | 'debug' | 'unknown';

export interface LogLine {
  raw: string;
  level: Level;
  /** The human-readable rendering; equals `raw` when the line is not JSON. */
  text: string;
}

const TIME_KEYS = ['time', 'ts', 'timestamp', '@timestamp'];
const LEVEL_KEYS = ['level', 'severity', 'lvl', 'log.level'];
const MSG_KEYS = ['msg', 'message', 'event'];

// pino's numeric levels; other numeric schemes are rare enough to ignore.
const PINO: Record<number, Level> = { 10: 'debug', 20: 'debug', 30: 'info', 40: 'warn', 50: 'error', 60: 'error' };

export function normaliseLevel(v: unknown): Level {
  if (typeof v === 'number') return PINO[v] ?? 'unknown';
  if (typeof v !== 'string') return 'unknown';
  const s = v.toLowerCase();
  if (/err|fatal|panic|crit|alert|emerg/.test(s)) return 'error';
  if (s.startsWith('warn')) return 'warn';
  if (s.startsWith('debug') || s.startsWith('trace')) return 'debug';
  if (s.startsWith('info') || s === 'notice') return 'info';
  return 'unknown';
}

// ponytail: a word match, not a parser. Catches `level=error`, `[WARN]`, klog's
// `E0827 …` and `ERROR:`; false positives ("no error") only miscolour a line.
const WORD = /(?:^|[\s[=:"])(error|err|fatal|panic|critical|warn|warning|info|debug|trace)(?:[\s\]:="]|$)/i;
const KLOG = /^([EWIF])\d{4} /;

export function guessLevel(line: string): Level {
  const klog = KLOG.exec(line);
  if (klog) return { E: 'error', F: 'error', W: 'warn', I: 'info' }[klog[1]] as Level;
  const m = WORD.exec(line);
  return m ? normaliseLevel(m[1]) : 'unknown';
}

/** HH:MM:SS.mmm in local time; the input string unchanged when it is not a time. */
export function shortTime(v: unknown): string {
  // zap and friends write epoch seconds as a float; pino writes epoch millis.
  const d = typeof v === 'number' ? new Date(v < 1e12 ? v * 1000 : v) : new Date(String(v));
  if (Number.isNaN(d.getTime())) return String(v);
  const pad = (n: number, w = 2) => String(n).padStart(w, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
}

function pick(obj: Record<string, unknown>, keys: string[]): [string, unknown] | undefined {
  for (const k of keys) if (k in obj) return [k, obj[k]];
  return undefined;
}

const scalar = (v: unknown) =>
  typeof v === 'string' && !/[\s"=]/.test(v) ? v : JSON.stringify(v);

export function parseLine(raw: string): LogLine {
  if (!raw.startsWith('{')) return { raw, level: guessLevel(raw), text: raw };
  let obj: unknown;
  try {
    obj = JSON.parse(raw);
  } catch {
    return { raw, level: guessLevel(raw), text: raw };
  }
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
    return { raw, level: 'unknown', text: raw };
  }
  const rec = obj as Record<string, unknown>;
  const time = pick(rec, TIME_KEYS);
  const lvl = pick(rec, LEVEL_KEYS);
  const msg = pick(rec, MSG_KEYS);
  const used = new Set([time?.[0], lvl?.[0], msg?.[0]]);
  const level = lvl ? normaliseLevel(lvl[1]) : 'unknown';

  const parts: string[] = [];
  if (time) parts.push(shortTime(time[1]));
  if (lvl) parts.push(level === 'unknown' ? String(lvl[1]) : level.toUpperCase().padEnd(5));
  if (msg) parts.push(typeof msg[1] === 'string' ? msg[1] : JSON.stringify(msg[1]));
  for (const [k, v] of Object.entries(rec)) {
    if (!used.has(k)) parts.push(`${k}=${scalar(v)}`);
  }
  return { raw, level, text: parts.join(' ') };
}

/** Splits a log body into parsed lines, dropping only the trailing newline. */
export function parseLogs(body: string): LogLine[] {
  const lines = body.split('\n');
  if (lines.length && lines[lines.length - 1] === '') lines.pop();
  return lines.map(parseLine);
}
